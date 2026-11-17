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

package deadline

import (
	"fmt"
	"testing"
	"time"
)

func day(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, Location) }

func at(m time.Month, d, h, min int) time.Time {
	return time.Date(2026, m, d, h, min, 0, 0, Location)
}

func ptr[T any](v T) *T { return &v }

func logString(ev Evaluation) []string {
	out := []string{}
	names := map[string]string{Owner: "Internal", Compliance: "GRC", Auditor: "EY"}
	for _, e := range ev.DelayLog {
		out = append(out, fmt.Sprintf("%s - %02d/%02d", names[e.HandledBy], int(e.Date.Month()), e.Date.Day()))
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestHandlerForStatus(t *testing.T) {
	cases := map[string]string{
		"EVIDENCE_PENDING":            Owner,
		"EVIDENCE_NEED_CLARIFICATION": Owner,
		"EVIDENCE_INTERNAL_REVIEW":    Compliance,
		"EVIDENCE_UNDER_VALIDATION":   Auditor,
	}
	for status, want := range cases {
		got, ok := HandlerForStatus(status)
		if !ok || got != want {
			t.Errorf("%s: got %q,%v want %q", status, got, ok, want)
		}
	}
	if _, ok := HandlerForStatus("COMPLETE"); ok {
		t.Error("COMPLETE should not be tracked")
	}
}

func TestStageFor(t *testing.T) {
	cases := []struct {
		status, phase, handler string
		oe                     bool
	}{
		{"POPULATION_PENDING", PhasePopulation, Owner, true},
		{"POPULATION_NEED_CLARIFICATION", PhasePopulation, Owner, true},
		{"POPULATION_INTERNAL_REVIEW", PhasePopulation, Compliance, true},
		{"POPULATION_UNDER_VALIDATION", PhasePopulation, Auditor, true},
		{"POPULATION_COMPLETE", PhasePopulation, Auditor, true},
		{"AWAITING_SAMPLE", PhasePopulation, Auditor, true},
		{"SUBMITTED_SAMPLE", PhaseOEEvidence, Owner, true},
		{"EVIDENCE_PENDING", PhaseOEEvidence, Owner, true},
		{"EVIDENCE_NEED_CLARIFICATION", PhaseOEEvidence, Owner, true},
		{"EVIDENCE_INTERNAL_REVIEW", PhaseOEEvidence, Compliance, true},
		{"EVIDENCE_UNDER_VALIDATION", PhaseOEEvidence, Auditor, true},
		{"EVIDENCE_PENDING", PhaseEvidence, Owner, false},
		{"EVIDENCE_UNDER_VALIDATION", PhaseEvidence, Auditor, false},
	}
	for _, tc := range cases {
		phase, handler, ok := StageFor(tc.status, tc.oe)
		if !ok || phase != tc.phase || handler != tc.handler {
			t.Errorf("%s (oe=%v): got %s/%s/%v, want %s/%s", tc.status, tc.oe, phase, handler, ok, tc.phase, tc.handler)
		}
	}
	if _, _, ok := StageFor("COMPLETE", true); ok {
		t.Error("COMPLETE should not be tracked")
	}
}

func TestPlannedDates(t *testing.T) {
	c, a := PlannedDates(PhaseEvidence, day(time.November, 7), PhaseDays{})
	if FormatDay(c) != "2026-11-17" || FormatDay(a) != "2026-11-27" {
		t.Fatalf("got %s %s", FormatDay(c), FormatDay(a))
	}
	c, a = PlannedDates(PhasePopulation, day(time.November, 7), PhaseDays{})
	if FormatDay(c) != "2026-11-10" || FormatDay(a) != "2026-11-13" {
		t.Fatalf("population: got %s %s", FormatDay(c), FormatDay(a))
	}
}

func TestDefaultsPerPhase(t *testing.T) {
	d := PhaseDays{}
	want := map[string][6]int{
		// initial O/C/A, revision O/C/A
		PhaseEvidence:   {30, 10, 10, 15, 5, 5},
		PhasePopulation: {4, 3, 3, 2, 2, 2},
		PhaseOEEvidence: {5, 2, 2, 2, 2, 2},
	}
	for phase, w := range want {
		got := [6]int{
			d.For(phase, Initial, Owner), d.For(phase, Initial, Compliance), d.For(phase, Initial, Auditor),
			d.For(phase, Revision, Owner), d.For(phase, Revision, Compliance), d.For(phase, Revision, Auditor),
		}
		if got != w {
			t.Errorf("%s: got %v want %v", phase, got, w)
		}
	}
}

func TestDaysFallBackToDefaults(t *testing.T) {
	d := PhaseDays{}
	d.Set(PhasePopulation, Initial, Compliance, 7)
	if d.For(PhasePopulation, Initial, Compliance) != 7 || d.For(PhasePopulation, Initial, Auditor) != 3 {
		t.Fatal("custom value or fallback wrong")
	}
	if d.For(PhaseEvidence, Initial, Compliance) != 10 {
		t.Fatal("a value saved for one phase must not leak into another")
	}
}

func held(handlers ...string) map[string]bool {
	m := map[string]bool{}
	for _, h := range handlers {
		m[HeldKey(PhaseEvidence, h)] = true
	}
	return m
}

func TestPlanTransition(t *testing.T) {
	open := func(h, cycle string) *Stage { return &Stage{Phase: PhaseEvidence, HandledBy: h, CycleType: cycle} }
	now := at(time.November, 6, 10, 0)
	cases := []struct {
		name      string
		in        TransitionInput
		wantClose bool
		wantOpen  *NewStage
	}{
		{
			name:      "owner submits, initial cycle: GRC gets 10 days from submit day",
			in:        TransitionInput{From: "EVIDENCE_PENDING", To: "EVIDENCE_INTERNAL_REVIEW", OpenStage: open(Owner, Initial), HeldBefore: held(Owner)},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Compliance, CycleType: Initial, StartedAt: now, DueDate: day(time.November, 16)},
		},
		{
			name:      "GRC approves: EY gets 10 days",
			in:        TransitionInput{From: "EVIDENCE_INTERNAL_REVIEW", To: "EVIDENCE_UNDER_VALIDATION", OpenStage: open(Compliance, Initial), HeldBefore: held(Owner, Compliance)},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Auditor, CycleType: Initial, StartedAt: now, DueDate: day(time.November, 16)},
		},
		{
			name:      "EY completes: stage closes, nothing opens",
			in:        TransitionInput{From: "EVIDENCE_UNDER_VALIDATION", To: "COMPLETE", OpenStage: open(Auditor, Initial), HeldBefore: held(Owner, Compliance, Auditor)},
			wantClose: true,
		},
		{
			name:      "GRC rejects: owner gets 15 days, revision",
			in:        TransitionInput{From: "EVIDENCE_INTERNAL_REVIEW", To: "EVIDENCE_PENDING", OpenStage: open(Compliance, Initial), HeldBefore: held(Owner, Compliance)},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Owner, CycleType: Revision, StartedAt: now, DueDate: day(time.November, 21)},
		},
		{
			name:      "EY rejects: owner gets 15 days, revision",
			in:        TransitionInput{From: "EVIDENCE_UNDER_VALIDATION", To: "EVIDENCE_NEED_CLARIFICATION", OpenStage: open(Auditor, Initial), HeldBefore: held(Owner, Compliance, Auditor)},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Owner, CycleType: Revision, StartedAt: now, DueDate: day(time.November, 21)},
		},
		{
			name:      "owner resubmits after a rejection: GRC gets 5 days (stays revision)",
			in:        TransitionInput{From: "EVIDENCE_NEED_CLARIFICATION", To: "EVIDENCE_INTERNAL_REVIEW", OpenStage: open(Owner, Revision), HeldBefore: held(Owner, Compliance, Auditor)},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Compliance, CycleType: Revision, StartedAt: now, DueDate: day(time.November, 11)},
		},
		{
			name:      "GRC passes it on after a revision: EY has never held it, so 10 initial days",
			in:        TransitionInput{From: "EVIDENCE_INTERNAL_REVIEW", To: "EVIDENCE_UNDER_VALIDATION", OpenStage: open(Compliance, Revision), HeldBefore: held(Owner, Compliance)},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Auditor, CycleType: Initial, StartedAt: now, DueDate: day(time.November, 16)},
		},
		{
			name:      "GRC passes it on again after EY already had it: 5 revision days",
			in:        TransitionInput{From: "EVIDENCE_INTERNAL_REVIEW", To: "EVIDENCE_UNDER_VALIDATION", OpenStage: open(Compliance, Revision), HeldBefore: held(Owner, Compliance, Auditor)},
			wantClose: true,
			wantOpen:  &NewStage{HandledBy: Auditor, CycleType: Revision, StartedAt: now, DueDate: day(time.November, 11)},
		},
		{
			name:      "admin override from COMPLETE opens a revision stage, nothing to close",
			in:        TransitionInput{From: "COMPLETE", To: "EVIDENCE_UNDER_VALIDATION", Override: true, HeldBefore: held(Owner, Compliance, Auditor)},
			wantClose: false,
			wantOpen:  &NewStage{HandledBy: Auditor, CycleType: Revision, StartedAt: now, DueDate: day(time.November, 11)},
		},
		{
			name: "no-op transition changes nothing",
			in:   TransitionInput{From: "EVIDENCE_PENDING", To: "EVIDENCE_PENDING", OpenStage: open(Owner, Initial)},
		},
		{
			name: "same handler already holds it: nothing changes",
			in:   TransitionInput{From: "EVIDENCE_NEED_CLARIFICATION", To: "EVIDENCE_PENDING", OpenStage: open(Owner, Revision), HeldBefore: held(Owner, Compliance, Auditor)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.Now = now
			tc.in.Days = PhaseDays{}
			got := PlanTransition(tc.in)
			if got.CloseOpen != tc.wantClose {
				t.Errorf("CloseOpen = %v, want %v", got.CloseOpen, tc.wantClose)
			}
			switch {
			case tc.wantOpen == nil && got.Open != nil:
				t.Errorf("unexpected open stage %+v", *got.Open)
			case tc.wantOpen != nil && got.Open == nil:
				t.Errorf("expected open stage %+v", *tc.wantOpen)
			case tc.wantOpen != nil:
				if got.Open.HandledBy != tc.wantOpen.HandledBy || got.Open.CycleType != tc.wantOpen.CycleType ||
					!got.Open.DueDate.Equal(tc.wantOpen.DueDate) {
					t.Errorf("open = %+v, want %+v", *got.Open, *tc.wantOpen)
				}
			}
		})
	}
}

