package hr

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

func TestCalendarWindowDefaultsAndLimits(t *testing.T) {
	start, end, err := calendarWindow("", "")
	if err != nil {
		t.Fatalf("default window: %v", err)
	}
	if got := int(end.Sub(start).Hours()/24) + 1; got != defaultCalendarDays {
		t.Errorf("default window spans %d days, want %d", got, defaultCalendarDays)
	}

	start, end, err = calendarWindow("2026-07-01", "2026-07-31")
	if err != nil {
		t.Fatalf("month window: %v", err)
	}
	if dateOnly(start) != "2026-07-01" || dateOnly(end) != "2026-07-31" {
		t.Errorf("month window = %s..%s", dateOnly(start), dateOnly(end))
	}

	if _, _, err := calendarWindow("2026-07-10", "2026-07-01"); err == nil {
		t.Error("an end before the start must be rejected")
	}
	if _, _, err := calendarWindow("2026-01-01", "2026-12-31"); err == nil {
		t.Error("a range longer than the cap must be rejected")
	}
	if _, _, err := calendarWindow("01/07/2026", ""); err == nil {
		t.Error("a non ISO date must be rejected")
	}
}

func TestIsScheduledUsesAssignmentThenWeekdayFallback(t *testing.T) {
	day := func(value string) time.Time {
		parsed, _ := time.Parse("2006-01-02", value)
		return parsed
	}
	// 2026-07-20 is a Monday, 2026-07-25 a Saturday.
	if !isScheduled(nil, day("2026-07-20")) {
		t.Error("with no assignment a Monday falls back to scheduled")
	}
	if isScheduled(nil, day("2026-07-25")) {
		t.Error("with no assignment a Saturday falls back to a rest day")
	}

	// A Tuesday-to-Saturday shift running from 2026-07-01 with no end date.
	weekend := []calendarSchedule{{
		StartDate: day("2026-07-01"),
		WorkDays:  json.RawMessage(`[2,3,4,5,6]`),
	}}
	if isScheduled(weekend, day("2026-07-20")) {
		t.Error("Monday is a rest day under a Tuesday–Saturday shift")
	}
	if !isScheduled(weekend, day("2026-07-25")) {
		t.Error("Saturday is a work day under a Tuesday–Saturday shift")
	}

	// An assignment that ended before the day must not apply; the fallback does.
	expired := []calendarSchedule{{
		StartDate: day("2026-07-01"),
		EndDate:   sql.NullTime{Time: day("2026-07-10"), Valid: true},
		WorkDays:  json.RawMessage(`[6,7]`),
	}}
	if !isScheduled(expired, day("2026-07-20")) {
		t.Error("an expired assignment must not decide the day")
	}
}

func TestLeaveOnPrefersApprovedAndReadsPortions(t *testing.T) {
	day := func(value string) time.Time {
		parsed, _ := time.Parse("2006-01-02", value)
		return parsed
	}
	// The repository orders pending first so approved wins on a shared day.
	leaves := []calendarLeave{
		{StartDate: day("2026-07-20"), EndDate: day("2026-07-24"), Status: "pending", StartPortion: "full", EndPortion: "full"},
		{StartDate: day("2026-07-22"), EndDate: day("2026-07-23"), Status: "approved", StartPortion: "half", EndPortion: "full"},
	}
	if leave, ok := leaveOn(leaves, day("2026-07-21")); !ok || leave.Status != "pending" {
		t.Errorf("a day covered only by the pending request stays pending, got %+v (ok=%t)", leave, ok)
	}
	leave, ok := leaveOn(leaves, day("2026-07-22"))
	if !ok || leave.Status != "approved" {
		t.Fatalf("approved leave must win over pending on a shared day, got %+v (ok=%t)", leave, ok)
	}
	if got := leavePortion(leave, day("2026-07-22")); got != "half" {
		t.Errorf("the first day of a half-day request is a half day, got %q", got)
	}
	if got := leavePortion(leave, day("2026-07-23")); got != "full" {
		t.Errorf("a full end day stays full, got %q", got)
	}
	if _, ok := leaveOn(leaves, day("2026-07-27")); ok {
		t.Error("a day outside every request must not be on leave")
	}
}
