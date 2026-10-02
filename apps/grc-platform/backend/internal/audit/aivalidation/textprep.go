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
	"encoding/csv"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// csvHeadRows / csvTailRows: a long CSV is sent as its header, first and
// last rows and the true row count — enough for the population rule's
// first-entry / last-entry / count cross-check without sending every row.
const (
	csvHeadRows = 50
	csvTailRows = 50
)

// csvHeadTail returns the CSV unchanged when short, otherwise header + first
// csvHeadRows + last csvTailRows records with an explicit omission marker
// and data-row count. Records are parsed, not split on newlines, so quoted
// multi-line fields count as one row; unparseable input falls back to lines.
func csvHeadTail(data []byte) string {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return linesHeadTail(string(data))
	}
	if len(records) <= 1+csvHeadRows+csvTailRows {
		return string(data)
	}

	dataRows := len(records) - 1
	var out strings.Builder
	w := csv.NewWriter(&out)
	_ = w.Write(records[0])
	_ = w.WriteAll(records[1 : 1+csvHeadRows])
	fmt.Fprintf(&out, "[... %d rows omitted ...]\n", dataRows-csvHeadRows-csvTailRows)
	w = csv.NewWriter(&out)
	_ = w.WriteAll(records[len(records)-csvTailRows:])
	fmt.Fprintf(&out, "[file has %d data rows plus a header row; showing the first %d and last %d]\n",
		dataRows, csvHeadRows, csvTailRows)
	return out.String()
}

// linesHeadTail is csvHeadTail's fallback for a file csv can't parse.
func linesHeadTail(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\r\n"), "\n")
	if len(lines) <= 1+csvHeadRows+csvTailRows {
		return text
	}
	dataRows := len(lines) - 1
	var out strings.Builder
	out.WriteString(strings.Join(lines[:1+csvHeadRows], "\n"))
	fmt.Fprintf(&out, "\n[... %d lines omitted ...]\n", dataRows-csvHeadRows-csvTailRows)
	out.WriteString(strings.Join(lines[len(lines)-csvTailRows:], "\n"))
	fmt.Fprintf(&out, "\n[file has %d lines after the first; showing the first %d and last %d]\n",
		dataRows, csvHeadRows, csvTailRows)
	return out.String()
}

var (
	pdfPageObj   = regexp.MustCompile(`/Type\s*/Page[^s]`)
	pdfPagesTree = regexp.MustCompile(`/Type\s*/Pages\b[^>]*?/Count\s+(\d+)|/Count\s+(\d+)[^>]*?/Type\s*/Pages\b`)
)

// pdfPageCount is a best-effort page count without a PDF parser: the largest
// /Count on a /Pages tree node (the root's is the total), else the number of
// /Type /Page objects. Returns 0 when neither is visible — e.g. the page tree
// sits inside a compressed object stream — meaning "unknown".
func pdfPageCount(data []byte) int {
	best := 0
	for _, m := range pdfPagesTree.FindAllSubmatch(data, -1) {
		for _, g := range m[1:] {
			if n, err := strconv.Atoi(string(g)); err == nil && n > best {
				best = n
			}
		}
	}
	if best > 0 {
		return best
	}
	return len(pdfPageObj.FindAllIndex(data, -1))
}
