package hr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/trimo/backend/internal/hraccess"
)

// CreateLifecycle records a personnel movement. The movement only mutates the
// employee when explicitly completed, making future-dated changes schedulable.
func (s *Service) CreateLifecycle(ctx context.Context, actorID int64, input map[string]any) (map[string]any, map[string]string, error) {
	def := resources["lifecycle-events"]
	values, problems := normalizeInput(def, input, true)
	if len(problems) > 0 {
		return nil, problems, nil
	}
	var current struct {
		DepartmentID sql.NullInt64 `db:"department_id"`
		PositionID   sql.NullInt64 `db:"position_id"`
		ManagerID    sql.NullInt64 `db:"manager_id"`
	}
	if err := s.repo.db.GetContext(ctx, &current, `SELECT department_id,position_id,manager_id FROM hr_employees WHERE id=?`, values["employee_id"]); err != nil {
		return nil, nil, mapDBError(err)
	}
	eventType := values["event_type"].(string)
	switch eventType {
	case "transfer":
		if values["to_department_id"] == nil && values["to_manager_id"] == nil {
			return nil, map[string]string{"to_department_id": "transfer needs a destination department and/or manager"}, nil
		}
	case "promotion":
		if values["to_position_id"] == nil {
			return nil, map[string]string{"to_position_id": "is required for a promotion"}, nil
		}
	case "probation_started":
		end, ok := values["probation_end_date"].(string)
		if !ok {
			return nil, map[string]string{"probation_end_date": "is required when probation starts"}, nil
		}
		if end < values["effective_date"].(string) {
			return nil, map[string]string{"probation_end_date": "must be on or after effective_date"}, nil
		}
	}
	if managerID, ok := values["to_manager_id"].(int64); ok {
		employeeID := values["employee_id"].(int64)
		if managerID == employeeID || s.managerChainContains(ctx, managerID, employeeID) {
			return nil, map[string]string{"to_manager_id": "would create a reporting cycle"}, nil
		}
	}
	if positionID, ok := values["to_position_id"].(int64); ok {
		var positionDepartment sql.NullInt64
		if err := s.repo.db.GetContext(ctx, &positionDepartment, `SELECT department_id FROM hr_positions WHERE id=? AND is_active=TRUE`, positionID); err != nil {
			return nil, nil, mapDBError(err)
		}
		targetDepartment := current.DepartmentID
		if supplied, ok := values["to_department_id"].(int64); ok {
			targetDepartment = sql.NullInt64{Int64: supplied, Valid: true}
		}
		if positionDepartment.Valid && targetDepartment.Valid && positionDepartment.Int64 != targetDepartment.Int64 {
			return nil, map[string]string{"to_position_id": "position does not belong to the destination department"}, nil
		}
	}
	if current.DepartmentID.Valid {
		values["from_department_id"] = current.DepartmentID.Int64
	} else {
		values["from_department_id"] = nil
	}
	if current.PositionID.Valid {
		values["from_position_id"] = current.PositionID.Int64
	} else {
		values["from_position_id"] = nil
	}
	if current.ManagerID.Valid {
		values["from_manager_id"] = current.ManagerID.Int64
	} else {
		values["from_manager_id"] = nil
	}
	values["created_by"] = actorID
	values["status"] = "scheduled"
	item, err := s.repo.Create(ctx, def, values)
	return item, nil, err
}

func (s *Service) CompleteLifecycle(ctx context.Context, id, actorID int64) (map[string]any, error) {
	return s.completeLifecycle(ctx, id, sql.NullInt64{Int64: actorID, Valid: true})
}

