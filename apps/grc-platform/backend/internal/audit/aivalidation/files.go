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
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/llm"
)

// Caps bound each job's cost — skipped items are always named back to the
// model rather than silently dropped, so a PASS can never rest on a file the
// model never actually saw.
const (
	maxFiles      = 10
	maxFileBytes  = 5 << 20  // ~5 MB
	maxTotalBytes = 20 << 20 // ~20 MB
)

// supportedExt is the evidence/population upload accept-list, minus
// zip/msg/eml, which are always listed as unsupported.
var supportedExt = map[string]bool{
	"pdf": true, "doc": true, "docx": true, "xls": true, "xlsx": true,
	"ppt": true, "pptx": true, "csv": true, "txt": true,
	"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true,
}

// fileRef is one submitted file's identity, independent of whether it came
// from an evidence round or a population round.
type fileRef struct {
	ID   int
	Name string
}

// FileDownloader fetches one file's bytes by ID. Satisfied by
// service.EvidenceService and service.PopulationService's DownloadFile
// methods without any adapter.
type FileDownloader interface {
	DownloadFile(ctx context.Context, fileID int) (data []byte, fileName, contentType string, err error)
}

// buildFileContent fetches and converts files into LLM content blocks, plus
// a manifest text block listing every file's fate (reviewed or why not) —
// the system prompt tells the model never to base a PASS on an unreviewed
// file, so this manifest matters as much as the blocks themselves.
func buildFileContent(ctx context.Context, dl FileDownloader, files []fileRef, budget *jobBudget) ([]llm.Block, string) {
	var blocks []llm.Block
	var manifest strings.Builder
	manifest.WriteString("Files in this submission:\n")
	if len(files) == 0 {
		manifest.WriteString("(none)\n")
	}

	var total int64
	for i, f := range files {
		if i >= maxFiles {
			fmt.Fprintf(&manifest, "- %s: not reviewed (over the %d-file cap for this job)\n", f.Name, maxFiles)
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(path.Ext(f.Name), "."))
		if !supportedExt[ext] {
			fmt.Fprintf(&manifest, "- %s: not reviewed (unsupported format)\n", f.Name)
			continue
		}
		data, _, _, err := dl.DownloadFile(ctx, f.ID)
		if err != nil {
			fmt.Fprintf(&manifest, "- %s: not reviewed (could not be read)\n", f.Name)
			continue
		}
		if int64(len(data)) > maxFileBytes {
			fmt.Fprintf(&manifest, "- %s: not reviewed (exceeds the %dMB per-file cap)\n", f.Name, maxFileBytes>>20)
			continue
		}
		if total+int64(len(data)) > maxTotalBytes {
			fmt.Fprintf(&manifest, "- %s: not reviewed (job's %dMB total cap reached)\n", f.Name, maxTotalBytes>>20)
			continue
		}

		newBlocks, note := blocksForFile(ext, f.Name, data, budget)
		fmt.Fprintf(&manifest, "- %s: %s\n", f.Name, note)
		if len(newBlocks) > 0 {
			blocks = append(blocks, newBlocks...)
			total += int64(len(data))
		}
	}
	return blocks, manifest.String()
}

// blocksForFile converts one file's bytes into content blocks by extension,
// plus a short manifest note.
func blocksForFile(ext, name string, data []byte, budget *jobBudget) (blocks []llm.Block, manifestNote string) {
	switch ext {
	case "pdf":
		// Embedded screenshots on a PDF page are already visible to the model
		// as a native document block — no separate extraction needed.
		pages := pdfPageCount(data)
		if !budget.reservePDFPages(pages) {
			return nil, fmt.Sprintf("not reviewed (%d pages; job's %d-page PDF cap reached)", pages, maxPDFPagesPerJob)
		}
		return []llm.Block{llm.NewPDFBlock(data)}, "reviewed"
	case "png", "jpg", "jpeg", "gif", "webp":
		if !supportedImageMediaType(http.DetectContentType(data)) {
			return nil, "not reviewed (image format not supported)"
		}
		return uploadedImageBlocks(data, budget)
	case "xlsx":
		text, err := XLSXToCSV(data)
		if err != nil {
			return nil, "not reviewed (could not parse spreadsheet)"
		}
		return []llm.Block{llm.NewTextBlock("--- " + name + " ---\n" + text)}, "reviewed"
	case "csv":
		return []llm.Block{llm.NewTextBlock("--- " + name + " ---\n" + csvHeadTail(data))}, "reviewed"
	case "txt":
		return []llm.Block{llm.NewTextBlock("--- " + name + " ---\n" + string(data))}, "reviewed"
	case "docx", "pptx":
		return ooxmlDocumentBlocks(ext, name, data, budget)
	case "doc", "ppt", "xls":
		return legacyDocumentBlocks(ext, name, data, budget)
	default:
		// zip/msg/eml, or anything else not in supportedExt — unreachable in
		// practice since the caller gates on supportedExt first, kept as a
		// safety net.
		return nil, "not reviewed (unsupported format)"
	}
}

