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

// PhaseEvidence is the only phase tracked in release 1 (DESIGN controls).
const PhaseEvidence = "EVIDENCE"

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

// Days maps cycle -> handler -> allowed days.
type Days map[string]map[string]int

// DefaultDays are used for every audit that has no saved rows.
func DefaultDays() Days {
	return Days{
		Initial:  {Owner: 30, Compliance: 10, Auditor: 10},
		Revision: {Owner: 15, Compliance: 5, Auditor: 5},
	}
}

// For returns the allowed days for a cycle and handler, falling back to the default.
func (d Days) For(cycle, handler string) int {
	if v, ok := d[cycle][handler]; ok && v > 0 {
		return v
	}
	return DefaultDays()[cycle][handler]
}

// Set records one value, creating the inner map when needed.
func (d Days) Set(cycle, handler string, days int) {
	if d[cycle] == nil {
		d[cycle] = map[string]int{}
	}
	d[cycle][handler] = days
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

// HandlerForStatus maps an evidence-phase status to the group holding the control.
// ok is false for COMPLETE (nobody holds it) and for statuses this feature does
// not track (the OE population and sample statuses).
func HandlerForStatus(status string) (handler string, ok bool) {
	switch status {
	case "EVIDENCE_PENDING", "EVIDENCE_NEED_CLARIFICATION":
		return Owner, true
	case "EVIDENCE_INTERNAL_REVIEW":
		return Compliance, true
	case "EVIDENCE_UNDER_VALIDATION":
		return Auditor, true
	}
	return "", false
}

// IsRejection reports whether from -> to sends a control back to the owner.
func IsRejection(from, to string) bool {
	return (from == "EVIDENCE_INTERNAL_REVIEW" && to == "EVIDENCE_PENDING") ||
		(from == "EVIDENCE_UNDER_VALIDATION" && to == "EVIDENCE_NEED_CLARIFICATION")
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
	HandledBy string
	CycleType string
	StartedAt time.Time
	DueDate   time.Time // a Day
}

// Change is what a status transition does to a control's stages.
type Change struct {
	CloseOpen bool      // close the control's open stage at the transition time
	Open      *NewStage // open this stage, when non-nil
}

// TransitionInput is everything PlanTransition needs.
type TransitionInput struct {
	From, To  string
	Override  bool // an admin backward override rather than the normal workflow
	Now       time.Time
	OpenStage *Stage // the control's open stage, nil when none
	PrevCycle string // cycle of the control's latest stage, "" when it has none
	Days      Days
}

// PlanTransition decides how a status change opens and closes stages.
func PlanTransition(in TransitionInput) Change {
	if in.From == in.To {
		return Change{}
	}
	handler, tracked := HandlerForStatus(in.To)
	if !tracked {
		if in.To == "COMPLETE" {
			return Change{CloseOpen: in.OpenStage != nil}
		}
		return Change{}
	}
	if in.OpenStage != nil && in.OpenStage.HandledBy == handler {
		return Change{}
	}
	// EVIDENCE_NEED_CLARIFICATION is only reached by an auditor rejection, so the
	// way out of it is a revision even for a control that has no stage history.
	cycle := Initial
	if IsRejection(in.From, in.To) || in.From == "EVIDENCE_NEED_CLARIFICATION" || in.Override || in.PrevCycle == Revision {
		cycle = Revision
	}
	return Change{
		CloseOpen: in.OpenStage != nil,
		Open: &NewStage{
			HandledBy: handler,
			CycleType: cycle,
			StartedAt: in.Now,
			DueDate:   AddDays(DayOf(in.Now), in.Days.For(cycle, handler)),
		},
	}
}

// PlannedDates returns the original planned GRC and EY dates for a control whose
// owner committed to ownerDue: GRC is the initial GRC days after that, EY the
// initial EY days after GRC.
func PlannedDates(ownerDue time.Time, days Days) (compliance, auditor time.Time) {
	compliance = AddDays(ownerDue, days.For(Initial, Compliance))
	auditor = AddDays(compliance, days.For(Initial, Auditor))
	return compliance, auditor
}

// FirstStage is the owner's opening stage for a newly created control.
func FirstStage(ownerDue, now time.Time) NewStage {
	return NewStage{HandledBy: Owner, CycleType: Initial, StartedAt: now, DueDate: ownerDue}
}

// LogEntry is one late day for one group.
type LogEntry struct {
	HandledBy string
	Date      time.Time // a Day
}

// Evaluation is what a report shows for one control.
type Evaluation struct {
	CurrentHandler        *string
	CurrentDueDate        *time.Time
	PlannedOwnerDate      *time.Time
	PlannedComplianceDate *time.Time
	PlannedAuditorDate    *time.Time
	Delayed               bool
	DelayLog              []LogEntry
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

	first := sorted[0]
	if first.HandledBy == Owner && first.CycleType == Initial {
		d := first.DueDate
		ev.PlannedOwnerDate = &d
	}
	ev.PlannedComplianceDate = first.PlannedComplianceDate
	ev.PlannedAuditorDate = first.PlannedAuditorDate

	revisionLate := false
	var lastInitialEnd *time.Time
	for _, s := range sorted {
		end := today
		if s.EndedAt != nil {
			end = DayOf(*s.EndedAt)
		}
		if s.EndedAt == nil {
			h, d := s.HandledBy, s.DueDate
			ev.CurrentHandler = &h
			ev.CurrentDueDate = &d
		}
		if s.CycleType == Initial {
			e := end
			lastInitialEnd = &e
		}
		d := AddDays(s.DueDate, 1)
		for i := 0; i < maxLogDays && !d.After(end); i++ {
			ev.DelayLog = append(ev.DelayLog, LogEntry{HandledBy: s.HandledBy, Date: d})
			if s.CycleType == Revision {
				revisionLate = true
			}
			d = AddDays(d, 1)
		}
	}

	initialLate := ev.PlannedAuditorDate != nil && lastInitialEnd != nil &&
		lastInitialEnd.After(*ev.PlannedAuditorDate)
	ev.Delayed = initialLate || revisionLate
	return ev
}
