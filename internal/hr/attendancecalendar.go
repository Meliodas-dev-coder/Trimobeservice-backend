package hr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/trimo/backend/internal/httpx"
)

// The attendance grid reads people first and records second: a row exists for
// every employee in scope, so an absence with no record at all is visible as a
// gap rather than missing from the page. maxCalendarDays caps one request — the
// UI offers a fortnight and a month, and a rolled month is at most 31 days.
const (
	maxCalendarDays     = 62
	defaultCalendarDays = 14
)

type calendarEmployee struct {
	ID             int64          `db:"id"`
	FullName       string         `db:"full_name"`
	PhotoURL       sql.NullString `db:"photo_url"`
	PositionTitle  sql.NullString `db:"position_title"`
	DepartmentID   sql.NullInt64  `db:"department_id"`
	DepartmentName sql.NullString `db:"department_name"`
}

type calendarRecord struct {
	ID              int64          `db:"id"`
	EmployeeID      int64          `db:"employee_id"`
	Date            time.Time      `db:"attendance_date"`
	Status          string         `db:"status"`
	ClockIn         sql.NullTime   `db:"clock_in"`
	ClockOut        sql.NullTime   `db:"clock_out"`
	WorkedMinutes   int            `db:"worked_minutes"`
	LateMinutes     int            `db:"late_minutes"`
	OvertimeMinutes int            `db:"overtime_minutes"`
	ShiftName       sql.NullString `db:"shift_name"`
}

type calendarSchedule struct {
	EmployeeID int64           `db:"employee_id"`
	StartDate  time.Time       `db:"start_date"`
	EndDate    sql.NullTime    `db:"end_date"`
	WorkDays   json.RawMessage `db:"work_days"`
}

type calendarLeave struct {
	EmployeeID   int64          `db:"employee_id"`
	StartDate    time.Time      `db:"start_date"`
	EndDate      time.Time      `db:"end_date"`
	Status       string         `db:"status"`
	StartPortion string         `db:"start_portion"`
	EndPortion   string         `db:"end_portion"`
	PolicyName   sql.NullString `db:"policy_name"`
}

// AttendanceCalendarEmployees lists the people the grid draws rows for. Only the
// employees the caller may read are returned; offboarded staff are dropped since
// the grid is an operational view of the current workforce.
func (r *Repository) AttendanceCalendarEmployees(ctx context.Context, departmentID *int64, opts ListOptions) ([]calendarEmployee, error) {
	where := []string{"e.employment_status <> 'offboarded'"}
	args := []any{}
	if departmentID != nil {
		where = append(where, "e.department_id = ?")
		args = append(args, *departmentID)
	}
	if opts.RestrictEmployee {
		if len(opts.AllowedEmployeeIDs) == 0 {
			return []calendarEmployee{}, nil
		}
		query, inArgs, err := sqlx.In("e.id IN (?)", opts.AllowedEmployeeIDs)
		if err != nil {
			return nil, err
		}
		where = append(where, query)
		args = append(args, inArgs...)
	}
	query := `SELECT e.id, CONCAT(e.first_name, ' ', e.last_name) AS full_name, e.photo_url, e.department_id,
		(SELECT d.name FROM hr_departments d WHERE d.id = e.department_id) AS department_name,
		(SELECT p.title FROM hr_positions p WHERE p.id = e.position_id) AS position_title
		FROM hr_employees e WHERE ` + strings.Join(where, " AND ") + ` ORDER BY e.last_name, e.first_name`
	employees := []calendarEmployee{}
	if err := r.db.SelectContext(ctx, &employees, query, args...); err != nil {
		return nil, mapDBError(err)
	}
	return employees, nil
}

