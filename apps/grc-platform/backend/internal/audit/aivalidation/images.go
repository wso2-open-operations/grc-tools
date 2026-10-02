// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package aivalidation

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/richardlehane/mscfb"
)

// extractedImage is one embedded picture pulled out of a docx/doc/ppt/pptx/xls
// container, ready to become an llm.Block.
type extractedImage struct {
	Name      string
	MediaType string // image/png | image/jpeg | image/gif | image/webp
	Data      []byte
}

// supportedImageMediaType reports whether typ is one of the image types the
// Anthropic API accepts as a native image block — vector/legacy formats
// (EMF, WMF, TIFF, BMP) are not, and are dropped rather than sent unreadable.
func supportedImageMediaType(typ string) bool {
	switch typ {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

// extractOOXMLImages pulls embedded pictures out of a docx or pptx file —
// both are zip archives with media under word/media/ or ppt/media/. Screenshots
// are routinely embedded this way, and unlike xlsx there is no cell data to
// also extract, only the pictures.
func extractOOXMLImages(data []byte) []extractedImage {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}
	var out []extractedImage
	for _, f := range zr.File {
		if !strings.Contains(f.Name, "/media/") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, 20<<20))
		rc.Close()
		if err != nil {
			continue
		}
		mediaType := http.DetectContentType(b)
		if !supportedImageMediaType(mediaType) {
			continue
		}
		out = append(out, extractedImage{Name: path.Base(f.Name), MediaType: mediaType, Data: b})
	}
	return out
}

// extractLegacyOLEImages pulls embedded pictures out of a legacy binary
// doc/ppt/xls (OLE Compound File Binary Format) file. There is no lifted
// parser for this container's picture records (MS-ODRAW), so this walks
// every stream and carves out images by file-format signature instead of
// parsing the Escher drawing structure — a pragmatic best-effort approach:
// most embedded screenshots in these legacy formats sit as a near-verbatim
// image file inside a stream, wrapped in a small vendor header. A screenshot
// this misses is reported as an unreviewed file by the caller, same as any
// other file the model can't read — never silently treated as compliant.
func extractLegacyOLEImages(data []byte) []extractedImage {
	r, err := mscfb.New(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	var out []extractedImage
	for range 512 { // hard cap: a compound file can hold many streams
		entry, err := r.Next()
		if err != nil {
			break
		}
		if entry.Size <= 0 || entry.Size > 20<<20 {
			continue
		}
		buf := make([]byte, entry.Size)
		n, _ := io.ReadFull(r, buf)
		buf = buf[:n]
		for _, img := range carveImages(buf) {
			img.Name = entry.Name
			out = append(out, img)
			if len(out) >= 20 {
				return out
			}
		}
	}
	return out
}

// carveImages scans b for embedded JPEG/PNG/GIF file signatures and returns
// each one as a separate image, start-to-end-marker. See
// extractLegacyOLEImages for why this signature-carving approach is used
// instead of a full MS-ODRAW parse.
func carveImages(b []byte) []extractedImage {
	var out []extractedImage
	out = append(out, carveBySignature(b, []byte{0xFF, 0xD8, 0xFF}, []byte{0xFF, 0xD9}, "image/jpeg", true)...)
	out = append(out, carveBySignature(b, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, []byte("IEND"), "image/png", false)...)
	out = append(out, carveBySignature(b, []byte("GIF89a"), []byte{0x3B}, "image/gif", true)...)
	out = append(out, carveBySignature(b, []byte("GIF87a"), []byte{0x3B}, "image/gif", true)...)
	return out
}

// carveBySignature finds every non-overlapping occurrence of start in b and
// extracts through the next occurrence of end (inclusive). When
// endIsFileEnd is false, end (e.g. PNG's "IEND") is followed by a 4-byte CRC
// that belongs to the file too.
func carveBySignature(b, start, end []byte, mediaType string, endIsFileEnd bool) []extractedImage {
	var out []extractedImage
	pos := 0
	for {
		i := bytes.Index(b[pos:], start)
		if i < 0 {
			break
		}
		from := pos + i
		j := bytes.Index(b[from+len(start):], end)
		if j < 0 {
			break
		}
		to := from + len(start) + j + len(end)
		if !endIsFileEnd {
			to += 4 // CRC32 trailing the PNG IEND chunk tag
		}
		if to > len(b) {
			to = len(b)
		}
		out = append(out, extractedImage{MediaType: mediaType, Data: b[from:to]})
		pos = to
		if len(out) >= 20 {
			break
		}
	}
	return out
}
