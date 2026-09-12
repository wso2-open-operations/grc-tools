"""Builds a zip archive of one Evidence's files, laid out so a human can
browse it after extracting (spec issue #92).

Kept out of `blob_storage.py` deliberately, for the same reason
`blob_paths.py` is kept separate from it (see that module's docstring):
`blob_storage.py` talks to the Azure SDK and knows nothing about this app's
domain models, and should stay that way. This module is the domain-aware
half -- it knows about `Evidence`/`EvidenceFile`/`Control` and turns them
into a zip's internal folder names. It reads blob bytes through
`app.storage.blob_storage.read_file`, but never talks to Azure directly, and
it never writes anything to storage.

A zip entry path is presentation, chosen freely when writing the archive --
independent of where the blob actually lives (`blob_paths.py`'s module
docstring: storage has no `evidence` level). So this module is free to add
one:

    With a Control:     product/framework/control/{evidence-title}/NN-{evidence-title}-{uuid}{ext}
    Without a Control:  {evidence-title}/NN-{evidence-title}-{uuid}{ext}

The first three segments (Control case) come from `build_control_prefix`,
reused as-is. `{evidence-title}` is the Evidence's title run through the
existing `sanitize_title` -- the same sanitiser and the same fallback
(`FALLBACK_TITLE_LABEL`) used at upload time, so a title that sanitises to
nothing behaves identically here.

`NN` is the file's 1-based position among the (caller-ordered) files, zero-
padded to 2 digits, or 3 if there are more than 99 -- see
`_entry_number_width`. The `{uuid}{ext}` half of each entry name is pulled
back out of the stored blob's own basename (`_uuid_and_extension`) rather
than reused wholesale, because a Control-filed blob's basename already
contains the upload-time title label (`save_file`'s
`{prefix}{label}-{uuid}{ext}`) -- reusing it verbatim would duplicate the
title in the entry name. Pulling out just the uuid+extension and rebuilding
the label from this module's own `title` gives one consistent entry-name
shape regardless of whether the underlying blob happened to have a label.
"""
import os
import re
import tempfile
import zipfile
from collections.abc import Iterator
from concurrent.futures import ThreadPoolExecutor
from typing import TYPE_CHECKING

from app.storage.blob_paths import build_control_prefix, sanitize_title
from app.storage.blob_storage import read_file

if TYPE_CHECKING:
    from app.models.evidence import Evidence
    from app.models.evidence_file import EvidenceFile

# Matches a canonical uuid.uuid4() string (8-4-4-4-12 hex) plus whatever
# follows it (the extension, if any) at the end of a blob basename. Every
# blob name `save_file` produces ends in exactly one of these -- either
# `{uuid}{ext}` (no prefix/label) or `{label}-{uuid}{ext}` -- so anchoring
# on the uuid shape itself, rather than splitting on "-", is what lets this
# survive a label that itself contains hyphens.
_UUID_AND_EXTENSION_RE = re.compile(
    r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}.*$"
)


def _entry_number_width(file_count: int) -> int:
    """2 digits normally, 3 once a collection exceeds 99 files -- see the
    spec's zip-layout section."""
    return 3 if file_count > 99 else 2


def _uuid_and_extension(blob_name: str) -> str:
    """The `{uuid}{ext}` tail of a stored blob name, with any
    `product/framework/control/` path and upload-time title label stripped
    off. Falls back to the bare basename in the (should-never-happen) case
    a blob name doesn't contain a recognisable uuid, rather than raising --
    a defensive fallback, not an expected path."""
    basename = blob_name.rsplit("/", 1)[-1]
    match = _UUID_AND_EXTENSION_RE.search(basename)
    return match.group(0) if match else basename


# Past this much the archive spills to a temporary file on disk instead of
# staying in memory. A single upload is capped at 15 MB
# (`blob_storage.MAX_UPLOAD_SIZE_BYTES`) but the number of files on one
# Evidence is not capped, so an agent collection can total far more than any
# one file. Holding all of that per concurrent download would put the ceiling
# on worker memory rather than on disk, where it belongs. This bound is about
# the finished archive only; `_MAX_CONCURRENT_READS` below is what bounds the
# separate, transient memory a batch of not-yet-written file bytes uses while
# it's being read.
_SPOOL_MAX_BYTES = 16 * 1024 * 1024

# Size of each chunk handed to the response while streaming the archive back.
_STREAM_CHUNK_BYTES = 64 * 1024

