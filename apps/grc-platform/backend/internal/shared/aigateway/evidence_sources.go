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

package aigateway

import "strings"

// evidenceSource is one verified threat-intel feed the Likelihood suggestion
// is allowed to consult. This list is the compliance-approved set and is the
// ONLY evidence the model may use: buildLikelihoodPrompt names every feed and
// likelihoodTools restricts web_search/web_fetch to Domains, so the prompt's
// instruction is enforced at the tool layer, not just asked for.
//
// It lives in code rather than .env: it is a fixed, reviewed list (not a
// secret or a per-environment setting), and each entry carries more than one
// field.
type evidenceSource struct {
	Name    string
	FeedURL string
	// Domains is every host the model may reach for this source — the feed's
	// own host plus the host its item links point at when that differs
	// (e.g. the Google feed is served from feedburner, items live on
	// security.googleblog.com; Kubernetes CVE items link to GitHub issues).
	Domains []string
	// Filtered is true for general security-news feeds, where most items are
	// unrelated to any given risk and the model must pick out only the ones
	// about the risk's subject. False for pure CVE/advisory feeds, where every
	// item is a vulnerability and the model only needs to match product or
	// component.
	Filtered bool
}

// The Guardian (https://www.theguardian.com/technology/data-computer-security/rss)
// is on compliance's verified list but deliberately NOT here: it blocks
// Anthropic's crawler, and the API rejects the ENTIRE request with a 400
// ("domains are not accessible to our user agent") if any allowed_domains
// entry is unreachable — which broke every likelihood suggestion. Re-add it
// only if that block is lifted (re-check with a one-domain request first).
var evidenceSources = []evidenceSource{
	{"The Hacker News", "https://feeds.feedburner.com/TheHackersNews/", []string{"feeds.feedburner.com", "thehackernews.com"}, true},
	{"BleepingComputer", "https://www.bleepingcomputer.com/feed/", []string{"bleepingcomputer.com"}, true},
	{"Kubernetes official CVE feed", "https://kubernetes.io/docs/reference/issues-security/official-cve-feed/index.json", []string{"kubernetes.io", "github.com"}, false},
	{"Ubuntu Security Notices", "https://ubuntu.com/security/notices/rss.xml", []string{"ubuntu.com"}, false},
	{"ScienceSoft", "https://www.scnsoft.com/blog/category/information-security/atom", []string{"scnsoft.com"}, true},
	{"Google Online Security Blog", "http://feeds.feedburner.com/GoogleOnlineSecurityBlog", []string{"feeds.feedburner.com", "security.googleblog.com"}, true},
	{"TechRepublic Security", "https://www.techrepublic.com/rssfeeds/topic/security/?feedType=rssfeeds", []string{"techrepublic.com"}, true},
	{"Schneier on Security", "https://www.schneier.com/feed/atom/", []string{"schneier.com"}, true},
	{"Proofpoint", "https://www.proofpoint.com/us/rss.xml", []string{"proofpoint.com"}, true},
}

// evidenceDomains returns the de-duplicated union of every source's Domains,
// in list order — the allowed_domains value for the likelihood tools.
func evidenceDomains() []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range evidenceSources {
		for _, d := range s.Domains {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
	}
	return out
}

// writeEvidenceSources renders the source list for the Likelihood prompt.
func writeEvidenceSources(b *strings.Builder) {
	for _, s := range evidenceSources {
		kind := "advisory/CVE feed — every item is a vulnerability; match on the affected product or component"
		if s.Filtered {
			kind = "general security-news feed — most items will be unrelated; use only items clearly about this risk's subject"
		}
		b.WriteString("- " + s.Name + ": " + s.FeedURL + " (" + kind + ")\n")
	}
}
