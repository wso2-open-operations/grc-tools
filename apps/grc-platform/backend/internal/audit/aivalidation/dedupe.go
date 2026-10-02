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
	"crypto/sha256"
	"sync"

	"github.com/wso2-open-operations/grc-tools/apps/grc-platform/backend/internal/shared/llm"
)

// maxCachedResults bounds the in-memory result cache; it is simply cleared
// when full — a miss only costs one extra LLM call.
const maxCachedResults = 500

// jobState tracks the one job allowed per submission at a time. started is
// set once the job begins reading the submission's files; a trigger arriving
// before that needs nothing (the queued job will read the latest files), one
// arriving after sets rerun so the job runs once more when it finishes.
type jobState struct {
	started bool
	rerun   bool
}

// dedupe keeps a burst of triggers on one submission (e.g. several "Add
// Files" clicks) down to at most one running job plus one follow-up, and
// remembers verdicts by input fingerprint so a byte-identical request is
// never sent to the model twice. In-memory only: lost on restart, which just
// means one more LLM call.
type dedupe struct {
	mu       sync.Mutex
	inflight map[submissionRef]*jobState
	results  map[[sha256.Size]byte]validationResult
}

func newDedupe() *dedupe {
	return &dedupe{
		inflight: make(map[submissionRef]*jobState),
		results:  make(map[[sha256.Size]byte]validationResult),
	}
}

// claim reports whether the caller should start a job for ref. false means a
// job for ref already exists and will cover this trigger.
func (d *dedupe) claim(ref submissionRef) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if st, ok := d.inflight[ref]; ok {
		if st.started {
			st.rerun = true
		}
		return false
	}
	d.inflight[ref] = &jobState{}
	return true
}

func (d *dedupe) markStarted(ref submissionRef) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if st, ok := d.inflight[ref]; ok {
		st.started = true
	}
}

// release ends one pass of ref's job. true means a trigger arrived mid-run and
// the job must run again; false means ref is released.
func (d *dedupe) release(ref submissionRef) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	st, ok := d.inflight[ref]
	if ok && st.rerun {
		*st = jobState{}
		return true
	}
	delete(d.inflight, ref)
	return false
}

func (d *dedupe) lookup(key [sha256.Size]byte) (validationResult, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	vr, ok := d.results[key]
	return vr, ok
}

func (d *dedupe) store(key [sha256.Size]byte, vr validationResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.results) >= maxCachedResults {
		clear(d.results)
	}
	d.results[key] = vr
}

// inputKey fingerprints everything the model sees that varies per call: the
// dynamic system prompt, the user-turn intro text, and every content block.
// The static prompt is a compile-time constant, so a change to it means a
// redeploy, which starts with an empty cache anyway.
func inputKey(dynamicPrompt, intro string, blocks []llm.Block) [sha256.Size]byte {
	h := sha256.New()
	h.Write([]byte(dynamicPrompt))
	h.Write([]byte{0})
	h.Write([]byte(intro))
	for _, b := range blocks {
		d := b.Digest()
		h.Write(d[:])
	}
	return [sha256.Size]byte(h.Sum(nil))
}
