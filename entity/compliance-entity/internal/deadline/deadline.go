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

// Package deadline holds the pure rules for workflow-aware control deadlines:
// who holds a control for a given status, how long they are allowed to hold it,
// and how the current holder, Delayed flag and delay log are worked out from a
// control's recorded hand-offs (audit_control_stage rows). It has no database
// access, so every rule here can be tested with plain values.
package deadline

import (
	"sort"
	"time"
)

// Handlers: the group that holds a control during one stage.
const (
	Owner      = "OWNER"      // the owning (internal) team that submits evidence
	Compliance = "COMPLIANCE" // GRC, reviews the submission
	Auditor    = "AUDITOR"    // EY / external auditor, validates it
)

// Cycles: a control's first pass is INITIAL; any pass after a rejection is REVISION.
const (
	Initial  = "INITIAL"
	Revision = "REVISION"
)

// Phases. A DESIGN control only has EVIDENCE. An OE control has POPULATION and
// then OE_EVIDENCE; OE evidence keeps its own days, apart from Design evidence.
const (
	PhaseEvidence   = "EVIDENCE"
	PhasePopulation = "POPULATION"
	PhaseOEEvidence = "OE_EVIDENCE"
)

// Phases lists every phase, in the order they are shown.
func Phases() []string { return []string{PhaseEvidence, PhasePopulation, PhaseOEEvidence} }

// ValidPhase reports whether p is a known phase.
func ValidPhase(p string) bool {
	return p == PhaseEvidence || p == PhasePopulation || p == PhaseOEEvidence
}

// MaxAllowedDays is the upper limit an admin may set for any allowed-days value.
const MaxAllowedDays = 365

// maxLogDays bounds the delay-log loop so corrupt dates can never spin forever.
const maxLogDays = 3650

// Location is the one fixed timezone every deadline day is counted in.
var Location = loadLocation()

func loadLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Colombo"); err == nil {
		return loc
	}
	// tzdata may be missing in a slim container image; Sri Lanka is a fixed UTC+5:30.
	return time.FixedZone("Asia/Colombo", 5*3600+30*60)
}

// Days maps cycle -> handler -> allowed days, for one phase.
type Days map[string]map[string]int

// DefaultDaysFor are the days a phase uses for every audit that has no saved rows.
func DefaultDaysFor(phase string) Days {
	switch phase {
	case PhasePopulation:
		return Days{
			Initial:  {Owner: 4, Compliance: 3, Auditor: 3},
			Revision: {Owner: 2, Compliance: 2, Auditor: 2},
		}
	case PhaseOEEvidence:
		return Days{
			Initial:  {Owner: 5, Compliance: 2, Auditor: 2},
			Revision: {Owner: 2, Compliance: 2, Auditor: 2},
		}
	}
	return Days{
		Initial:  {Owner: 30, Compliance: 10, Auditor: 10},
		Revision: {Owner: 15, Compliance: 5, Auditor: 5},
	}
}

// Set records one value, creating the inner map when needed.
func (d Days) Set(cycle, handler string, days int) {
	if d[cycle] == nil {
		d[cycle] = map[string]int{}
	}
	d[cycle][handler] = days
}

// PhaseDays holds an audit's saved days for every phase. Anything not saved
// falls back to that phase's defaults.
type PhaseDays map[string]Days

// For returns the allowed days for a phase, cycle and handler.
func (p PhaseDays) For(phase, cycle, handler string) int {
	if v, ok := p[phase][cycle][handler]; ok && v > 0 {
		return v
	}
	return DefaultDaysFor(phase)[cycle][handler]
}

// Set records one value for a phase.
func (p PhaseDays) Set(phase, cycle, handler string, days int) {
	if p[phase] == nil {
		p[phase] = Days{}
	}
	p[phase].Set(cycle, handler, days)
}

// ValidCycle reports whether c is INITIAL or REVISION.
func ValidCycle(c string) bool { return c == Initial || c == Revision }

// ValidHandler reports whether h is OWNER, COMPLIANCE or AUDITOR.
func ValidHandler(h string) bool { return h == Owner || h == Compliance || h == Auditor }

// DayOf returns the calendar day (midnight, Location) that t falls on.
func DayOf(t time.Time) time.Time {
	t = t.In(Location)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Location)
}

// DayFromDB converts a DATE read from MySQL (midnight UTC with parseTime) to a Day.
func DayFromDB(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Location)
}

// ParseDay parses a YYYY-MM-DD string into a Day.
func ParseDay(s string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", s, Location)
	if err != nil {
		return time.Time{}, err
	}
	return DayOf(t), nil
}

// FormatDay renders a Day as YYYY-MM-DD.
func FormatDay(d time.Time) string { return d.Format("2006-01-02") }

// AddDays returns d moved by n calendar days.
func AddDays(d time.Time, n int) time.Time { return d.AddDate(0, 0, n) }