# Reading a blob one at a time, as this function used to, costs about 5
# seconds per file against the real Stage storage account -- Azure's own
# work is ~20ms of that; the rest is round-trip network distance to the East
# US storage account. Sequentially, an Evidence with 38 files takes roughly
# 190 seconds, and Choreo's endpoint timeout is 30 seconds, so anything past
# about a dozen files 504s before the zip is ever built. Reading a batch of
# files concurrently turns that per-file network cost into a per-batch cost
# instead: 20 files at ~5 seconds each in parallel is ~5 seconds, not ~100.
#
# 20 is also the batch size (see `build_evidence_zip`), not just the pool's
# worker cap -- the two are the same number on purpose, so that reading one
# batch never holds more than 20 files' bytes in memory at a time, no matter
# how many files the whole Evidence has.
_MAX_CONCURRENT_READS = 20

# Shared by the whole application (created once, at import time) rather than
# a fresh pool per request. A per-request pool would let the number of
# concurrent blob reads grow with the number of people downloading at once --
# ten simultaneous downloads would mean ten pools of up to 20 threads each,
# 200 reads hitting the storage account (and holding memory) at the same
# time. A single module-level pool keeps that ceiling fixed at
# `_MAX_CONCURRENT_READS` reads in flight across the whole process, however
# many downloads are running concurrently -- the number this function is
# trying to protect is per-storage-account concurrency, not per-request
# concurrency.
_read_pool = ThreadPoolExecutor(
    max_workers=_MAX_CONCURRENT_READS, thread_name_prefix="evidence-zip-read"
)


def build_evidence_zip(
    evidence: "Evidence", files: list["EvidenceFile"]
) -> tempfile.SpooledTemporaryFile:
    """Build a zip of `files`, which the caller must have already ordered by
    `sort_order` -- this function trusts that order and does not re-sort, so
    entry `01` is whichever file is first in `files`.

    Returns an open temporary file positioned at the start. Ownership passes
    to the caller, which must close it; `stream_archive` below does that and
    is the intended way to consume the result.

    Callers are responsible for checking `files` is non-empty first: an
    empty zip is a confusing success, not something this function decides
    to allow or reject (see the route).
    """
    title = sanitize_title(evidence.title)
    folder = f"{build_control_prefix(evidence.control)}{title}" if evidence.control_id else title
    width = _entry_number_width(len(files))

    buffer = tempfile.SpooledTemporaryFile(max_size=_SPOOL_MAX_BYTES)
    with zipfile.ZipFile(buffer, mode="w", compression=zipfile.ZIP_DEFLATED) as archive:
        # Batches of `_MAX_CONCURRENT_READS` files at a time, read through
        # the shared `_read_pool` -- see that name's comment for why 20 and
        # why shared rather than per-request. `position` is still each
        # file's 1-based index across the WHOLE `files` list (`batch_start`
        # plus its offset within the batch), not its index within the
        # batch -- entry numbering and ordering must come out byte-for-byte
        # the same as the old single-file-at-a-time loop, batching is purely
        # an internal read-scheduling detail.
        for batch_start in range(0, len(files), _MAX_CONCURRENT_READS):
            batch = files[batch_start : batch_start + _MAX_CONCURRENT_READS]

            # `ThreadPoolExecutor.map` returns results in the same order as
            # its inputs (not completion order) and, if any call raised,
            # re-raises the first such exception -- in input order -- once
            # that result is reached. Wrapping it in `list()` forces the
            # whole batch to finish (or raise) before this batch writes
            # anything to the archive, so a missing blob still fails the
            # whole download loudly instead of producing a short zip with
            # only the files read before the failure.
            batch_contents = list(_read_pool.map(read_file, (ef.file_name for ef in batch)))

            for offset, (evidence_file, content) in enumerate(zip(batch, batch_contents)):
                position = batch_start + offset + 1
                entry_name = (
                    f"{folder}/{position:0{width}d}-{title}-"
                    f"{_uuid_and_extension(evidence_file.file_name)}"
                )
                archive.writestr(entry_name, content)

    buffer.seek(0)
    return buffer


def archive_size(archive: tempfile.SpooledTemporaryFile) -> int:
    """Byte length of a built archive, leaving it positioned back at the
    start. Lets the response still send a Content-Length, so the browser
    shows a real progress bar rather than an unknown-length download."""
    size = archive.seek(0, os.SEEK_END)
    archive.seek(0)
    return size


def stream_archive(archive: tempfile.SpooledTemporaryFile) -> Iterator[bytes]:
    """Yield the archive a chunk at a time and close it when done, including
    when the client disconnects part-way through and the response closes the
    generator early."""
    try:
        while chunk := archive.read(_STREAM_CHUNK_BYTES):
            yield chunk
    finally:
        archive.close()