// AttendanceCalendarRecords loads the attendance rows for the given employees
// over the window. Employee IDs come from the already-authorized row set, so no
// further scope filtering is needed here.
func (r *Repository) AttendanceCalendarRecords(ctx context.Context, ids []int64, start, end time.Time) ([]calendarRecord, error) {
	if len(ids) == 0 {
		return []calendarRecord{}, nil
	}
	query, args, err := sqlx.In(`SELECT a.id, a.employee_id, a.attendance_date, a.status, a.clock_in, a.clock_out,
		a.worked_minutes, a.late_minutes, a.overtime_minutes,
		(SELECT s.name FROM hr_shifts s WHERE s.id = a.shift_id) AS shift_name
		FROM hr_attendance a WHERE a.employee_id IN (?) AND a.attendance_date BETWEEN ? AND ?`, ids, dateOnly(start), dateOnly(end))
	if err != nil {
		return nil, err
	}
	records := []calendarRecord{}
	if err := r.db.SelectContext(ctx, &records, r.db.Rebind(query), args...); err != nil {
		return nil, mapDBError(err)
	}
	return records, nil
}

// AttendanceCalendarSchedules loads the shift assignments overlapping the window
// so the grid can tell a rest day apart from an unexplained gap.
func (r *Repository) AttendanceCalendarSchedules(ctx context.Context, ids []int64, start, end time.Time) ([]calendarSchedule, error) {
	if len(ids) == 0 {
		return []calendarSchedule{}, nil
	}
	query, args, err := sqlx.In(`SELECT sa.employee_id, sa.start_date, sa.end_date, s.work_days
		FROM hr_shift_assignments sa JOIN hr_shifts s ON s.id = sa.shift_id
		WHERE sa.employee_id IN (?) AND sa.start_date <= ? AND (sa.end_date IS NULL OR sa.end_date >= ?)
		ORDER BY sa.start_date`, ids, dateOnly(end), dateOnly(start))
	if err != nil {
		return nil, err
	}
	schedules := []calendarSchedule{}
	if err := r.db.SelectContext(ctx, &schedules, r.db.Rebind(query), args...); err != nil {
		return nil, mapDBError(err)
	}
	return schedules, nil
}

// AttendanceCalendarLeave loads the booked leave overlapping the window, past and
// future alike. Approved leave explains a day with no clock-in; pending leave is
// carried separately so a request in review is never shown as settled.
func (r *Repository) AttendanceCalendarLeave(ctx context.Context, ids []int64, start, end time.Time) ([]calendarLeave, error) {
	if len(ids) == 0 {
		return []calendarLeave{}, nil
	}
	query, args, err := sqlx.In(`SELECT lr.employee_id, lr.start_date, lr.end_date, lr.status, lr.start_portion, lr.end_portion,
		(SELECT p.name FROM hr_leave_policies p WHERE p.id = lr.policy_id) AS policy_name
		FROM hr_leave_requests lr
		WHERE lr.employee_id IN (?) AND lr.status IN ('approved','pending')
		AND lr.start_date <= ? AND lr.end_date >= ?
		ORDER BY FIELD(lr.status,'pending','approved')`, ids, dateOnly(end), dateOnly(start))
	if err != nil {
		return nil, err
	}
	leaves := []calendarLeave{}
	if err := r.db.SelectContext(ctx, &leaves, r.db.Rebind(query), args...); err != nil {
		return nil, mapDBError(err)
	}
	return leaves, nil
}

func dateOnly(value time.Time) string { return value.Format("2006-01-02") }