// A control that was already waiting on the owner after an auditor rejection when
// tracking began has no stage history, but its resubmission is still a revision.
func TestPlanTransitionResubmitWithoutHistoryIsRevision(t *testing.T) {
	now := at(time.November, 6, 10, 0)
	got := PlanTransition(TransitionInput{
		From: "EVIDENCE_NEED_CLARIFICATION", To: "EVIDENCE_INTERNAL_REVIEW", Now: now, Days: PhaseDays{},
	})
	if got.Open == nil || got.Open.HandledBy != Compliance || got.Open.CycleType != Revision ||
		FormatDay(got.Open.DueDate) != "2026-11-11" {
		t.Fatalf("got %+v", got.Open)
	}
}

// The deadline is counted from the calendar day in Asia/Colombo, not from UTC:
// 23:30 UTC on 9 Nov is already 05:00 on 10 Nov in Sri Lanka.
func TestPlanTransitionUsesColomboDay(t *testing.T) {
	now := time.Date(2026, time.November, 9, 23, 30, 0, 0, time.UTC)
	got := PlanTransition(TransitionInput{
		From: "EVIDENCE_PENDING", To: "EVIDENCE_INTERNAL_REVIEW", Now: now,
		OpenStage: &Stage{Phase: PhaseEvidence, HandledBy: Owner, CycleType: Initial}, HeldBefore: held(Owner), Days: PhaseDays{},
	})
	if got.Open == nil || FormatDay(got.Open.DueDate) != "2026-11-20" {
		t.Fatalf("got %+v", got.Open)
	}
}

