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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

// Package knowledge holds the written help the Risk assistant draws on when
// someone asks how to do something in the Risk Hub, where something is, or what
// a term means.
//
// It is plain Markdown in this directory, compiled into the program. basics.md
// describes the pages, buttons and fields as they are in the web app. Nothing
// here is data about risks: it is documentation, so sending the relevant parts
// to the model is safe.
package knowledge

import (
	"embed"
	"sort"
	"strings"
)

//go:embed *.md
var files embed.FS

// Section is one headed passage of the help text.
type Section struct {
	Source string // which document it came from
	Title  string
	Text   string
}

// maxSectionLen keeps a long section from filling the prompt.
const maxSectionLen = 1800

// alwaysTitles are included with every answer: the map of where things are is
// small and almost every how-to question needs it.
var alwaysTitles = map[string]bool{"Where things are in the Risk Hub": true}

// sources names each embedded file for the model and the reader.
var sources = map[string]string{
	"basics.md": "Risk Hub basics",
}

var sections = load()

func load() []Section {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil
	}
	var all []Section
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".md") || name == "README.md" {
			continue
		}
		raw, err := files.ReadFile(name)
		if err != nil {
			continue
		}
		source := sources[name]
		if source == "" {
			source = strings.TrimSuffix(name, ".md")
		}
		all = append(all, parse(source, string(raw))...)
	}
	return all
}

// parse splits Markdown into sections at its headings. Text before the first
// heading, and headings with no text under them, are dropped.
func parse(source, md string) []Section {
	var out []Section
	var title string
	var body []string
	flush := func() {
		text := strings.TrimSpace(strings.Join(body, "\n"))
		if title != "" && text != "" {
			if len(text) > maxSectionLen {
				text = cut(text, maxSectionLen)
			}
			out = append(out, Section{Source: source, Title: title, Text: text})
		}
		body = nil
	}
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "#") {
			flush()
			title = strings.TrimSpace(strings.TrimLeft(line, "#"))
			continue
		}
		body = append(body, line)
	}
	flush()
	return out
}

// cut shortens text to at most n bytes, at the end of a line where it can.
func cut(text string, n int) string {
	head := text[:n]
	if i := strings.LastIndex(head, "\n"); i > n/2 {
		head = head[:i]
	}
	return strings.TrimSpace(head)
}

var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "to": true, "of": true, "in": true, "on": true, "at": true, "for": true,
	"and": true, "or": true, "is": true, "are": true, "was": true, "be": true, "do": true, "does": true, "did": true,
	"how": true, "what": true, "where": true, "when": true, "who": true, "why": true, "which": true, "can": true,
	"i": true, "me": true, "my": true, "we": true, "you": true, "it": true, "this": true, "that": true, "with": true,
	"from": true, "by": true, "as": true, "if": true, "so": true, "about": true, "please": true, "tell": true,
	"show": true, "give": true, "need": true, "want": true, "should": true, "would": true, "could": true,
}

// tokens lower-cases text and splits it into words, dropping filler words and
// folding a trailing "s" so that "risks" and "risk" match.
func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if stopwords[w] || len(w) < 2 {
			continue
		}
		if len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") {
			w = strings.TrimSuffix(w, "s")
		}
		out[w] = true
	}
	return out
}

// Relevant returns the sections that best match a question: the always-included
// ones, then up to max more, best first. Sections that share no word with the
// question are not returned.
func Relevant(question string, max int) []Section {
	q := tokens(question)
	type scored struct {
		s     Section
		score int
		order int
	}
	var matches []scored
	var always []Section
	for i, sec := range sections {
		if alwaysTitles[sec.Title] {
			always = append(always, sec)
			continue
		}
		score := 0
		title := tokens(sec.Title)
		body := tokens(sec.Text)
		for w := range q {
			if title[w] {
				score += 3
			}
			if body[w] {
				score++
			}
		}
		if score > 0 {
			matches = append(matches, scored{sec, score, i})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].order < matches[j].order
	})
	out := append([]Section{}, always...)
	for i := 0; i < len(matches) && i < max; i++ {
		out = append(out, matches[i].s)
	}
	return out
}

// All returns every section, for tests and tooling.
func All() []Section { return sections }