// StageFor maps a status to the phase it belongs to and the group holding the
// control. oe says whether the control is an OE control, which decides whether
// the evidence statuses belong to OE_EVIDENCE or to Design's EVIDENCE. ok is
// false for COMPLETE (nobody holds it) and for any untracked status.
//
// For OE the Auditor holds the control from POPULATION_UNDER_VALIDATION until
// the sample is submitted, and the Owner holds it from SUBMITTED_SAMPLE.
func StageFor(status string, oe bool) (phase, handler string, ok bool) {
	switch status {
	case "POPULATION_PENDING", "POPULATION_NEED_CLARIFICATION":
		return PhasePopulation, Owner, true
	case "POPULATION_INTERNAL_REVIEW":
		return PhasePopulation, Compliance, true
	case "POPULATION_UNDER_VALIDATION", "POPULATION_COMPLETE", "AWAITING_SAMPLE":
		return PhasePopulation, Auditor, true
	}
	evidence := PhaseEvidence
	if oe {
		evidence = PhaseOEEvidence
	}
	switch status {
	case "SUBMITTED_SAMPLE":
		return PhaseOEEvidence, Owner, true
	case "EVIDENCE_PENDING", "EVIDENCE_NEED_CLARIFICATION":
		return evidence, Owner, true
	case "EVIDENCE_INTERNAL_REVIEW":
		return evidence, Compliance, true
	case "EVIDENCE_UNDER_VALIDATION":
		return evidence, Auditor, true
	}
	return "", "", false
}

// HeldKey is the HeldBefore key for a group in a phase: a group that held the
// control in one phase is still new to it in the next.
func HeldKey(phase, handler string) string { return phase + ":" + handler }

// IsRejection reports whether from -> to sends a control back to the owner.
func IsRejection(from, to string) bool {
	return (from == "EVIDENCE_INTERNAL_REVIEW" && to == "EVIDENCE_PENDING") ||
		(from == "EVIDENCE_UNDER_VALIDATION" && to == "EVIDENCE_NEED_CLARIFICATION") ||
		(from == "POPULATION_INTERNAL_REVIEW" && to == "POPULATION_PENDING") ||
		(from == "POPULATION_UNDER_VALIDATION" && to == "POPULATION_NEED_CLARIFICATION")
}

// Stage is one period during which one group held one control.
type Stage struct {
	ID                    int64
	ControlID             int
	Phase                 string
	HandledBy             string
	CycleType             string
	StartedAt             time.Time
	DueDate               time.Time // a Day
	PlannedComplianceDate *time.Time
	PlannedAuditorDate    *time.Time
	EndedAt               *time.Time
	UpdatedBy             *string
}

// NewStage describes a stage about to be opened.
type NewStage struct {
	Phase     string
	HandledBy string
	CycleType string
	StartedAt time.Time
	DueDate   time.Time // a Day
	// The original plan for GRC and EY, set only on a phase's first Owner stage.
	PlannedComplianceDate *time.Time
	PlannedAuditorDate    *time.Time
}

// Change is what a status transition does to a control's stages.
type Change struct {
	CloseOpen bool      // close the control's open stage at the transition time
	Open      *NewStage // open this stage, when non-nil
}

// TransitionInput is everything PlanTransition needs.
type TransitionInput struct {
	From, To   string
	OE         bool // the control is an OE control
	Override   bool // an admin backward override rather than the normal workflow
	Now        time.Time
	OpenStage  *Stage          // the control's open stage, nil when none
	HeldBefore map[string]bool // HeldKey of every group that held this control in an earlier stage
	Days       PhaseDays
}

// PlanTransition decides how a status change opens and closes stages.
func PlanTransition(in TransitionInput) Change {
	if in.From == in.To {
		return Change{}
	}
	phase, handler, tracked := StageFor(in.To, in.OE)
	if !tracked {
		if in.To == "COMPLETE" {
			return Change{CloseOpen: in.OpenStage != nil}
		}
		return Change{}
	}
	if in.OpenStage != nil && in.OpenStage.HandledBy == handler && in.OpenStage.Phase == phase {
		return Change{}
	}
	// The cycle depends on the group and the phase, not on the control: a group
	// that is getting the control for the first time in a phase has the initial
	// days, even when others already went through a revision, or when it held the
	// control in an earlier phase. One that held it before in this phase has the
	// revision days. A NEED_CLARIFICATION status is only reached by an auditor
	// rejection, so the way out of it is a revision even for a control with no
	// stage history.
	cycle := Initial
	if in.HeldBefore[HeldKey(phase, handler)] || IsRejection(in.From, in.To) ||
		in.From == "EVIDENCE_NEED_CLARIFICATION" || in.From == "POPULATION_NEED_CLARIFICATION" || in.Override {
		cycle = Revision
	}
	ns := &NewStage{
		Phase:     phase,
		HandledBy: handler,
		CycleType: cycle,
		StartedAt: in.Now,
		DueDate:   AddDays(DayOf(in.Now), in.Days.For(phase, cycle, handler)),
	}
	// An owner's first stage in a phase is where that phase's plan is fixed.
	if handler == Owner && cycle == Initial {
		c, a := PlannedDates(phase, ns.DueDate, in.Days)
		ns.PlannedComplianceDate, ns.PlannedAuditorDate = &c, &a
	}
	return Change{CloseOpen: in.OpenStage != nil, Open: ns}
}