func TestRevisionDeadlineMonthBoundary(t *testing.T) {
	got := PlanTransition(TransitionInput{
		From: "EVIDENCE_UNDER_VALIDATION", To: "EVIDENCE_NEED_CLARIFICATION", Now: at(time.November, 24, 12, 0),
		OpenStage: &Stage{Phase: PhaseEvidence, HandledBy: Auditor, CycleType: Initial}, HeldBefore: held(Owner, Compliance, Auditor), Days: PhaseDays{},
	})
	if FormatDay(got.Open.DueDate) != "2026-12-09" {
		t.Fatalf("24 Nov + 15 days should be 9 Dec, got %s", FormatDay(got.Open.DueDate))
	}
}

// Example A: nobody is late, the control completes before the original EY date.
func TestEvaluateBestCase(t *testing.T) {
	stages := []Stage{
		{ID: 1, HandledBy: Owner, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 7),
			PlannedComplianceDate: ptr(day(time.November, 17)), PlannedAuditorDate: ptr(day(time.November, 27)), EndedAt: ptr(at(time.November, 6, 10, 0))},
		{ID: 2, HandledBy: Compliance, CycleType: Initial, StartedAt: at(time.November, 6, 10, 0), DueDate: day(time.November, 16), EndedAt: ptr(at(time.November, 14, 16, 0))},
		{ID: 3, HandledBy: Auditor, CycleType: Initial, StartedAt: at(time.November, 14, 16, 0), DueDate: day(time.November, 24), EndedAt: ptr(at(time.November, 24, 12, 0))},
	}
	ev := Evaluate(stages, at(time.December, 1, 9, 0))
	if ev.Delayed || len(ev.DelayLog) != 0 {
		t.Fatalf("expected no delay, got delayed=%v log=%v", ev.Delayed, logString(ev))
	}
	if ev.CurrentHandler != nil || ev.CurrentDueDate != nil {
		t.Fatal("a completed control has no current handler")
	}
	if FormatDay(*ev.PlannedOwnerDate) != "2026-11-07" || FormatDay(*ev.PlannedComplianceDate) != "2026-11-17" || FormatDay(*ev.PlannedAuditorDate) != "2026-11-27" {
		t.Fatal("planned dates wrong")
	}
}

