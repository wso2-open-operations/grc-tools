import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path

from azure.core.exceptions import ResourceNotFoundError
from azure.storage.blob import BlobSasPermissions, BlobServiceClient, generate_blob_sas
from fastapi import UploadFile

from app.config import settings

# How long a signed link stays valid after it's generated. Chosen per ADR
# 0003: long enough to browse a gallery of evidence without a link dying
# mid-view, short enough that a leaked link is quickly useless.
SIGNED_URL_EXPIRY_MINUTES = 15

_blob_service: BlobServiceClient | None = None


def _get_blob_service() -> BlobServiceClient:
    global _blob_service
    if _blob_service is None:
        _blob_service = BlobServiceClient.from_connection_string(
            settings.AZURE_STORAGE_CONNECTION_STRING
        )
    return _blob_service


def save_file(file: UploadFile) -> tuple[str, str]:
    """Upload an evidence file to blob storage and return (blob_name, public_url)."""
    extension = Path(file.filename).suffix
    unique_name = f"{uuid.uuid4()}{extension}"
    blob = _get_blob_service().get_blob_client(
        container=settings.AZURE_STORAGE_CONTAINER, blob=unique_name
    )
    blob.upload_blob(file.file, overwrite=True)
    return unique_name, f"/uploads/{unique_name}"


def delete_file(file_name: str) -> None:
    """Delete a blob by name. Missing blobs are treated as already deleted."""
    blob = _get_blob_service().get_blob_client(
        container=settings.AZURE_STORAGE_CONTAINER, blob=file_name
    )
    try:
        blob.delete_blob()
    except ResourceNotFoundError:
        pass


def get_signed_url(file_ref: str, expiry_minutes: int = SIGNED_URL_EXPIRY_MINUTES) -> str:
    """Convert a stored file reference into a freshly generated, time-limited
    Azure SAS URL that points directly at the blob (see ADR 0003).

    `file_ref` is whatever is stored on `Evidence`/`EvidenceFile.file_url`
    today — the `"/uploads/{blob_name}"` form `save_file` returns — but a
    bare blob name is also accepted defensively. The stored value itself is
    never touched; this only runs at read/serialization time, so existing
    rows need no migration.

    `generate_blob_sas` is purely local (no network call) — it only needs
    the account name and account key, both already known from the storage
    connection string.
    """
    blob_name = file_ref.removeprefix("/uploads/")
    service = _get_blob_service()
    blob_client = service.get_blob_client(
        container=settings.AZURE_STORAGE_CONTAINER, blob=blob_name
    )
    sas_token = generate_blob_sas(
        account_name=service.account_name,
        container_name=settings.AZURE_STORAGE_CONTAINER,
        blob_name=blob_name,
        account_key=service.credential.account_key,
        permission=BlobSasPermissions(read=True),
        expiry=datetime.now(timezone.utc) + timedelta(minutes=expiry_minutes),
    )
    return f"{blob_client.url}?{sas_token}"