// completeLifecycle applies a scheduled lifecycle event. actor is the user who
// completed it; it is NULL when the background sweeper applies an event whose
// creator is unknown. The only place it is written is the offboarding leave
// cancellation (decided_by), which is a nullable column.
func (s *Service) completeLifecycle(ctx context.Context, id int64, actor sql.NullInt64) (map[string]any, error) {
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var event struct {
			EmployeeID     int64          `db:"employee_id"`
			EventType      string         `db:"event_type"`
			EffectiveDate  string         `db:"effective_date"`
			Status         string         `db:"status"`
			ToDepartmentID sql.NullInt64  `db:"to_department_id"`
			ToPositionID   sql.NullInt64  `db:"to_position_id"`
			ToManagerID    sql.NullInt64  `db:"to_manager_id"`
			ProbationEnd   sql.NullString `db:"probation_end_date"`
		}
		if err := tx.GetContext(ctx, &event, `SELECT employee_id, event_type, DATE_FORMAT(effective_date,'%Y-%m-%d') effective_date, status, to_department_id, to_position_id, to_manager_id, DATE_FORMAT(probation_end_date,'%Y-%m-%d') probation_end_date FROM hr_lifecycle_events WHERE id = ? FOR UPDATE`, id); err != nil {
			return mapDBError(err)
		}
		if event.Status != "scheduled" {
			return ErrInvalidTransition
		}
		effective, _ := time.Parse("2006-01-02", event.EffectiveDate)
		if effective.After(time.Now().UTC()) {
			return fmt.Errorf("%w: effective date has not arrived", ErrInvalidTransition)
		}
		if _, err := tx.ExecContext(ctx, `SELECT id FROM hr_employees WHERE id = ? FOR UPDATE`, event.EmployeeID); err != nil {
			return mapDBError(err)
		}
		sets := []string{}
		args := []any{}
		switch event.EventType {
		case "onboarding":
			sets = append(sets, "employment_status = 'onboarding'")
		case "probation_started":
			sets = append(sets, "employment_status = 'probation'")
		case "probation_completed":
			sets = append(sets, "employment_status = 'active'")
		case "transfer", "promotion":
			// A move changes org placement only; it must not silently flip
			// employment_status (e.g. drag someone off 'leave' or 'probation' into
			// 'active'). Confirmation is its own probation_completed event.
			if event.ToDepartmentID.Valid {
				sets, args = append(sets, "department_id = ?"), append(args, event.ToDepartmentID.Int64)
			}
			if event.ToPositionID.Valid {
				sets, args = append(sets, "position_id = ?"), append(args, event.ToPositionID.Int64)
			}
			if event.ToManagerID.Valid {
				if event.ToManagerID.Int64 == event.EmployeeID || s.managerChainContains(ctx, event.ToManagerID.Int64, event.EmployeeID) {
					return fmt.Errorf("%w: reporting cycle", ErrConflict)
				}
				sets, args = append(sets, "manager_id = ?"), append(args, event.ToManagerID.Int64)
			}
		case "offboarding":
			var protected bool
			if err := tx.GetContext(ctx, &protected, `SELECT EXISTS(SELECT 1 FROM hr_employees e JOIN users u ON u.id=e.user_id WHERE e.id=? AND u.is_super_admin=TRUE)`, event.EmployeeID); err != nil {
				return err
			}
			if protected {
				return ErrForbidden
			}
			sets, args = append(sets, "employment_status = 'offboarded'", "end_date = ?"), append(args, event.EffectiveDate)
		default:
			return ErrInvalidTransition
		}
		if len(sets) > 0 {
			args = append(args, event.EmployeeID)
			if _, err := tx.ExecContext(ctx, `UPDATE hr_employees SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
				return mapDBError(err)
			}
		}
		if event.EventType == "offboarding" {
			if _, err := tx.ExecContext(ctx, `UPDATE users u JOIN hr_employees e ON e.user_id=u.id SET u.is_active=FALSE WHERE e.id=? AND u.is_super_admin=FALSE`, event.EmployeeID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens rt JOIN hr_employees e ON e.user_id=rt.user_id SET rt.revoked_at=COALESCE(rt.revoked_at,UTC_TIMESTAMP()) WHERE e.id=?`, event.EmployeeID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_contracts SET status='terminated',end_date=GREATEST(start_date,?) WHERE employee_id=? AND status IN ('draft','active')`, event.EffectiveDate, event.EmployeeID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_shift_assignments SET status='ended',end_date=GREATEST(start_date,?) WHERE employee_id=? AND status='active'`, event.EffectiveDate, event.EmployeeID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_benefit_enrollments SET status='ended',end_date=GREATEST(start_date,?) WHERE employee_id=? AND status='active'`, event.EffectiveDate, event.EmployeeID); err != nil {
				return err
			}
			rows, err := tx.QueryxContext(ctx, `SELECT id,policy_id,YEAR(start_date) balance_year,requested_days,status FROM hr_leave_requests WHERE employee_id=? AND (status='pending' OR (status='approved' AND start_date>=?)) FOR UPDATE`, event.EmployeeID, event.EffectiveDate)
			if err != nil {
				return err
			}
			type pendingLeave struct {
				ID       int64   `db:"id"`
				PolicyID int64   `db:"policy_id"`
				Year     int     `db:"balance_year"`
				Days     float64 `db:"requested_days"`
				Status   string  `db:"status"`
			}
			var pending []pendingLeave
			for rows.Next() {
				var p pendingLeave
				if err := rows.StructScan(&p); err != nil {
					rows.Close()
					return err
				}
				pending = append(pending, p)
			}
			rows.Close()
			for _, p := range pending {
				column := "pending_days"
				if p.Status == "approved" {
					column = "used_days"
				}
				if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_balances SET `+column+`=GREATEST(0,`+column+`-?) WHERE employee_id=? AND policy_id=? AND balance_year=?`, p.Days, event.EmployeeID, p.PolicyID, p.Year); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_requests SET status='cancelled',decided_at=UTC_TIMESTAMP(),decided_by=? WHERE id=?`, actor, p.ID); err != nil {
					return err
				}
			}
		}
		if event.EventType == "probation_started" && event.ProbationEnd.Valid {
			if _, err := tx.ExecContext(ctx, `UPDATE hr_contracts SET probation_end_date=? WHERE employee_id=? AND status IN ('draft','active') ORDER BY start_date DESC LIMIT 1`, event.ProbationEnd.String, event.EmployeeID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hr_lifecycle_events SET status='completed', completed_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
			return err
		}
		return notifyEmployeeTx(ctx, tx, event.EmployeeID, "lifecycle.completed", "Employee lifecycle updated", "A personnel lifecycle event was completed.", "lifecycle_event", id)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, resources["lifecycle-events"], id)
}

// SweepDueLifecycleEvents completes every scheduled lifecycle event whose
// effective date has arrived, so future-dated onboarding/probation/transfer/
// offboarding take effect on their own instead of waiting on a manual click.
// Each event is applied in its own transaction (via completeLifecycle) so one
// failing event cannot block the rest; the event's creator is recorded as the
// actor. Events another worker already completed surface ErrInvalidTransition
// and are skipped, not counted as failures.
func (s *Service) SweepDueLifecycleEvents(ctx context.Context) (int, error) {
	type due struct {
		ID        int64         `db:"id"`
		CreatedBy sql.NullInt64 `db:"created_by"`
	}
	var events []due
	if err := s.repo.db.SelectContext(ctx, &events, `SELECT id, created_by FROM hr_lifecycle_events WHERE status='scheduled' AND effective_date <= CURDATE() ORDER BY effective_date, id LIMIT 200`); err != nil {
		return 0, err
	}
	applied := 0
	var firstErr error
	for _, event := range events {
		if _, err := s.completeLifecycle(ctx, event.ID, event.CreatedBy); err != nil {
			if !errors.Is(err, ErrInvalidTransition) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		applied++
	}
	return applied, firstErr
}

type leavePolicyRow struct {
	Days               string          `db:"days_per_year"`
	AccrualMode        string          `db:"accrual_mode"`
	CarryOverDays      string          `db:"carry_over_days"`
	MinNotice          int             `db:"minimum_notice_days"`
	MaxConsecutive     sql.NullInt64   `db:"max_consecutive_days"`
	RequiresAttachment bool            `db:"requires_attachment"`
	ApprovalLevels     json.RawMessage `db:"approval_levels"`
}

func (s *Service) CreateLeaveRequest(ctx context.Context, actorID int64, input map[string]any) (map[string]any, map[string]string, error) {
	def := resources["leave-requests"]
	values, problems := normalizeInput(def, input, true)
	delete(problems, "requested_days")
	if len(problems) > 0 {
		return nil, problems, nil
	}
	start, _ := time.Parse("2006-01-02", values["start_date"].(string))
	end, _ := time.Parse("2006-01-02", values["end_date"].(string))
	if end.Before(start) {
		return nil, map[string]string{"end_date": "must be on or after start_date"}, nil
	}
	startPortion := stringValue(values["start_portion"], "full")
	endPortion := stringValue(values["end_portion"], "full")
	days := businessLeaveDays(start, end, startPortion, endPortion)
	if days <= 0 {
		return nil, map[string]string{"start_date": "request must contain at least half a working day"}, nil
	}
	values["requested_days"] = strconv.FormatFloat(days, 'f', 2, 64)
	values["start_portion"], values["end_portion"] = startPortion, endPortion
	values["status"] = "pending"

	var id int64
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT id FROM hr_employees WHERE id=? FOR UPDATE`, values["employee_id"]); err != nil {
			return mapDBError(err)
		}
		var policy leavePolicyRow
		if err := tx.GetContext(ctx, &policy, `SELECT CAST(days_per_year AS CHAR) days_per_year,accrual_mode,CAST(carry_over_days AS CHAR) carry_over_days, minimum_notice_days, max_consecutive_days, requires_attachment, approval_levels FROM hr_leave_policies WHERE id=? AND is_active=TRUE FOR UPDATE`, values["policy_id"]); err != nil {
			return mapDBError(err)
		}
		levels, err := approvalLevelCount(policy.ApprovalLevels)
		if err != nil {
			return fmt.Errorf("%w: leave policy has invalid approval_levels", ErrConflict)
		}
		if policy.RequiresAttachment && values["document_id"] == nil && values["attachment_url"] == nil {
			return fmt.Errorf("%w: this policy requires a supporting document", ErrConflict)
		}
		if documentID, ok := values["document_id"].(int64); ok {
			var ownerID int64
			if err := tx.GetContext(ctx, &ownerID, `SELECT employee_id FROM hr_documents WHERE id=?`, documentID); err != nil {
				return mapDBError(err)
			}
			if ownerID != values["employee_id"].(int64) {
				return fmt.Errorf("%w: supporting document belongs to another employee", ErrConflict)
			}
		}
		if start.Before(time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, policy.MinNotice)) {
			return fmt.Errorf("%w: leave policy notice period is not met", ErrConflict)
		}
		if policy.MaxConsecutive.Valid && days > float64(policy.MaxConsecutive.Int64) {
			return fmt.Errorf("%w: request exceeds maximum consecutive days", ErrConflict)
		}
		if start.Year() != end.Year() {
			return fmt.Errorf("%w: leave requests cannot cross a calendar year", ErrConflict)
		}
		var overlapping int
		if err := tx.GetContext(ctx, &overlapping, `SELECT COUNT(*) FROM hr_leave_requests WHERE employee_id=? AND status IN ('pending','approved') AND start_date <= ? AND end_date >= ? FOR UPDATE`, values["employee_id"], values["end_date"], values["start_date"]); err != nil {
			return err
		}
		if overlapping > 0 {
			return ErrLeaveOverlap
		}
		var hireDate string
		if err := tx.GetContext(ctx, &hireDate, `SELECT DATE_FORMAT(hire_date,'%Y-%m-%d') FROM hr_employees WHERE id=?`, values["employee_id"]); err != nil {
			return err
		}
		allocation := leaveAllocation(policy, start, hireDate)
		carry := 0.0
		carryCap, _ := strconv.ParseFloat(policy.CarryOverDays, 64)
		if carryCap > 0 {
			var prior sql.NullFloat64
			_ = tx.GetContext(ctx, &prior, `SELECT allocated_days+carried_days+adjustment_days-used_days-pending_days FROM hr_leave_balances WHERE employee_id=? AND policy_id=? AND balance_year=? FOR UPDATE`, values["employee_id"], values["policy_id"], start.Year()-1)
			if prior.Valid {
				carry = math.Min(carryCap, math.Max(0, prior.Float64))
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO hr_leave_balances (employee_id,policy_id,balance_year,allocated_days,carried_days) VALUES (?,?,?,?,?)`, values["employee_id"], values["policy_id"], start.Year(), allocation, carry); err != nil {
			return err
		}
		if policy.AccrualMode == "monthly" {
			if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_balances SET allocated_days=GREATEST(allocated_days,?) WHERE employee_id=? AND policy_id=? AND balance_year=?`, allocation, values["employee_id"], values["policy_id"], start.Year()); err != nil {
				return err
			}
		}
		var available float64
		if err := tx.GetContext(ctx, &available, `SELECT allocated_days+carried_days+adjustment_days-used_days-pending_days FROM hr_leave_balances WHERE employee_id=? AND policy_id=? AND balance_year=? FOR UPDATE`, values["employee_id"], values["policy_id"], start.Year()); err != nil {
			return err
		}
		if available+0.0001 < days {
			return ErrInsufficientLeave
		}
		values["current_approval_level"] = 1
		values["total_approval_levels"] = levels
		values["approval_levels_snapshot"] = []byte(policy.ApprovalLevels)
		id, err = insertResource(ctx, tx, def, values)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_balances SET pending_days=pending_days+? WHERE employee_id=? AND policy_id=? AND balance_year=?`, days, values["employee_id"], values["policy_id"], start.Year()); err != nil {
			return err
		}
		if err := notifyEmployeeTx(ctx, tx, values["employee_id"].(int64), "leave.submitted", "Leave request submitted", "Your leave request is awaiting approval.", "leave_request", id); err != nil {
			return err
		}
		return notifyLeaveApproverTx(ctx, tx, values["employee_id"].(int64), policy.ApprovalLevels, 0, id)
	})
	if err != nil {
		return nil, nil, err
	}
	item, err := s.repo.Get(ctx, def, id)
	return item, nil, err
}

func leaveAllocation(policy leavePolicyRow, asOf time.Time, hireDate string) float64 {
	annual, _ := strconv.ParseFloat(policy.Days, 64)
	switch policy.AccrualMode {
	case "manual":
		return 0
	case "monthly":
		hire, _ := time.Parse("2006-01-02", hireDate)
		fromMonth := 1
		if hire.Year() == asOf.Year() {
			fromMonth = int(hire.Month())
		}
		months := int(asOf.Month()) - fromMonth + 1
		if months < 0 {
			months = 0
		}
		if months > 12 {
			months = 12
		}
		return math.Round((annual*float64(months)/12)*100) / 100
	default:
		return annual
	}
}

type leaveRequestLock struct {
	EmployeeID     int64           `db:"employee_id"`
	PolicyID       int64           `db:"policy_id"`
	StartDate      string          `db:"start_date"`
	Days           string          `db:"requested_days"`
	Status         string          `db:"status"`
	Current        int             `db:"current_approval_level"`
	Total          int             `db:"total_approval_levels"`
	ApprovalLevels json.RawMessage `db:"approval_levels_snapshot"`
}

func (s *Service) DecideLeave(ctx context.Context, id, actorID int64, action, note string) (map[string]any, error) {
	if action != "approve" && action != "reject" && action != "cancel" {
		return nil, ErrInvalidTransition
	}
	required := "approve"
	if action == "cancel" {
		required = "cancel"
	}
	actorAccess, err := s.resolveActor(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !actorAccess.CanHR("leave", required) {
		return nil, ErrForbidden
	}
	allowedIDs, unrestricted, err := s.access.AllowedEmployeeIDs(ctx, actorAccess, "leave", required)
	if err != nil {
		return nil, err
	}
	allowed := map[int64]bool{}
	for _, employeeID := range allowedIDs {
		allowed[employeeID] = true
	}
	err = s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var request leaveRequestLock
		if err := tx.GetContext(ctx, &request, `SELECT employee_id,policy_id,DATE_FORMAT(start_date,'%Y-%m-%d') start_date,CAST(requested_days AS CHAR) requested_days,status,current_approval_level,total_approval_levels,approval_levels_snapshot FROM hr_leave_requests WHERE id=? FOR UPDATE`, id); err != nil {
			return mapDBError(err)
		}
		days, _ := strconv.ParseFloat(request.Days, 64)
		year, _ := strconv.Atoi(request.StartDate[:4])
		if _, err := tx.ExecContext(ctx, `SELECT id FROM hr_leave_balances WHERE employee_id=? AND policy_id=? AND balance_year=? FOR UPDATE`, request.EmployeeID, request.PolicyID, year); err != nil {
			return mapDBError(err)
		}
		if !unrestricted && !allowed[request.EmployeeID] {
			return ErrForbidden
		}
		if action != "cancel" && !actorAccess.IsSuper && actorAccess.Employee != nil && actorAccess.Employee.ID == request.EmployeeID {
			return ErrForbidden
		}
		switch action {
		case "approve":
			if request.Status != "pending" {
				return ErrInvalidTransition
			}
			var alreadyActed bool
			if err := tx.GetContext(ctx, &alreadyActed, `SELECT EXISTS(SELECT 1 FROM hr_leave_approvals WHERE request_id=? AND approver_user_id=?)`, id, actorID); err != nil {
				return err
			}
			if alreadyActed {
				return ErrApprovalOrder
			}
			if err := authorizeLeaveApprover(ctx, tx, request, actorID, actorAccess); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO hr_leave_approvals(request_id,approval_level,approver_user_id,action,comment) VALUES (?,?,?,?,?)`, id, request.Current, actorID, "approved", nullString(note)); err != nil {
				return mapDBError(err)
			}
			if request.Current >= request.Total {
				if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_requests SET status='approved',decided_at=UTC_TIMESTAMP(),decided_by=? WHERE id=?`, actorID, id); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_balances SET pending_days=GREATEST(0,pending_days-?),used_days=used_days+? WHERE employee_id=? AND policy_id=? AND balance_year=?`, days, days, request.EmployeeID, request.PolicyID, year); err != nil {
					return err
				}
			} else {
				if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_requests SET current_approval_level=current_approval_level+1 WHERE id=?`, id); err != nil {
					return err
				}
				if err := notifyLeaveApproverTx(ctx, tx, request.EmployeeID, request.ApprovalLevels, request.Current, id); err != nil {
					return err
				}
			}
		case "reject":
			if request.Status != "pending" {
				return ErrInvalidTransition
			}
			var alreadyActed bool
			if err := tx.GetContext(ctx, &alreadyActed, `SELECT EXISTS(SELECT 1 FROM hr_leave_approvals WHERE request_id=? AND approver_user_id=?)`, id, actorID); err != nil {
				return err
			}
			if alreadyActed {
				return ErrApprovalOrder
			}
			if err := authorizeLeaveApprover(ctx, tx, request, actorID, actorAccess); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO hr_leave_approvals(request_id,approval_level,approver_user_id,action,comment) VALUES (?,?,?,?,?)`, id, request.Current, actorID, "rejected", nullString(note)); err != nil {
				return mapDBError(err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_requests SET status='rejected',decided_at=UTC_TIMESTAMP(),decided_by=? WHERE id=?`, actorID, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_balances SET pending_days=GREATEST(0,pending_days-?) WHERE employee_id=? AND policy_id=? AND balance_year=?`, days, request.EmployeeID, request.PolicyID, year); err != nil {
				return err
			}
		case "cancel":
			if request.Status != "pending" && request.Status != "approved" {
				return ErrInvalidTransition
			}
			if request.Status == "pending" {
				if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_balances SET pending_days=GREATEST(0,pending_days-?) WHERE employee_id=? AND policy_id=? AND balance_year=?`, days, request.EmployeeID, request.PolicyID, year); err != nil {
					return err
				}
			} else {
				if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_balances SET used_days=GREATEST(0,used_days-?) WHERE employee_id=? AND policy_id=? AND balance_year=?`, days, request.EmployeeID, request.PolicyID, year); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_leave_requests SET status='cancelled',decided_at=UTC_TIMESTAMP(),decided_by=? WHERE id=?`, actorID, id); err != nil {
				return err
			}
		}
		return notifyEmployeeTx(ctx, tx, request.EmployeeID, "leave."+action, "Leave request updated", "Your leave request was "+pastTense(action)+".", "leave_request", id)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, resources["leave-requests"], id)
}

func businessLeaveDays(start, end time.Time, startPortion, endPortion string) float64 {
	days := 0.0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			days++
		}
	}
	if days == 0 {
		return 0
	}
	if start.Equal(end) {
		if startPortion == "half" || endPortion == "half" {
			return 0.5
		}
		return 1
	}
	if start.Weekday() != time.Saturday && start.Weekday() != time.Sunday && startPortion == "half" {
		days -= 0.5
	}
	if end.Weekday() != time.Saturday && end.Weekday() != time.Sunday && endPortion == "half" {
		days -= 0.5
	}
	return days
}

func approvalLevelCount(raw json.RawMessage) (int, error) {
	var levels []any
	if err := json.Unmarshal(raw, &levels); err != nil || len(levels) == 0 || len(levels) > 10 {
		return 0, errors.New("invalid approval levels")
	}
	for _, level := range levels {
		switch value := level.(type) {
		case string:
			if value != "manager" && value != "hr" && value != "hr_admin" && value != "position_hierarchy" {
				return 0, errors.New("invalid approver")
			}
		case map[string]any:
			approver, _ := value["approver"].(string)
			if approver != "manager" && approver != "hr" && approver != "hr_admin" && approver != "user" && approver != "position_hierarchy" {
				return 0, errors.New("invalid approver")
			}
			if approver == "user" {
				if id, ok := numericInt(value["user_id"]); !ok || id <= 0 {
					return 0, errors.New("specific approver needs user_id")
				}
			}
		default:
			return 0, errors.New("invalid approval level")
		}
	}
	return len(levels), nil
}

func authorizeLeaveApprover(ctx context.Context, tx *sqlx.Tx, request leaveRequestLock, actorID int64, actorAccess *hraccess.Access) error {
	// The super-admin is the top-level authority: it can validate/approve (or
	// reject) any pending step even when it was never assigned as the requester's
	// manager, L+1 (position hierarchy), or a named approver. Every other HR gate
	// already exempts the super-admin the same way.
	if actorAccess != nil && actorAccess.IsSuper {
		return nil
	}
	approver, specific, err := leaveApproverRequirement(request)
	if err != nil {
		return err
	}
	switch approver {
	case "hr":
		if canApproveLeaveHRStage(actorAccess, false) {
			return nil
		}
	case "hr_admin":
		if canApproveLeaveHRStage(actorAccess, true) {
			return nil
		}
	case "user":
		if specific == actorID {
			return nil
		}
	case "manager":
		var managerUser sql.NullInt64
		if err := tx.GetContext(ctx, &managerUser, `SELECT COALESCE(m.user_id,(SELECT u.id FROM users u WHERE u.role='admin' AND u.is_active=TRUE AND u.email=m.work_email LIMIT 1)) FROM hr_employees e LEFT JOIN hr_employees m ON m.id=e.manager_id WHERE e.id=?`, request.EmployeeID); err != nil {
			return err
		}
		if managerUser.Valid && managerUser.Int64 == actorID {
			return nil
		}
	case "position_hierarchy":
		approverUser, err := resolvePositionApproverUserTx(ctx, tx, request.EmployeeID, positionHierarchyDepthForRequest(request))
		if err != nil {
			return err
		}
		if approverUser.Valid && approverUser.Int64 == actorID {
			return nil
		}
		// Chain exhausted (vacant rungs / top reached): HR may step in so a request
		// with an unresolvable position approver never gets stuck.
		if !approverUser.Valid && canApproveLeaveHRStage(actorAccess, false) {
			return nil
		}
	}
	return ErrForbidden
}

func leaveApproverRequirement(request leaveRequestLock) (string, int64, error) {
	var levels []any
	if err := json.Unmarshal(request.ApprovalLevels, &levels); err != nil || request.Current < 1 || request.Current > len(levels) {
		return "", 0, ErrApprovalOrder
	}
	level := levels[request.Current-1]
	approver := "hr"
	var specific int64
	switch value := level.(type) {
	case string:
		approver = value
	case map[string]any:
		if v, ok := value["approver"].(string); ok {
			approver = v
		}
		if v, ok := numericInt(value["user_id"]); ok {
			specific = v
		}
	default:
		return "", 0, ErrApprovalOrder
	}
	return approver, specific, nil
}

func canApproveLeaveHRStage(access *hraccess.Access, adminStage bool) bool {
	if access == nil {
		return false
	}
	if access.IsSuper {
		return true
	}
	if !access.CanHR("leave", "approve") {
		return false
	}
	scopes := access.Scopes("leave", "approve")
	if containsString(scopes, hraccess.ScopeAll) {
		return true
	}
	return !adminStage && containsString(scopes, hraccess.ScopeDepartmentTree)
}

func stringValue(value any, fallback string) string {
	if s, ok := value.(string); ok && s != "" {
		return s
	}
	return fallback
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}
func pastTense(action string) string {
	switch action {
	case "approve":
		return "approved"
	case "reject":
		return "rejected"
	case "cancel":
		return "cancelled"
	case "reimburse":
		return "reimbursed"
	case "submit":
		return "submitted"
	case "complete":
		return "completed"
	}
	return action
}
func nullableSQLString(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}

// standardWorkdayMinutes is the daily worked-time threshold beyond which extra
// time is counted as overtime (8 hours). Overtime is measured against minutes
// actually worked that day, never against the shift's end clock.
const standardWorkdayMinutes = 8 * 60

func (s *Service) RecordAttendance(ctx context.Context, id int64, input map[string]any) (map[string]any, map[string]string, error) {
	def := resources["attendance"]
	shiftRequiresActive := id == 0
	values, problems := normalizeInput(def, input, id == 0)
	if id > 0 {
		current, err := s.repo.Get(ctx, def, id)
		if err != nil {
			return nil, nil, err
		}
		for _, field := range def.Fields {
			if _, ok := values[field.Name]; !ok && current[field.Name] != nil {
				values[field.Name] = current[field.Name]
			}
		}
		delete(problems, "worked_minutes")
		delete(problems, "late_minutes")
		delete(problems, "overtime_minutes")
		if raw, supplied := input["shift_id"]; supplied {
			if raw == nil {
				shiftRequiresActive = true
			} else {
				incomingID, incomingOK := values["shift_id"].(int64)
				currentID, currentOK := numericInt(current["shift_id"])
				shiftRequiresActive = !incomingOK || !currentOK || incomingID != currentID
			}
		}
	}
	if len(problems) > 0 {
		return nil, problems, nil
	}
	date := values["attendance_date"].(string)
	employeeID := values["employee_id"].(int64)
	if values["shift_id"] == nil {
		var shiftID sql.NullInt64
		_ = s.repo.db.GetContext(ctx, &shiftID, `SELECT a.shift_id FROM hr_shift_assignments a JOIN hr_shifts s ON s.id=a.shift_id AND s.is_active=TRUE WHERE a.employee_id=? AND a.status='active' AND a.start_date<=? AND (a.end_date IS NULL OR a.end_date>=?) ORDER BY a.start_date DESC LIMIT 1`, employeeID, date, date)
		if shiftID.Valid {
			values["shift_id"] = shiftID.Int64
		}
	}
	worked, late, overtime := 0, 0, 0
	inTime, hasIn, err := parseAttendanceTimestamp(values["clock_in"])
	if err != nil {
		return nil, map[string]string{"clock_in": "must be an RFC3339 timestamp"}, nil
	}
	outTime, hasOut, err := parseAttendanceTimestamp(values["clock_out"])
	if err != nil {
		return nil, map[string]string{"clock_out": "must be an RFC3339 timestamp"}, nil
	}
	if hasOut && !hasIn {
		return nil, map[string]string{"clock_in": "is required when clock_out is set"}, nil
	}
	if hasIn {
		values["clock_in"] = inTime.UTC().Format("2006-01-02 15:04:05")
	}
	if hasOut {
		values["clock_out"] = outTime.UTC().Format("2006-01-02 15:04:05")
		if outTime.Before(inTime) {
			return nil, map[string]string{"clock_out": "must be after clock_in"}, nil
		}
		worked = int(outTime.Sub(inTime).Minutes())
	}
	if hasIn {
		if shiftID, ok := values["shift_id"].(int64); ok {
			var shift struct {
				Start    string          `db:"start_time"`
				Break    int             `db:"break_minutes"`
				Grace    int             `db:"grace_minutes"`
				WorkDays json.RawMessage `db:"work_days"`
			}
			query := `SELECT TIME_FORMAT(start_time,'%H:%i:%s') start_time,break_minutes,grace_minutes,work_days FROM hr_shifts WHERE id=?`
			if shiftRequiresActive {
				query += ` AND is_active=TRUE`
			}
			if err := s.repo.db.GetContext(ctx, &shift, query, shiftID); err != nil {
				return nil, nil, mapDBError(err)
			}
			location := orgLocation()
			startExpectedLocal, _ := time.ParseInLocation("2006-01-02 15:04:05", date+" "+shift.Start, location)
			startExpected := startExpectedLocal.UTC()
			var scheduledDays []int
			_ = json.Unmarshal(shift.WorkDays, &scheduledDays)
			isoDay := int(startExpectedLocal.Weekday())
			if isoDay == 0 {
				isoDay = 7
			}
			scheduled := false
			for _, day := range scheduledDays {
				if day == isoDay {
					scheduled = true
					break
				}
			}
			// Lateness is only meaningful on a scheduled work day, measured from the
			// shift start plus its grace period.
			if scheduled {
				late = maxInt(0, int(inTime.Sub(startExpected.Add(time.Duration(shift.Grace)*time.Minute)).Minutes()))
			}
			// The unpaid break is only deducted once the worked span is longer than
			// it: a short session (e.g. a quick clock-in/out test) never used a break.
			if hasOut && worked > shift.Break {
				worked -= shift.Break
			}
		}
		// Overtime is time worked beyond a standard 8-hour day — not time spent past
		// the shift's end clock — and applies whether or not a shift is assigned.
		if hasOut {
			overtime = maxInt(0, worked-standardWorkdayMinutes)
		}
	}
	values["worked_minutes"], values["late_minutes"], values["overtime_minutes"] = worked, late, overtime
	if id == 0 {
		item, err := s.repo.Create(ctx, def, values)
		return item, nil, err
	}
	_, err = s.repo.db.ExecContext(ctx, `UPDATE hr_attendance SET employee_id=?,shift_id=?,attendance_date=?,clock_in=?,clock_out=?,status=?,worked_minutes=?,late_minutes=?,overtime_minutes=?,source=?,notes=? WHERE id=?`, values["employee_id"], values["shift_id"], values["attendance_date"], values["clock_in"], values["clock_out"], stringValue(values["status"], "present"), worked, late, overtime, stringValue(values["source"], "manual"), values["notes"], id)
	if err != nil {
		return nil, nil, mapDBError(err)
	}
	item, err := s.repo.Get(ctx, def, id)
	return item, nil, err
}

// actorEmployeeForClock resolves the calling account's own HR employee id. Clock
// in/out is inherently self-service, so it does not go through the attendance
// create/update policy gate: an active employee may always record their own
// working time, and only their own. Accounts without a linked employee record
// (e.g. the owner super-admin) have no working time to clock.
func (s *Service) actorEmployeeForClock(ctx context.Context, actorUserID int64) (int64, error) {
	access, err := s.resolveActor(ctx, actorUserID)
	if err != nil {
		return 0, err
	}
	if access.Employee == nil {
		return 0, fmt.Errorf("%w: no employee record is linked to this account", ErrForbidden)
	}
	return access.Employee.ID, nil
}

// todayAttendance returns the employee's attendance record for the given civil
// date, or (nil, nil) when they have not clocked in yet.
func (s *Service) todayAttendance(ctx context.Context, employeeID int64, date string) (map[string]any, error) {
	var id int64
	err := s.repo.db.GetContext(ctx, &id, `SELECT id FROM hr_attendance WHERE employee_id=? AND attendance_date=?`, employeeID, date)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, resources["attendance"], id)
}

// MyAttendanceToday returns the calling employee's own record for today (in the
// org timezone), or nil when they have not clocked in.
func (s *Service) MyAttendanceToday(ctx context.Context, actorUserID int64) (map[string]any, error) {
	employeeID, err := s.actorEmployeeForClock(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	date := time.Now().In(orgLocation()).Format("2006-01-02")
	return s.todayAttendance(ctx, employeeID, date)
}

// ClockIn stamps clock_in with the server time (the source of truth, so the
// number cannot be faked) on today's record, creating it if needed. Lateness is
// computed immediately from the assigned shift; overtime is settled at clock-out.
func (s *Service) ClockIn(ctx context.Context, actorUserID int64) (map[string]any, map[string]string, error) {
	employeeID, err := s.actorEmployeeForClock(ctx, actorUserID)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().In(orgLocation())
	date := now.Format("2006-01-02")
	existing, err := s.todayAttendance(ctx, employeeID, date)
	if err != nil {
		return nil, nil, err
	}
	if existing != nil {
		if existing["clock_out"] != nil {
			return nil, nil, fmt.Errorf("%w: you already clocked out for today", ErrInvalidTransition)
		}
		if existing["clock_in"] != nil {
			return nil, nil, fmt.Errorf("%w: you are already clocked in", ErrInvalidTransition)
		}
		id, _ := numericInt(existing["id"])
		return s.RecordAttendance(ctx, id, map[string]any{"clock_in": now.Format(time.RFC3339)})
	}
	return s.RecordAttendance(ctx, 0, map[string]any{
		"employee_id":     employeeID,
		"attendance_date": date,
		"clock_in":        now.Format(time.RFC3339),
		"status":          "present",
		"source":          "self_service",
	})
}

// ClockOut stamps clock_out with the server time on today's open record and
// recomputes worked, late and overtime minutes from the assigned shift.
func (s *Service) ClockOut(ctx context.Context, actorUserID int64) (map[string]any, map[string]string, error) {
	employeeID, err := s.actorEmployeeForClock(ctx, actorUserID)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().In(orgLocation())
	date := now.Format("2006-01-02")
	existing, err := s.todayAttendance(ctx, employeeID, date)
	if err != nil {
		return nil, nil, err
	}
	if existing == nil || existing["clock_in"] == nil {
		return nil, nil, fmt.Errorf("%w: clock in before clocking out", ErrInvalidTransition)
	}
	if existing["clock_out"] != nil {
		return nil, nil, fmt.Errorf("%w: you already clocked out for today", ErrInvalidTransition)
	}
	id, _ := numericInt(existing["id"])
	return s.RecordAttendance(ctx, id, map[string]any{"clock_out": now.Format(time.RFC3339)})
}

func parseAttendanceTimestamp(value any) (time.Time, bool, error) {
	if value == nil {
		return time.Time{}, false, nil
	}
	raw, ok := value.(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return time.Time{}, false, errors.New("invalid attendance timestamp")
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed.UTC(), true, nil
		}
	}
	return time.Time{}, false, errors.New("invalid attendance timestamp")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// orgLocation resolves the organization's civil timezone for attendance lateness
// and overtime. Trimo operates in Antananarivo (UTC+3, no DST); the fixed zone is
// a fallback for hosts without the IANA tz database.
func orgLocation() *time.Location {
	if loc, err := time.LoadLocation("Indian/Antananarivo"); err == nil {
		return loc
	}
	return time.FixedZone("EAT", 3*60*60)
}

func (s *Service) TimesheetAction(ctx context.Context, id, actorID int64, action, note string) (map[string]any, error) {
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var row struct {
			EmployeeID int64  `db:"employee_id"`
			Status     string `db:"status"`
		}
		if err := tx.GetContext(ctx, &row, `SELECT employee_id,status FROM hr_timesheets WHERE id=? FOR UPDATE`, id); err != nil {
			return mapDBError(err)
		}
		switch action {
		case "submit":
			if row.Status != "draft" && row.Status != "rejected" {
				return ErrInvalidTransition
			}
			_, err := tx.ExecContext(ctx, `UPDATE hr_timesheets SET status='submitted',submitted_at=UTC_TIMESTAMP(),approved_at=NULL,approved_by=NULL WHERE id=?`, id)
			if err != nil {
				return err
			}
		case "approve":
			if row.Status != "submitted" {
				return ErrInvalidTransition
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_timesheets SET status='approved',approved_at=UTC_TIMESTAMP(),approved_by=? WHERE id=?`, actorID, id); err != nil {
				return err
			}
		case "reject":
			if row.Status != "submitted" {
				return ErrInvalidTransition
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_timesheets SET status='rejected',notes=CONCAT_WS('\n',notes,?) WHERE id=?`, nullString(note), id); err != nil {
				return err
			}
		default:
			return ErrInvalidTransition
		}
		return notifyEmployeeTx(ctx, tx, row.EmployeeID, "timesheet."+action, "Timesheet updated", "Your timesheet was "+pastTense(action)+".", "timesheet", id)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, resources["timesheets"], id)
}

func (s *Service) ExpenseAction(ctx context.Context, id, actorID int64, action, note, reference string) (map[string]any, error) {
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var row struct {
			EmployeeID int64  `db:"employee_id"`
			Status     string `db:"status"`
		}
		if err := tx.GetContext(ctx, &row, `SELECT employee_id,status FROM hr_expenses WHERE id=? FOR UPDATE`, id); err != nil {
			return mapDBError(err)
		}
		switch action {
		case "approve":
			if row.Status != "pending" {
				return ErrInvalidTransition
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_expenses SET status='approved',approved_at=UTC_TIMESTAMP(),approved_by=?,rejection_reason=NULL WHERE id=?`, actorID, id); err != nil {
				return err
			}
		case "reject":
			if row.Status != "pending" {
				return ErrInvalidTransition
			}
			if strings.TrimSpace(note) == "" {
				return fmt.Errorf("%w: rejection note is required", ErrConflict)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_expenses SET status='rejected',rejection_reason=?,approved_by=? WHERE id=?`, note, actorID, id); err != nil {
				return err
			}
		case "reimburse":
			if row.Status != "approved" {
				return ErrInvalidTransition
			}
			if strings.TrimSpace(reference) == "" {
				return fmt.Errorf("%w: payment reference is required", ErrConflict)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_expenses SET status='reimbursed',reimbursed_at=UTC_TIMESTAMP(),reimbursed_by=?,payment_reference=? WHERE id=?`, actorID, reference, id); err != nil {
				return err
			}
		default:
			return ErrInvalidTransition
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hr_expense_actions(expense_id,actor_user_id,action,comment) VALUES (?,?,?,?)`, id, actorID, action, nullString(note)); err != nil {
			return err
		}
		return notifyEmployeeTx(ctx, tx, row.EmployeeID, "expense."+action, "Expense updated", "Your expense was "+pastTense(action)+".", "expense", id)
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, resources["expenses"], id)
}

// WorkflowAction handles small state machines whose data edits remain generic.
// Hiring, leave, expenses, lifecycle and timesheets use their stronger dedicated
// transaction methods above.
func (s *Service) WorkflowAction(ctx context.Context, resource string, id, actorID int64, action, note string) (map[string]any, error) {
	if (action == "reject" || action == "cancel") && strings.TrimSpace(note) == "" {
		return nil, fmt.Errorf("%w: note is required for %s", ErrValidation, action)
	}
	table := ""
	currentAllowed := map[string][]string{}
	target := ""
	extra := ""
	switch resource {
	case "performance-reviews":
		table = "hr_performance_reviews"
		currentAllowed["complete"] = []string{"draft", "in_progress", "employee_acknowledged"}
		target = "completed"
		extra = ",completed_at=UTC_TIMESTAMP()"
	case "one-to-ones":
		table = "hr_one_to_ones"
		currentAllowed["complete"] = []string{"scheduled"}
		currentAllowed["cancel"] = []string{"scheduled"}
		if action == "complete" {
			target = "completed"
			extra = ",completed_at=UTC_TIMESTAMP()"
		} else {
			target = "cancelled"
		}
	case "interviews":
		table = "hr_interviews"
		currentAllowed["complete"] = []string{"scheduled"}
		currentAllowed["cancel"] = []string{"scheduled"}
		if action == "complete" {
			target = "completed"
		} else {
			target = "cancelled"
		}
	case "offers":
		table = "hr_offers"
		currentAllowed["submit"] = []string{"draft"}
		currentAllowed["approve"] = []string{"sent"}
		currentAllowed["reject"] = []string{"sent"}
		currentAllowed["cancel"] = []string{"draft", "sent"}
		switch action {
		case "submit":
			target = "sent"
			extra = ",sent_at=UTC_TIMESTAMP()"
		case "approve":
			target = "accepted"
			extra = ",responded_at=UTC_TIMESTAMP()"
		case "reject":
			target = "declined"
			extra = ",responded_at=UTC_TIMESTAMP()"
		case "cancel":
			target = "withdrawn"
		}
	case "vacancies":
		table = "hr_vacancies"
		currentAllowed["submit"] = []string{"draft", "paused"}
		currentAllowed["complete"] = []string{"open", "paused"}
		currentAllowed["cancel"] = []string{"draft", "open", "paused"}
		currentAllowed["pause"] = []string{"open"}
		if action == "submit" {
			target = "open"
			extra = ",published_at=COALESCE(published_at,UTC_TIMESTAMP())"
		} else if action == "complete" {
			target = "filled"
		} else if action == "pause" {
			target = "paused"
		} else {
			target = "closed"
		}
	case "candidates":
		table = "hr_candidates"
		currentAllowed["approve"] = []string{"applied", "screening", "interview"}
		currentAllowed["reject"] = []string{"applied", "screening", "interview", "offer"}
		currentAllowed["cancel"] = []string{"applied", "screening", "interview", "offer"}
		if action == "reject" {
			target = "rejected"
		} else if action == "cancel" {
			target = "withdrawn"
		}
	default:
		return nil, ErrInvalidTransition
	}
	allowed := currentAllowed[action]
	if len(allowed) == 0 || target == "" && resource != "candidates" {
		return nil, ErrInvalidTransition
	}
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var status string
		if err := tx.GetContext(ctx, &status, `SELECT status FROM `+table+` WHERE id=? FOR UPDATE`, id); err != nil {
			return mapDBError(err)
		}
		if !contains(allowed, status) {
			return ErrInvalidTransition
		}
		if resource == "candidates" && action == "approve" {
			switch status {
			case "applied":
				target = "screening"
			case "screening":
				target = "interview"
			case "interview":
				target = "offer"
			}
		}
		if resource == "offers" && (action == "submit" || action == "approve") {
			var candidateStatus string
			if err := tx.GetContext(ctx, &candidateStatus, `SELECT c.status FROM hr_offers o JOIN hr_candidates c ON c.id=o.candidate_id WHERE o.id=? FOR UPDATE`, id); err != nil {
				return mapDBError(err)
			}
			if candidateStatus != "offer" {
				return fmt.Errorf("%w: candidate is not at offer stage", ErrInvalidTransition)
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET status=?`+extra+` WHERE id=?`, target, id); err != nil {
			return err
		}
		if resource == "candidates" && (action == "reject" || action == "cancel") {
			if _, err := tx.ExecContext(ctx, `UPDATE hr_offers SET status='withdrawn',responded_at=COALESCE(responded_at,UTC_TIMESTAMP()) WHERE candidate_id=? AND status IN ('draft','sent','accepted')`, id); err != nil {
				return err
			}
		}
		if resource == "offers" && action == "approve" {
			if _, err := tx.ExecContext(ctx, `UPDATE hr_offers SET status='withdrawn' WHERE candidate_id=(SELECT candidate_id FROM (SELECT candidate_id FROM hr_offers WHERE id=?) x) AND id<>? AND status IN ('draft','sent')`, id, id); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO hr_workflow_actions(resource_type,resource_id,from_status,to_status,action,actor_user_id,note) VALUES(?,?,?,?,?,?,?)`, resource, id, status, target, action, actorID, nullString(note))
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, resources[resource], id)
}

// HireCandidate atomically turns an accepted offer into an employee, links the
// candidate, and closes the vacancy when all openings have been filled.
func (s *Service) HireCandidate(ctx context.Context, candidateID, actorID int64, input map[string]any) (map[string]any, map[string]any, map[string]string, error) {
	def := resources["employees"]
	values, problems := normalizeInput(def, input, true)
	// Candidate identity is snapshotted inside the transaction; callers only
	// provide employee-specific identifiers and start details.
	delete(problems, "first_name")
	delete(problems, "last_name")
	delete(problems, "hire_date")
	if len(problems) > 0 {
		return nil, nil, problems, nil
	}
	var employeeID int64
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var candidate struct {
			VacancyID int64          `db:"vacancy_id"`
			FirstName string         `db:"first_name"`
			LastName  string         `db:"last_name"`
			Email     string         `db:"email"`
			Phone     sql.NullString `db:"phone"`
			Status    string         `db:"status"`
			Hired     sql.NullInt64  `db:"hired_employee_id"`
		}
		if err := tx.GetContext(ctx, &candidate, `SELECT vacancy_id,first_name,last_name,email,phone,status,hired_employee_id FROM hr_candidates WHERE id=? FOR UPDATE`, candidateID); err != nil {
			return mapDBError(err)
		}
		if candidate.Hired.Valid || candidate.Status != "offer" {
			return ErrInvalidTransition
		}
		var vacancy struct {
			PositionID   sql.NullInt64 `db:"position_id"`
			DepartmentID sql.NullInt64 `db:"department_id"`
			Openings     int           `db:"openings"`
			Status       string        `db:"status"`
		}
		if err := tx.GetContext(ctx, &vacancy, `SELECT position_id,department_id,openings,status FROM hr_vacancies WHERE id=? FOR UPDATE`, candidate.VacancyID); err != nil {
			return mapDBError(err)
		}
		if vacancy.Status != "open" && vacancy.Status != "paused" {
			return ErrInvalidTransition
		}
		var offer struct {
			ID          int64          `db:"id"`
			Salary      string         `db:"offered_salary"`
			Currency    string         `db:"currency"`
			StartDate   string         `db:"start_date"`
			PositionID  sql.NullInt64  `db:"position_id"`
			Terms       sql.NullString `db:"terms"`
			DocumentURL sql.NullString `db:"document_url"`
		}
		if err := tx.GetContext(ctx, &offer, `SELECT id,CAST(offered_salary AS CHAR) offered_salary,currency,DATE_FORMAT(start_date,'%Y-%m-%d') start_date,position_id,terms,document_url FROM hr_offers WHERE candidate_id=? AND status='accepted' ORDER BY id DESC LIMIT 1 FOR UPDATE`, candidateID); err != nil {
			return fmt.Errorf("%w: candidate needs an accepted offer", ErrInvalidTransition)
		}
		values["first_name"], values["last_name"] = candidate.FirstName, candidate.LastName
		if _, ok := values["personal_email"]; !ok {
			values["personal_email"] = candidate.Email
		}
		if _, ok := values["phone"]; !ok && candidate.Phone.Valid {
			values["phone"] = candidate.Phone.String
		}
		if _, ok := values["hire_date"]; !ok {
			values["hire_date"] = offer.StartDate
		}
		if _, ok := values["position_id"]; !ok && offer.PositionID.Valid {
			values["position_id"] = offer.PositionID.Int64
		} else if _, ok := values["position_id"]; !ok && vacancy.PositionID.Valid {
			values["position_id"] = vacancy.PositionID.Int64
		}
		if _, ok := values["department_id"]; !ok && vacancy.DepartmentID.Valid {
			values["department_id"] = vacancy.DepartmentID.Int64
		}
		if managerID, ok := values["manager_id"].(int64); ok {
			var managerExists bool
			if err := tx.GetContext(ctx, &managerExists, `SELECT EXISTS(SELECT 1 FROM hr_employees WHERE id=?)`, managerID); err != nil {
				return err
			}
			if !managerExists {
				return fmt.Errorf("%w: manager must reference an existing employee", ErrConflict)
			}
		} else if positionID, ok := values["position_id"].(int64); ok {
			// No manager named on the hire: seed the reporting line from the
			// department's position ladder (nearest filled position above).
			manager, err := resolveManagerFromPositionTx(ctx, tx, positionID, 0)
			if err != nil {
				return err
			}
			if manager > 0 {
				values["manager_id"] = manager
			}
		}
		values["employment_status"] = "onboarding"
		var err error
		employeeID, err = insertResource(ctx, tx, def, values)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hr_candidates SET status='hired',hired_employee_id=? WHERE id=?`, employeeID, candidateID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hr_offers SET status='accepted',responded_at=COALESCE(responded_at,UTC_TIMESTAMP()) WHERE id=?`, offer.ID); err != nil {
			return err
		}
		contractType := stringValue(values["employment_type"], "permanent")
		if _, err := tx.ExecContext(ctx, `INSERT INTO hr_contracts(employee_id,contract_type,start_date,salary,currency,pay_frequency,status,document_url,terms) VALUES(?,?,?,?,?,'monthly','active',?,?)`, employeeID, contractType, offer.StartDate, offer.Salary, offer.Currency, nullableSQLString(offer.DocumentURL), nullableSQLString(offer.Terms)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hr_compensation(employee_id,effective_date,base_salary,currency,pay_frequency,is_current,reason) VALUES(?,?,?,?, 'monthly',TRUE,'Initial accepted offer')`, employeeID, offer.StartDate, offer.Salary, offer.Currency); err != nil {
			return err
		}
		var hired int
		if err := tx.GetContext(ctx, &hired, `SELECT COUNT(*) FROM hr_candidates WHERE vacancy_id=? AND status='hired'`, candidate.VacancyID); err != nil {
			return err
		}
		if hired >= vacancy.Openings {
			if _, err := tx.ExecContext(ctx, `UPDATE hr_vacancies SET status='filled' WHERE id=?`, candidate.VacancyID); err != nil {
				return err
			}
		}
		return notifyEmployeeTx(ctx, tx, employeeID, "recruitment.hired", "Welcome to Trimo", "Your candidate record has been converted to an employee profile.", "candidate", candidateID)
	})
	if err != nil {
		return nil, nil, nil, err
	}
	candidate, err := s.repo.Get(ctx, resources["candidates"], candidateID)
	if err != nil {
		return nil, nil, nil, err
	}
	employee, err := s.repo.Get(ctx, def, employeeID)
	return candidate, employee, nil, err
}