// Example B: late at every step. The report changes as days pass.
func TestEvaluateLateAtEveryStep(t *testing.T) {
	complete := []Stage{
		{ID: 1, HandledBy: Owner, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 7),
			PlannedComplianceDate: ptr(day(time.November, 17)), PlannedAuditorDate: ptr(day(time.November, 27)), EndedAt: ptr(at(time.November, 10, 14, 0))},
		{ID: 2, HandledBy: Compliance, CycleType: Initial, StartedAt: at(time.November, 10, 14, 0), DueDate: day(time.November, 20), EndedAt: ptr(at(time.November, 23, 21, 0))},
		{ID: 3, HandledBy: Auditor, CycleType: Initial, StartedAt: at(time.November, 23, 21, 0), DueDate: day(time.December, 3), EndedAt: ptr(at(time.December, 5, 11, 0))},
	}
	// Truncate the history to what was recorded at a given moment.
	asOf := func(now time.Time) []Stage {
		out := []Stage{}
		for _, s := range complete {
			if s.StartedAt.After(now) {
				continue
			}
			if s.EndedAt != nil && s.EndedAt.After(now) {
				s.EndedAt = nil
			}
			out = append(out, s)
		}
		return out
	}
	cases := []struct {
		name    string
		now     time.Time
		handler string
		due     string
		delayed bool
		log     []string
	}{
		{"5 Nov, on time", at(time.November, 5, 12, 0), Owner, "2026-11-07", false, []string{}},
		{"8 Nov, internal late one day", at(time.November, 8, 12, 0), Owner, "2026-11-07", true, []string{"Internal - 11/08"}},
		{"10 Nov before submit", at(time.November, 10, 9, 0), Owner, "2026-11-07", true, []string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10"}},
		{"11 Nov, with GRC", at(time.November, 11, 9, 0), Compliance, "2026-11-20", true, []string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10"}},
		{"22 Nov, GRC late", at(time.November, 22, 9, 0), Compliance, "2026-11-20", true,
			[]string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10", "GRC - 11/21", "GRC - 11/22"}},
		{"24 Nov, with EY", at(time.November, 24, 9, 0), Auditor, "2026-12-03", true,
			[]string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10", "GRC - 11/21", "GRC - 11/22", "GRC - 11/23"}},
		{"28 Nov, EY on time but earlier groups were late", at(time.November, 28, 9, 0), Auditor, "2026-12-03", true,
			[]string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10", "GRC - 11/21", "GRC - 11/22", "GRC - 11/23"}},
		{"4 Dec, EY late", at(time.December, 4, 9, 0), Auditor, "2026-12-03", true,
			[]string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10", "GRC - 11/21", "GRC - 11/22", "GRC - 11/23", "EY - 12/04"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := Evaluate(asOf(tc.now), tc.now)
			if ev.CurrentHandler == nil || *ev.CurrentHandler != tc.handler || FormatDay(*ev.CurrentDueDate) != tc.due {
				t.Errorf("current = %v %v, want %s %s", ev.CurrentHandler, ev.CurrentDueDate, tc.handler, tc.due)
			}
			if ev.Delayed != tc.delayed {
				t.Errorf("delayed = %v, want %v", ev.Delayed, tc.delayed)
			}
			if got := logString(ev); !equalStrings(got, tc.log) {
				t.Errorf("log = %v, want %v", got, tc.log)
			}
		})
	}

	t.Run("5 Dec approved: complete but stays Delayed, full log", func(t *testing.T) {
		ev := Evaluate(complete, at(time.December, 6, 9, 0))
		want := []string{"Internal - 11/08", "Internal - 11/09", "Internal - 11/10", "GRC - 11/21", "GRC - 11/22", "GRC - 11/23", "EY - 12/04", "EY - 12/05"}
		if got := logString(ev); !equalStrings(got, want) {
			t.Errorf("log = %v, want %v", got, want)
		}
		if !ev.Delayed {
			t.Error("a control where someone was late stays Delayed")
		}
		if ev.CurrentHandler != nil {
			t.Error("a completed control has no current handler")
		}
	})
}

// Example C: a rejection with nobody late logs nothing and is not Delayed.
func TestEvaluateRejectionWithoutLateness(t *testing.T) {
	stages := []Stage{
		{ID: 1, HandledBy: Owner, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 7),
			PlannedComplianceDate: ptr(day(time.November, 17)), PlannedAuditorDate: ptr(day(time.November, 27)), EndedAt: ptr(at(time.November, 6, 10, 0))},
		{ID: 2, HandledBy: Compliance, CycleType: Initial, StartedAt: at(time.November, 6, 10, 0), DueDate: day(time.November, 16), EndedAt: ptr(at(time.November, 14, 16, 0))},
		{ID: 3, HandledBy: Auditor, CycleType: Initial, StartedAt: at(time.November, 14, 16, 0), DueDate: day(time.November, 24), EndedAt: ptr(at(time.November, 24, 12, 0))},
		{ID: 4, HandledBy: Owner, CycleType: Revision, StartedAt: at(time.November, 24, 12, 0), DueDate: day(time.December, 9)},
	}
	ev := Evaluate(stages, at(time.December, 1, 9, 0))
	if ev.Delayed || len(ev.DelayLog) != 0 {
		t.Fatalf("expected no delay, got delayed=%v log=%v", ev.Delayed, logString(ev))
	}
	if ev.CurrentHandler == nil || *ev.CurrentHandler != Owner || FormatDay(*ev.CurrentDueDate) != "2026-12-09" {
		t.Fatalf("current = %v %v", ev.CurrentHandler, ev.CurrentDueDate)
	}

	// Once 9 Dec passes with no submission, the revision stage is late and the control is Delayed.
	late := Evaluate(stages, at(time.December, 11, 9, 0))
	if !late.Delayed {
		t.Fatal("a missed revision deadline should set Delayed")
	}
	if got := logString(late); !equalStrings(got, []string{"Internal - 12/10", "Internal - 12/11"}) {
		t.Fatalf("log = %v", got)
	}
}

func TestEvaluateBoundaries(t *testing.T) {
	mk := func(end time.Time) []Stage {
		return []Stage{{ID: 1, HandledBy: Owner, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 7), EndedAt: &end}}
	}
	if ev := Evaluate(mk(at(time.November, 7, 23, 59)), at(time.November, 8, 9, 0)); len(ev.DelayLog) != 0 {
		t.Errorf("handed on at 23:59 on the due day is on time, got %v", logString(ev))
	}
	if ev := Evaluate(mk(at(time.November, 8, 0, 1)), at(time.November, 8, 9, 0)); len(ev.DelayLog) != 1 {
		t.Errorf("handed on just after midnight is one late day, got %v", logString(ev))
	}
}

func TestEvaluateNoStages(t *testing.T) {
	ev := Evaluate(nil, time.Now())
	if ev.CurrentHandler != nil || ev.Delayed || ev.DelayLog != nil {
		t.Fatal("no stages means nothing to report")
	}
}

// A control created before this feature (lazily tracked) has no planned dates;
// lateness is still logged and sets Delayed.
func TestEvaluateWithoutPlannedDates(t *testing.T) {
	stages := []Stage{{ID: 1, HandledBy: Compliance, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 11)}}
	ev := Evaluate(stages, at(time.December, 20, 9, 0))
	if !ev.Delayed || len(ev.DelayLog) == 0 {
		t.Fatal("lateness is logged and sets Delayed even without planned dates")
	}
}

