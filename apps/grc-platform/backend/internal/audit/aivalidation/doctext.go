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
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	// maxDocTextChars caps one document's extracted text (~25k tokens); the
	// rest is marked as not reviewed rather than silently dropped.
	maxDocTextChars = 100_000
	// maxXMLPartBytes bounds one decompressed XML part, against zip bombs.
	maxXMLPartBytes = 20 << 20
)

var (
	errNoDocText   = errors.New("no text parts found")
	slideNumber    = regexp.MustCompile(`slide(\d+)\.xml$`)
	extraBlankRuns = regexp.MustCompile(`\n{3,}`)
)

// extractOOXMLText returns a docx's body text (then its headers/footers,
// where version/approval details often sit) or a pptx's slide text in slide
// order, plus whether it was cut at maxDocTextChars. Table cells are
// tab-separated, paragraphs newline-separated; deleted tracked-change text
// and field codes are left out.
func extractOOXMLText(data []byte, ext string) (text string, truncated bool, err error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", false, err
	}

	type part struct {
		f     *zip.File
		label string
	}
	var parts []part
	switch ext {
	case "docx":
		var headerFooter []*zip.File
		for _, f := range zr.File {
			switch {
			case f.Name == "word/document.xml":
				parts = append([]part{{f, ""}}, parts...)
			case path.Dir(f.Name) == "word" &&
				(strings.HasPrefix(path.Base(f.Name), "header") || strings.HasPrefix(path.Base(f.Name), "footer")):
				headerFooter = append(headerFooter, f)
			}
		}
		sort.Slice(headerFooter, func(i, j int) bool { return headerFooter[i].Name < headerFooter[j].Name })
		for _, f := range headerFooter {
			parts = append(parts, part{f, "[" + strings.TrimSuffix(path.Base(f.Name), ".xml") + "]"})
		}
	case "pptx":
		type slide struct {
			f *zip.File
			n int
		}
		var slides []slide
		for _, f := range zr.File {
			if path.Dir(f.Name) != "ppt/slides" {
				continue
			}
			if m := slideNumber.FindStringSubmatch(f.Name); m != nil {
				n, _ := strconv.Atoi(m[1])
				slides = append(slides, slide{f, n})
			}
		}
		sort.Slice(slides, func(i, j int) bool { return slides[i].n < slides[j].n })
		for _, s := range slides {
			parts = append(parts, part{s.f, fmt.Sprintf("[Slide %d]", s.n)})
		}
	}
	if len(parts) == 0 {
		return "", false, errNoDocText
	}

	var out strings.Builder
	for _, p := range parts {
		t, err := xmlPartText(p.f)
		if err != nil {
			return "", false, err
		}
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if p.label != "" {
			out.WriteString(p.label + "\n")
		}
		out.WriteString(t)
		out.WriteString("\n\n")
	}

	text = strings.TrimSpace(extraBlankRuns.ReplaceAllString(out.String(), "\n\n"))
	if len(text) > maxDocTextChars {
		cut := maxDocTextChars
		for cut > 0 && !isRuneStart(text[cut]) {
			cut--
		}
		return text[:cut], true, nil
	}
	return text, false, nil
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// xmlPartText walks one WordprocessingML/DrawingML part and keeps only the
// visible text runs (<w:t>, <a:t>).
func xmlPartText(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	dec := xml.NewDecoder(io.LimitReader(rc, maxXMLPartBytes))
	var b strings.Builder
	// inTabStops: <w:tabs> holds paragraph tab-stop definitions, whose
	// <w:tab> children are formatting, not tab characters.
	// cellDepth > 0 inside a table cell: its paragraphs join with spaces so
	// each table row comes out as one tab-separated line.
	inText, inTabStops, cellDepth := false, false, 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return b.String(), nil
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "tabs":
				inTabStops = true
			case "tc":
				cellDepth++
			case "tab":
				if !inTabStops {
					b.WriteByte('\t')
				}
			case "br", "cr":
				b.WriteByte('\n')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "tabs":
				inTabStops = false
			case "p":
				if cellDepth > 0 {
					b.WriteByte(' ')
				} else {
					b.WriteByte('\n')
				}
			case "tc":
				cellDepth = max(cellDepth-1, 0)
				b.WriteByte('\t')
			case "tr":
				b.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}
}
