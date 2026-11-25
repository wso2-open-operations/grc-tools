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

package bulkimport

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// A real run stopped part-way by an entity outage must leave nothing it cannot
// pick up again: running the same file continues where it stopped, and writes no
// risk twice. These drive Service.Run itself, not one row at a time.

func resumeRows() []map[string]string {
	return []map[string]string{
		plainRow(map[string]string{"Migration ID": "1"}),
		plainRow(map[string]string{"Risk Title": "Second", "Migration ID": "2"}),
		plainRow(map[string]string{"Risk Title": "Third", "Workflow Status": "CLOSED", "Migration ID": "3"}),
	}
}

func TestRun_StoppedBetweenRisksPicksUpWhereItStopped(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)
	fe.failCreateAfter = 1 // the first risk is written, the second cannot be created

	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false, resumeRows()...))
	if !errors.Is(err, ErrStructural) {
		t.Fatalf("a mid run outage must abort the run: err = %v", err)
	}
	if res == nil || res.Outcome != OutcomeAborted {
		t.Fatalf("result = %+v", res)
	}
	if len(fe.createdRisks) != 1 {
		t.Fatalf("created %d risks before the outage, want 1", len(fe.createdRisks))
	}

	// The entity is back. The same file again.
	fe.failCreateAfter = 0
	res, err = svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false, resumeRows()...))
	if err != nil || res.Outcome != OutcomeClean {
		t.Fatalf("second run: outcome=%v err=%v findings=%+v", res.Outcome, err, res.Report.Findings())
	}
	if len(fe.createdRisks) != 3 {
		t.Errorf("created %d risks in all, want 3: the first must not be written twice", len(fe.createdRisks))
	}
	titles := map[string]int{}
	for _, r := range fe.createdRisks {
		titles[r.RiskTitle]++
	}
	for title, n := range titles {
		if n != 1 {
			t.Errorf("%q was created %d times", title, n)
		}
	}
	if c := res.Report.Counts(); c.Migrated != 2 || c.Skipped != 1 {
		t.Errorf("counts = %+v, want 2 written and 1 skipped as already complete", c)
	}
	var codes bytes.Buffer
	res.Report.WriteRiskCodesCSV(&codes)
	for _, want := range []string{"2025-ASG-Q3-0001", "2025-ASG-Q3-0002", "2025-ASG-Q3-0003"} {
		if !strings.Contains(codes.String(), want) {
			t.Errorf("risk_codes.csv is missing %s:\n%s", want, codes.String())
		}
	}
}

// The harder stop: the risk exists but is only half built (created, not yet
// walked to its status). The run must finish it, not create it a second time.
func TestRun_StoppedInsideARiskFinishesItWithoutCreatingItAgain(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)
	fe.failNextPatch = true // the status walk of the first risk fails

	_, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false, resumeRows()...))
	if !errors.Is(err, ErrStructural) {
		t.Fatalf("err = %v, want the run aborted", err)
	}
	if len(fe.createdRisks) != 1 {
		t.Fatalf("created %d risks before the outage, want the one half built risk", len(fe.createdRisks))
	}
	var stranded *Risk
	for _, r := range fe.risks {
		stranded = r
	}
	if stranded == nil || stranded.WorkflowStatus != "PENDING_RISK_OWNER_APPROVAL" {
		t.Fatalf("the first risk should be left at its starting status: %+v", stranded)
	}
	id := stranded.ID

	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false, resumeRows()...))
	if err != nil || res.Outcome != OutcomeClean {
		t.Fatalf("second run: outcome=%v err=%v findings=%+v", res.Outcome, err, res.Report.Findings())
	}
	if len(fe.createdRisks) != 3 {
		t.Errorf("created %d risks in all, want 3: the half built one must be finished, not repeated", len(fe.createdRisks))
	}
	if got := fe.risks[id].WorkflowStatus; got != "APPROVED" {
		t.Errorf("the half built risk ended at %q, want APPROVED", got)
	}
	if res.Report.Counts().Mismatches != 0 {
		t.Errorf("verify disagreed with what was written: %+v", res.Report.Findings())
	}
}

// A dry run in between says what is left, so the admin sees the state before
// deciding to run it again.
func TestRun_DryRunAfterAStoppedRunSaysWhatIsLeft(t *testing.T) {
	fe := newFakeEntity(t)
	svc := newRunService(t, fe)
	fe.failCreateAfter = 1
	if _, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, false, resumeRows()...)); err == nil {
		t.Fatal("the run should have stopped")
	}
	fe.failCreateAfter = 0

	res, err := svc.Run(context.Background(), runRequest(t, idStdReg, templateStandard, true, resumeRows()...))
	if err != nil || res.Outcome != OutcomeClean {
		t.Fatalf("dry run: outcome=%v err=%v", res.Outcome, err)
	}
	if c := res.Report.Counts(); !c.PlanKnown || c.ToWrite != 2 || c.AlreadyDone != 1 {
		t.Errorf("counts = %+v, want 2 left to write and 1 already done", c)
	}
}