// ---- OE (population + evidence) ----

// oeWalk applies a status path to an OE control the way the repository does:
// every stage ever opened counts as held, and the open stage is the last one.
type oeWalk struct {
	t      *testing.T
	stages []Stage
	held   map[string]bool
	status string
	now    time.Time
	days   PhaseDays
}

func newOEWalk(t *testing.T, start time.Time) *oeWalk {
	w := &oeWalk{t: t, held: map[string]bool{}, status: "POPULATION_PENDING", now: start, days: PhaseDays{}}
	first := FirstStage(PhasePopulation, AddDays(DayOf(start), w.days.For(PhasePopulation, Initial, Owner)), start)
	w.stages = append(w.stages, Stage{ID: 1, Phase: first.Phase, HandledBy: first.HandledBy, CycleType: first.CycleType,
		StartedAt: first.StartedAt, DueDate: first.DueDate})
	w.held[HeldKey(first.Phase, first.HandledBy)] = true
	return w
}

// to moves the control to status after daysLater days and returns the stage it opened, if any.
func (w *oeWalk) to(status string, daysLater int) *Stage {
	w.now = w.now.AddDate(0, 0, daysLater)
	var open *Stage
	if n := len(w.stages); n > 0 && w.stages[n-1].EndedAt == nil {
		open = &w.stages[n-1]
	}
	ch := PlanTransition(TransitionInput{From: w.status, To: status, OE: true, Now: w.now, OpenStage: open, HeldBefore: w.held, Days: w.days})
	w.status = status
	if ch.CloseOpen && open != nil {
		end := w.now
		open.EndedAt = &end
	}
	if ch.Open == nil {
		return nil
	}
	s := Stage{ID: int64(len(w.stages) + 1), Phase: ch.Open.Phase, HandledBy: ch.Open.HandledBy, CycleType: ch.Open.CycleType,
		StartedAt: ch.Open.StartedAt, DueDate: ch.Open.DueDate,
		PlannedComplianceDate: ch.Open.PlannedComplianceDate, PlannedAuditorDate: ch.Open.PlannedAuditorDate}
	w.stages = append(w.stages, s)
	w.held[HeldKey(s.Phase, s.HandledBy)] = true
	return &w.stages[len(w.stages)-1]
}