// AttendanceCalendar assembles the employee × day grid. Every employee carries a
// cell for every day of the window (present or not) so the client renders a
// rectangular table without having to fill holes itself.
func (s *Service) AttendanceCalendar(ctx context.Context, actorUserID int64, start, end time.Time, departmentID *int64) (map[string]any, error) {
	opts, err := s.ScopeList(ctx, actorUserID, "attendance", ListOptions{Filters: map[string]any{}})
	if err != nil {
		return nil, err
	}
	employees, err := s.repo.AttendanceCalendarEmployees(ctx, departmentID, opts)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(employees))
	for _, employee := range employees {
		ids = append(ids, employee.ID)
	}
	records, err := s.repo.AttendanceCalendarRecords(ctx, ids, start, end)
	if err != nil {
		return nil, err
	}
	schedules, err := s.repo.AttendanceCalendarSchedules(ctx, ids, start, end)
	if err != nil {
		return nil, err
	}
	leaves, err := s.attendanceCalendarLeave(ctx, actorUserID, ids, start, end)
	if err != nil {
		return nil, err
	}

	byEmployee := map[int64]map[string]calendarRecord{}
	for _, record := range records {
		day := byEmployee[record.EmployeeID]
		if day == nil {
			day = map[string]calendarRecord{}
			byEmployee[record.EmployeeID] = day
		}
		day[dateOnly(record.Date)] = record
	}
	scheduleByEmployee := map[int64][]calendarSchedule{}
	for _, schedule := range schedules {
		scheduleByEmployee[schedule.EmployeeID] = append(scheduleByEmployee[schedule.EmployeeID], schedule)
	}
	leaveByEmployee := map[int64][]calendarLeave{}
	for _, leave := range leaves {
		leaveByEmployee[leave.EmployeeID] = append(leaveByEmployee[leave.EmployeeID], leave)
	}

	days := make([]string, 0, defaultCalendarDays)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		days = append(days, dateOnly(day))
	}

	rows := make([]map[string]any, 0, len(employees))
	for _, employee := range employees {
		cells := make([]map[string]any, 0, len(days))
		totals := map[string]int{}
		for offset, key := range days {
			date := start.AddDate(0, 0, offset)
			scheduled := isScheduled(scheduleByEmployee[employee.ID], date)
			cell := map[string]any{"date": key, "scheduled": scheduled}
			// Booked leave is attached to any scheduled day it covers, past or
			// future, so a planned absence reads as planned rather than as a hole.
			// A rest day inside a leave span stays a rest day: it consumes nothing.
			leave, onLeave := leaveOn(leaveByEmployee[employee.ID], date)
			if scheduled && onLeave {
				cell["leave"] = map[string]any{
					"status":  leave.Status,
					"policy":  nullStringValue(leave.PolicyName),
					"portion": leavePortion(leave, date),
				}
			}
			record, recorded := byEmployee[employee.ID][key]
			switch {
			case recorded:
				cell["attendance_id"] = record.ID
				cell["status"] = record.Status
				cell["worked_minutes"] = record.WorkedMinutes
				cell["late_minutes"] = record.LateMinutes
				cell["overtime_minutes"] = record.OvertimeMinutes
				cell["clock_in"] = nullTimeValue(record.ClockIn)
				cell["clock_out"] = nullTimeValue(record.ClockOut)
				cell["shift_name"] = nullStringValue(record.ShiftName)
				totals[record.Status]++
				totals["worked_minutes"] += record.WorkedMinutes
				totals["overtime_minutes"] += record.OvertimeMinutes
				if record.LateMinutes > 0 {
					totals["late"]++
					totals["late_minutes"] += record.LateMinutes
				}
			case scheduled && onLeave:
				totals["leave_"+leave.Status]++
			case scheduled && !date.After(time.Now()):
				// A scheduled day in the past with no record and no leave: shown as a
				// gap, never silently counted as present.
				totals["missing"]++
			}
			cells = append(cells, cell)
		}
		rows = append(rows, map[string]any{
			"employee_id":     employee.ID,
			"full_name":       employee.FullName,
			"photo_url":       nullStringValue(employee.PhotoURL),
			"position_title":  nullStringValue(employee.PositionTitle),
			"department_id":   nullInt64Value(employee.DepartmentID),
			"department_name": nullStringValue(employee.DepartmentName),
			"days":            cells,
			"totals":          totals,
		})
	}
	return map[string]any{"start": dateOnly(start), "end": dateOnly(end), "days": days, "employees": rows}, nil
}

// attendanceCalendarLeave loads booked leave on its own authorization axis.
// Attendance access does not imply leave access, so the leave scope is resolved
// separately and the grid simply carries no leave when the caller may not read
// it — the attendance rows are unaffected either way.
func (s *Service) attendanceCalendarLeave(ctx context.Context, actorUserID int64, ids []int64, start, end time.Time) ([]calendarLeave, error) {
	opts, err := s.ScopeList(ctx, actorUserID, "leave-requests", ListOptions{Filters: map[string]any{}})
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			return []calendarLeave{}, nil
		}
		return nil, err
	}
	visible := ids
	if opts.RestrictEmployee {
		// The leave scope can be narrower than the attendance scope; only the
		// intersection may be shown.
		allowed := make(map[int64]bool, len(opts.AllowedEmployeeIDs))
		for _, id := range opts.AllowedEmployeeIDs {
			allowed[id] = true
		}
		visible = make([]int64, 0, len(ids))
		for _, id := range ids {
			if allowed[id] {
				visible = append(visible, id)
			}
		}
	}
	return s.repo.AttendanceCalendarLeave(ctx, visible, start, end)
}

