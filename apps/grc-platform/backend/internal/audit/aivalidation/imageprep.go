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
	"bytes"
	"crypto/sha256"
	"image"
	_ "image/gif" // register decoders for image.DecodeConfig / image.Decode
	"image/jpeg"
	"image/png"

	_ "golang.org/x/image/webp"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/llm"
)

const (
	// maxImagesPerJob caps image blocks sent per job, tiles included.
	maxImagesPerJob = 20
	// maxPDFPagesPerJob caps PDF pages sent per job — each page costs both
	// its text and a rendered page image.
	maxPDFPagesPerJob = 50
	// minEmbeddedImageEdge: an image extracted from a docx/pptx/doc/ppt/xls
	// smaller than this on its longest side is a logo, icon or bullet, not a
	// screenshot. Uploaded image files are never dropped this way.
	minEmbeddedImageEdge = 200
	// maxImageEdge is the longest side the model reads at full resolution;
	// the API scales anything larger down to it.
	maxImageEdge = 2576
	// tileOverlap repeats a strip between consecutive tiles so a text line
	// cut by one tile boundary is whole in the next.
	tileOverlap = 64
	// maxTilePixels bounds a full decode for tiling; a larger image is sent
	// as-is rather than decoded into a huge in-memory bitmap.
	maxTilePixels = 60_000_000
)

// jobBudget is shared by every file in one job, so the image and PDF-page
// caps and duplicate detection apply across the whole submission (and the
// auditor's sample files on an evidence job).
type jobBudget struct {
	images   int
	pdfPages int
	seen     map[[sha256.Size]byte]bool
}

func newJobBudget() *jobBudget {
	return &jobBudget{seen: make(map[[sha256.Size]byte]bool)}
}

// imageOutcome is what happened to one candidate image.
type imageOutcome int

const (
	imageIncluded imageOutcome = iota
	imageDuplicate
	imageTiny
	imageOverCap
	imageInvalid
)

// addImage decodes the image's header, then drops it (tiny embedded image,
// exact duplicate, cap reached, undecodable) or returns its blocks — one, or
// several tiles for a long screenshot the API would otherwise shrink until
// its text is unreadable.
func (b *jobBudget) addImage(data []byte, embedded bool) ([]llm.Block, imageOutcome) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, imageInvalid
	}
	if embedded && max(cfg.Width, cfg.Height) < minEmbeddedImageEdge {
		return nil, imageTiny
	}
	sum := sha256.Sum256(data)
	if b.seen[sum] {
		return nil, imageDuplicate
	}

	tiles := tileImage(data, cfg, format)
	if b.images+len(tiles) > maxImagesPerJob {
		return nil, imageOverCap
	}
	b.images += len(tiles)
	b.seen[sum] = true

	blocks := make([]llm.Block, 0, len(tiles))
	for _, t := range tiles {
		blocks = append(blocks, llm.NewImageBlock(t.mediaType, t.data))
	}
	return blocks, imageIncluded
}

// reservePDFPages charges pages against the job's PDF-page cap. pages <= 0
// means the count couldn't be read; such a PDF is let through uncounted.
func (b *jobBudget) reservePDFPages(pages int) bool {
	if pages <= 0 {
		return true
	}
	if b.pdfPages+pages > maxPDFPagesPerJob {
		return false
	}
	b.pdfPages += pages
	return true
}

type imageTile struct {
	mediaType string
	data      []byte
}

// tileImage splits an elongated image (long side over maxImageEdge and at
// least twice the short side — a scrolling capture) into overlapping
// maxImageEdge-long tiles along its long axis, so each tile reaches the
// model unscaled. Anything else, or anything that fails to decode/encode,
// comes back as the original single image.
func tileImage(data []byte, cfg image.Config, format string) []imageTile {
	original := []imageTile{{mediaType: "image/" + format, data: data}}
	w, h := cfg.Width, cfg.Height
	long, short := max(w, h), min(w, h)
	if long <= maxImageEdge || long < 2*short || w*h > maxTilePixels {
		return original
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return original
	}
	sub, ok := img.(interface {
		SubImage(r image.Rectangle) image.Image
	})
	if !ok {
		return original
	}

	bounds := img.Bounds()
	vertical := h >= w
	step := maxImageEdge - tileOverlap
	var tiles []imageTile
	for off := 0; ; off += step {
		end := min(off+maxImageEdge, long)
		var r image.Rectangle
		if vertical {
			r = image.Rect(bounds.Min.X, bounds.Min.Y+off, bounds.Max.X, bounds.Min.Y+end)
		} else {
			r = image.Rect(bounds.Min.X+off, bounds.Min.Y, bounds.Min.X+end, bounds.Max.Y)
		}
		t, err := encodeTile(sub.SubImage(r), format)
		if err != nil {
			return original
		}
		tiles = append(tiles, t)
		if end == long {
			return tiles
		}
	}
}

// encodeTile keeps a JPEG source as JPEG (re-encoding a photo as PNG can
// balloon its size); everything else becomes lossless PNG, which keeps
// screenshot text crisp.
func encodeTile(img image.Image, format string) (imageTile, error) {
	var buf bytes.Buffer
	if format == "jpeg" {
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
			return imageTile{}, err
		}
		return imageTile{mediaType: "image/jpeg", data: buf.Bytes()}, nil
	}
	if err := png.Encode(&buf, img); err != nil {
		return imageTile{}, err
	}
	return imageTile{mediaType: "image/png", data: buf.Bytes()}, nil
}