func (w *oeWalk) expect(status string, daysLater int, phase, handler, cycle string, allowed int) {
	w.t.Helper()
	before := w.now.AddDate(0, 0, daysLater)
	s := w.to(status, daysLater)
	if s == nil {
		w.t.Fatalf("%s: expected a new stage", status)
	}
	if s.Phase != phase || s.HandledBy != handler || s.CycleType != cycle {
		w.t.Fatalf("%s: got %s/%s/%s, want %s/%s/%s", status, s.Phase, s.HandledBy, s.CycleType, phase, handler, cycle)
	}
	if want := AddDays(DayOf(before), allowed); !s.DueDate.Equal(want) {
		w.t.Fatalf("%s: due %s, want %s (%d days)", status, FormatDay(s.DueDate), FormatDay(want), allowed)
	}
}

// The example: Owner 4, GRC 3, EY 3, then evidence: Owner 5, GRC 2, EY 2.
func TestOEWalkInitialEverywhere(t *testing.T) {
	w := newOEWalk(t, at(time.November, 1, 9, 0))
	w.expect("POPULATION_INTERNAL_REVIEW", 1, PhasePopulation, Compliance, Initial, 3)
	w.expect("POPULATION_UNDER_VALIDATION", 1, PhasePopulation, Auditor, Initial, 3)
	// The same EY window carries on through completing and the sample.
	if s := w.to("POPULATION_COMPLETE", 1); s != nil {
		t.Fatalf("EY keeps one window through POPULATION_COMPLETE, got %+v", *s)
	}
	if s := w.to("AWAITING_SAMPLE", 1); s != nil {
		t.Fatalf("EY keeps one window through AWAITING_SAMPLE, got %+v", *s)
	}
	// EY submits the sample: the Owner has 5 initial days, though they held the population.
	w.expect("SUBMITTED_SAMPLE", 1, PhaseOEEvidence, Owner, Initial, 5)
	if s := w.to("EVIDENCE_PENDING", 0); s != nil {
		t.Fatalf("owner already holds it, got %+v", *s)
	}
	w.expect("EVIDENCE_INTERNAL_REVIEW", 1, PhaseOEEvidence, Compliance, Initial, 2)
	w.expect("EVIDENCE_UNDER_VALIDATION", 1, PhaseOEEvidence, Auditor, Initial, 2)
	w.to("COMPLETE", 1)
	if last := w.stages[len(w.stages)-1]; last.EndedAt == nil {
		t.Fatal("COMPLETE closes the open stage")
	}
}