// uploadedImageBlocks handles an image file uploaded as itself.
func uploadedImageBlocks(data []byte, budget *jobBudget) ([]llm.Block, string) {
	blocks, outcome := budget.addImage(data, false)
	switch outcome {
	case imageDuplicate:
		return nil, "reviewed (identical to an image already included)"
	case imageOverCap:
		return nil, fmt.Sprintf("not reviewed (job's %d-image cap reached)", maxImagesPerJob)
	case imageInvalid:
		return nil, "not reviewed (image could not be decoded)"
	}
	if len(blocks) > 1 {
		return blocks, fmt.Sprintf("reviewed (long image, sent as %d overlapping segments)", len(blocks))
	}
	return blocks, "reviewed"
}

// ooxmlDocumentBlocks sends a docx/pptx as its text followed by its embedded
// pictures, each labeled with the file they came from so an image is read in
// the context of its document.
func ooxmlDocumentBlocks(ext, name string, data []byte, budget *jobBudget) ([]llm.Block, string) {
	var blocks []llm.Block
	var textNote string
	text, truncated, err := extractOOXMLText(data, ext)
	switch {
	case err != nil:
		textNote = "document text NOT reviewed (could not be read)"
	case text == "":
		textNote = "document has no text"
	default:
		blocks = append(blocks, llm.NewTextBlock("--- "+name+" (document text) ---\n"+text))
		textNote = "document text reviewed"
		if truncated {
			textNote = fmt.Sprintf("document text reviewed only up to the first %d characters; the rest NOT reviewed", maxDocTextChars)
		}
	}

	imgBlocks, imgParts := embeddedImageBlocks(extractOOXMLImages(data), budget)
	if len(imgBlocks) > 0 {
		blocks = append(blocks, llm.NewTextBlock("--- images embedded in "+name+" ---"))
		blocks = append(blocks, imgBlocks...)
	}
	return blocks, strings.Join(append([]string{textNote}, imgParts...), "; ")
}

// legacyDocumentBlocks handles a binary doc/ppt/xls, whose text this package
// can't read — only its embedded pictures are sent, and the note says so, so
// the model treats the document's content as unreviewed.
func legacyDocumentBlocks(ext, name string, data []byte, budget *jobBudget) ([]llm.Block, string) {
	textNote := fmt.Sprintf("document text NOT reviewed (legacy .%s format; only embedded images could be read)", ext)
	imgBlocks, imgParts := embeddedImageBlocks(extractLegacyOLEImages(data), budget)
	if len(imgBlocks) == 0 && len(imgParts) == 0 {
		return nil, fmt.Sprintf("not reviewed (legacy .%s format: text cannot be read and no embedded screenshots found)", ext)
	}
	blocks := []llm.Block{}
	if len(imgBlocks) > 0 {
		blocks = append(blocks, llm.NewTextBlock("--- images embedded in "+name+" ---"))
		blocks = append(blocks, imgBlocks...)
	}
	return blocks, strings.Join(append([]string{textNote}, imgParts...), "; ")
}

// embeddedImageBlocks handles the pictures extracted from a document and
// returns manifest phrases accounting for every one of them — any left out
// by the image cap is named as not reviewed, so a PASS can't rest on it.
// Both return values are empty when the document holds no real pictures.
func embeddedImageBlocks(imgs []extractedImage, budget *jobBudget) ([]llm.Block, []string) {
	var blocks []llm.Block
	var included, tiny, dup, overCap, invalid int
	for _, img := range imgs {
		b, outcome := budget.addImage(img.Data, true)
		switch outcome {
		case imageIncluded:
			blocks = append(blocks, b...)
			included++
		case imageTiny:
			tiny++
		case imageDuplicate:
			dup++
		case imageOverCap:
			overCap++
		case imageInvalid:
			invalid++
		}
	}
	if included == 0 && overCap == 0 && invalid == 0 {
		return nil, nil
	}

	parts := []string{fmt.Sprintf("%d embedded image(s) reviewed", included)}
	if tiny > 0 {
		parts = append(parts, fmt.Sprintf("%d small icon/logo image(s) skipped", tiny))
	}
	if dup > 0 {
		parts = append(parts, fmt.Sprintf("%d duplicate image(s) skipped", dup))
	}
	if invalid > 0 {
		parts = append(parts, fmt.Sprintf("%d image(s) not reviewed (could not be decoded)", invalid))
	}
	if overCap > 0 {
		parts = append(parts, fmt.Sprintf("%d image(s) not reviewed (job's %d-image cap reached)", overCap, maxImagesPerJob))
	}
	return blocks, parts
}