// PlannedDates returns the original planned GRC and EY dates for a phase whose
// owner committed to ownerDue: GRC is the initial GRC days after that, EY the
// initial EY days after GRC.
func PlannedDates(phase string, ownerDue time.Time, days PhaseDays) (compliance, auditor time.Time) {
	compliance = AddDays(ownerDue, days.For(phase, Initial, Compliance))
	auditor = AddDays(compliance, days.For(phase, Initial, Auditor))
	return compliance, auditor
}

// FirstPhase is the phase a new control starts in.
func FirstPhase(oe bool) string {
	if oe {
		return PhasePopulation
	}
	return PhaseEvidence
}

// FirstStage is the owner's opening stage for a newly created control.
func FirstStage(phase string, ownerDue, now time.Time) NewStage {
	return NewStage{Phase: phase, HandledBy: Owner, CycleType: Initial, StartedAt: now, DueDate: ownerDue}
}

// UltimateDelayed reports whether a control missed its ultimate deadline (the
// control's own due date). finishedAt is when it was completed, nil while it is
// still open, in which case today is what is compared.
func UltimateDelayed(ultimate, now time.Time, finishedAt *time.Time) bool {
	end := DayOf(now)
	if finishedAt != nil {
		end = DayOf(*finishedAt)
	}
	return end.After(DayOf(ultimate))
}

// LogEntry is one late day for one group.
type LogEntry struct {
	Phase     string
	HandledBy string
	Date      time.Time // a Day
}

// PhasePlan is the original plan for one phase: the date each group was first
// expected to hand the control on.
type PhasePlan struct {
	Phase      string
	Owner      *time.Time
	Compliance *time.Time
	Auditor    *time.Time
}

// Evaluation is what a report shows for one control.
type Evaluation struct {
	CurrentHandler *string
	CurrentDueDate *time.Time
	CurrentPhase   *string
	// The planned dates of the control's first phase, kept flat for callers that
	// predate OE; PhasePlans has every phase.
	PlannedOwnerDate      *time.Time
	PlannedComplianceDate *time.Time
	PlannedAuditorDate    *time.Time
	PhasePlans            []PhasePlan
	// Delayed is true when any group held the control past its own deadline,
	// which is the same as the delay log not being empty.
	Delayed  bool
	DelayLog []LogEntry
}

// Evaluate works out the current holder, planned dates, Delayed flag and delay
// log from a control's stages as of now. Nothing here is stored.
func Evaluate(stages []Stage, now time.Time) Evaluation {
	var ev Evaluation
	if len(stages) == 0 {
		return ev
	}
	sorted := append([]Stage(nil), stages...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].StartedAt.Equal(sorted[j].StartedAt) {
			return sorted[i].StartedAt.Before(sorted[j].StartedAt)
		}
		return sorted[i].ID < sorted[j].ID
	})
	today := DayOf(now)

	// One plan per phase, from that phase's first owner stage.
	planned := map[string]bool{}
	for _, s := range sorted {
		if s.HandledBy != Owner || s.CycleType != Initial || planned[s.Phase] {
			continue
		}
		planned[s.Phase] = true
		d := s.DueDate
		ev.PhasePlans = append(ev.PhasePlans, PhasePlan{
			Phase: s.Phase, Owner: &d, Compliance: s.PlannedComplianceDate, Auditor: s.PlannedAuditorDate,
		})
	}
	if len(ev.PhasePlans) > 0 {
		p := ev.PhasePlans[0]
		ev.PlannedOwnerDate, ev.PlannedComplianceDate, ev.PlannedAuditorDate = p.Owner, p.Compliance, p.Auditor
	}
	// A control that predates tracking can start with a stage that is not the
	// owner's first: its planned dates, if it has any, still belong to the plan.
	if len(ev.PhasePlans) == 0 {
		first := sorted[0]
		ev.PlannedComplianceDate = first.PlannedComplianceDate
		ev.PlannedAuditorDate = first.PlannedAuditorDate
	}

	for _, s := range sorted {
		end := today
		if s.EndedAt != nil {
			end = DayOf(*s.EndedAt)
		}
		if s.EndedAt == nil {
			h, d, p := s.HandledBy, s.DueDate, s.Phase
			ev.CurrentHandler = &h
			ev.CurrentDueDate = &d
			ev.CurrentPhase = &p
		}
		d := AddDays(s.DueDate, 1)
		for i := 0; i < maxLogDays && !d.After(end); i++ {
			ev.DelayLog = append(ev.DelayLog, LogEntry{Phase: s.Phase, HandledBy: s.HandledBy, Date: d})
			d = AddDays(d, 1)
		}
	}
	ev.Delayed = len(ev.DelayLog) > 0
	return ev
}