// Owner -> GRC rejects -> Owner -> GRC -> EY: EY has never held it, so initial 3.
func TestOEPopulationRejectionThenFirstEYIsInitial(t *testing.T) {
	w := newOEWalk(t, at(time.November, 1, 9, 0))
	w.expect("POPULATION_INTERNAL_REVIEW", 1, PhasePopulation, Compliance, Initial, 3)
	w.expect("POPULATION_PENDING", 1, PhasePopulation, Owner, Revision, 2)
	w.expect("POPULATION_INTERNAL_REVIEW", 1, PhasePopulation, Compliance, Revision, 2)
	w.expect("POPULATION_UNDER_VALIDATION", 1, PhasePopulation, Auditor, Initial, 3)
	// EY rejects: Owner revision, GRC revision, EY revision.
	w.expect("POPULATION_NEED_CLARIFICATION", 1, PhasePopulation, Owner, Revision, 2)
	w.expect("POPULATION_INTERNAL_REVIEW", 1, PhasePopulation, Compliance, Revision, 2)
	w.expect("POPULATION_UNDER_VALIDATION", 1, PhasePopulation, Auditor, Revision, 2)
}

// The count starts again in evidence, and EY rejecting evidence is a revision there.
func TestOEEvidencePhaseStartsFresh(t *testing.T) {
	w := newOEWalk(t, at(time.November, 1, 9, 0))
	w.to("POPULATION_INTERNAL_REVIEW", 1)
	w.to("POPULATION_PENDING", 1) // rejected once in population
	w.to("POPULATION_INTERNAL_REVIEW", 1)
	w.to("POPULATION_UNDER_VALIDATION", 1)
	w.to("POPULATION_COMPLETE", 1)
	// Skipping the sample step goes straight to evidence.
	w.expect("EVIDENCE_PENDING", 1, PhaseOEEvidence, Owner, Initial, 5)
	w.expect("EVIDENCE_INTERNAL_REVIEW", 1, PhaseOEEvidence, Compliance, Initial, 2)
	w.expect("EVIDENCE_PENDING", 1, PhaseOEEvidence, Owner, Revision, 2)
	w.expect("EVIDENCE_INTERNAL_REVIEW", 1, PhaseOEEvidence, Compliance, Revision, 2)
	w.expect("EVIDENCE_UNDER_VALIDATION", 1, PhaseOEEvidence, Auditor, Initial, 2)
	w.expect("EVIDENCE_NEED_CLARIFICATION", 1, PhaseOEEvidence, Owner, Revision, 2)
}

func TestOEDaysAreOwnPerPhase(t *testing.T) {
	w := newOEWalk(t, at(time.November, 1, 9, 0))
	w.days.Set(PhasePopulation, Initial, Compliance, 8)
	w.days.Set(PhaseEvidence, Initial, Compliance, 20) // Design evidence must not affect OE
	w.expect("POPULATION_INTERNAL_REVIEW", 1, PhasePopulation, Compliance, Initial, 8)
	w.expect("POPULATION_UNDER_VALIDATION", 1, PhasePopulation, Auditor, Initial, 3)
	w.expect("SUBMITTED_SAMPLE", 1, PhaseOEEvidence, Owner, Initial, 5)
	w.expect("EVIDENCE_INTERNAL_REVIEW", 1, PhaseOEEvidence, Compliance, Initial, 2)
}