// leaveOn returns the leave covering the date. Rows arrive pending-first, so an
// approved request always wins over a pending one on the same day.
func leaveOn(leaves []calendarLeave, date time.Time) (calendarLeave, bool) {
	key := dateOnly(date)
	var found calendarLeave
	var ok bool
	for _, leave := range leaves {
		if dateOnly(leave.StartDate) <= key && key <= dateOnly(leave.EndDate) {
			found, ok = leave, true
		}
	}
	return found, ok
}

// leavePortion reports whether the day is taken whole or as a half day. Only the
// first and last day of a request may be a half day.
func leavePortion(leave calendarLeave, date time.Time) string {
	key := dateOnly(date)
	if key == dateOnly(leave.StartDate) && leave.StartPortion == "half" {
		return "half"
	}
	if key == dateOnly(leave.EndDate) && leave.EndPortion == "half" {
		return "half"
	}
	return "full"
}

// isScheduled answers whether the employee was expected at work on that day. The
// assignment covering the date wins; with no assignment at all we fall back to a
// Monday–Friday week so the grid still greys out weekends.
func isScheduled(schedules []calendarSchedule, date time.Time) bool {
	isoDay := int(date.Weekday())
	if isoDay == 0 {
		isoDay = 7
	}
	key := dateOnly(date)
	for _, schedule := range schedules {
		if dateOnly(schedule.StartDate) > key {
			continue
		}
		if schedule.EndDate.Valid && dateOnly(schedule.EndDate.Time) < key {
			continue
		}
		var workDays []int
		_ = json.Unmarshal(schedule.WorkDays, &workDays)
		for _, day := range workDays {
			if day == isoDay {
				return true
			}
		}
		return false
	}
	return isoDay <= 5
}

func nullStringValue(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func nullInt64Value(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullTimeValue(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time.UTC().Format(time.RFC3339)
}

func (h *Handler) attendanceCalendar(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	start, end, err := calendarWindow(r.URL.Query().Get("start"), r.URL.Query().Get("end"))
	if err != nil {
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var departmentID *int64
	if raw := r.URL.Query().Get("department_id"); raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			httpx.Error(w, http.StatusUnprocessableEntity, "department_id must be numeric")
			return
		}
		departmentID = &parsed
	}
	calendar, err := h.svc.AttendanceCalendar(r.Context(), actor, start, end, departmentID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"attendance_calendar": calendar})
}

// calendarWindow parses the requested window, defaulting to the fortnight ending
// today and refusing anything longer than maxCalendarDays.
func calendarWindow(rawStart, rawEnd string) (time.Time, time.Time, error) {
	const layout = "2006-01-02"
	today := time.Now().UTC().Truncate(24 * time.Hour)
	start, end := today.AddDate(0, 0, -(defaultCalendarDays - 1)), today
	if rawStart != "" {
		parsed, err := time.Parse(layout, rawStart)
		if err != nil {
			return start, end, errInvalidDate
		}
		start = parsed
		end = start.AddDate(0, 0, defaultCalendarDays-1)
	}
	if rawEnd != "" {
		parsed, err := time.Parse(layout, rawEnd)
		if err != nil {
			return start, end, errInvalidDate
		}
		end = parsed
	}
	if end.Before(start) {
		return start, end, errInvalidRange
	}
	if end.Sub(start) > time.Duration(maxCalendarDays-1)*24*time.Hour {
		return start, end, errRangeTooLong
	}
	return start, end, nil
}

var (
	errInvalidDate  = calendarError("start and end must be YYYY-MM-DD dates")
	errInvalidRange = calendarError("end must not precede start")
	errRangeTooLong = calendarError("the requested range is too long")
)

type calendarError string

func (e calendarError) Error() string { return string(e) }