// Each phase freezes its own plan on the owner's first stage in it.
func TestOEPlansPerPhase(t *testing.T) {
	w := newOEWalk(t, at(time.November, 2, 9, 0))
	w.stages[0].PlannedComplianceDate = ptr(day(time.November, 9))
	w.stages[0].PlannedAuditorDate = ptr(day(time.November, 12))
	w.to("POPULATION_INTERNAL_REVIEW", 1)
	w.to("POPULATION_UNDER_VALIDATION", 1)
	w.to("SUBMITTED_SAMPLE", 1) // 5 Nov
	ev := Evaluate(w.stages, at(time.November, 5, 12, 0))
	if len(ev.PhasePlans) != 2 {
		t.Fatalf("want 2 phase plans, got %d", len(ev.PhasePlans))
	}
	pop, evd := ev.PhasePlans[0], ev.PhasePlans[1]
	if pop.Phase != PhasePopulation || FormatDay(*pop.Owner) != "2026-11-06" || FormatDay(*pop.Compliance) != "2026-11-09" || FormatDay(*pop.Auditor) != "2026-11-12" {
		t.Errorf("population plan wrong: %+v", pop)
	}
	// Evidence plan: Owner 5 days from 5 Nov, GRC 2 more, EY 2 more.
	if evd.Phase != PhaseOEEvidence || FormatDay(*evd.Owner) != "2026-11-10" || FormatDay(*evd.Compliance) != "2026-11-12" || FormatDay(*evd.Auditor) != "2026-11-14" {
		t.Errorf("evidence plan wrong: %s %s %s %s", evd.Phase, FormatDay(*evd.Owner), FormatDay(*evd.Compliance), FormatDay(*evd.Auditor))
	}
	// The flat fields are the first phase's plan.
	if FormatDay(*ev.PlannedOwnerDate) != "2026-11-06" {
		t.Errorf("flat planned owner = %s", FormatDay(*ev.PlannedOwnerDate))
	}
	if ev.CurrentPhase == nil || *ev.CurrentPhase != PhaseOEEvidence {
		t.Errorf("current phase = %v", ev.CurrentPhase)
	}
}

// Delayed means someone held the control past their own deadline, whoever it was.
// A late Owner does not make an on-time EY look late.
func TestDelayedBlamesOnlyTheLateGroup(t *testing.T) {
	stages := []Stage{
		{ID: 1, Phase: PhasePopulation, HandledBy: Owner, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 5),
			PlannedComplianceDate: ptr(day(time.November, 8)), PlannedAuditorDate: ptr(day(time.November, 11)), EndedAt: ptr(at(time.November, 7, 10, 0))},
		{ID: 2, Phase: PhasePopulation, HandledBy: Compliance, CycleType: Initial, StartedAt: at(time.November, 7, 10, 0), DueDate: day(time.November, 10), EndedAt: ptr(at(time.November, 9, 10, 0))},
		{ID: 3, Phase: PhasePopulation, HandledBy: Auditor, CycleType: Initial, StartedAt: at(time.November, 9, 10, 0), DueDate: day(time.November, 12), EndedAt: ptr(at(time.November, 11, 10, 0))},
	}
	ev := Evaluate(stages, at(time.November, 20, 9, 0))
	if !ev.Delayed {
		t.Fatal("the Owner was two days late, so the control is Delayed")
	}
	if got := logString(ev); !equalStrings(got, []string{"Internal - 11/06", "Internal - 11/07"}) {
		t.Fatalf("only the Owner is in the log, got %v", got)
	}
	for _, e := range ev.DelayLog {
		if e.Phase != PhasePopulation {
			t.Errorf("log entry phase = %q", e.Phase)
		}
	}
}

func TestStillDelayedAfterLateOwnerRecovers(t *testing.T) {
	stages := []Stage{
		{ID: 1, Phase: PhaseEvidence, HandledBy: Owner, CycleType: Initial, StartedAt: at(time.November, 1, 9, 0), DueDate: day(time.November, 5),
			EndedAt: ptr(at(time.November, 6, 10, 0))},
		{ID: 2, Phase: PhaseEvidence, HandledBy: Compliance, CycleType: Initial, StartedAt: at(time.November, 6, 10, 0), DueDate: day(time.November, 16)},
	}
	if ev := Evaluate(stages, at(time.November, 8, 9, 0)); !ev.Delayed {
		t.Fatal("one late day in the past still means Delayed")
	}
	if ev := Evaluate(stages[1:], at(time.November, 8, 9, 0)); ev.Delayed {
		t.Fatal("a holder inside their deadline is not Delayed")
	}
}
