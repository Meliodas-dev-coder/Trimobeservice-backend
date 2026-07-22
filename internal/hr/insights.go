package hr

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/trimo/backend/internal/hraccess"
)

func notifyEmployeeTx(ctx context.Context, tx *sqlx.Tx, employeeID int64, kind, title, message, entityType string, entityID int64) error {
	var userID sql.NullInt64
	if err := tx.GetContext(ctx, &userID, `SELECT user_id FROM hr_employees WHERE id=?`, employeeID); err != nil {
		return mapDBError(err)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO hr_notifications(recipient_user_id,employee_id,notification_type,title,message,entity_type,entity_id) VALUES (?,?,?,?,?,?,?)`, nullableInt(userID), employeeID, kind, title, message, entityType, entityID)
	return err
}

func notifyLeaveApproverTx(ctx context.Context, tx *sqlx.Tx, employeeID int64, raw json.RawMessage, levelIndex int, requestID int64) error {
	var levels []any
	if json.Unmarshal(raw, &levels) != nil || levelIndex < 0 || levelIndex >= len(levels) {
		return nil
	}
	approver := "hr"
	var userID int64
	switch level := levels[levelIndex].(type) {
	case string:
		approver = level
	case map[string]any:
		if value, ok := level["approver"].(string); ok {
			approver = value
		}
		userID, _ = numericInt(level["user_id"])
	}
	title, message := "Leave approval required", "A leave request is waiting for your approval."
	switch approver {
	case "manager":
		_, err := tx.ExecContext(ctx, `INSERT INTO hr_notifications(recipient_user_id,notification_type,title,message,entity_type,entity_id) SELECT COALESCE(m.user_id,(SELECT u.id FROM users u WHERE u.role='admin' AND u.is_active=TRUE AND u.email=m.work_email LIMIT 1)),'leave.approval_required',?,?, 'leave_request',? FROM hr_employees e JOIN hr_employees m ON m.id=e.manager_id WHERE e.id=? AND COALESCE(m.user_id,(SELECT u.id FROM users u WHERE u.role='admin' AND u.is_active=TRUE AND u.email=m.work_email LIMIT 1)) IS NOT NULL`, title, message, requestID, employeeID)
		return err
	case "user":
		if userID <= 0 {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO hr_notifications(recipient_user_id,notification_type,title,message,entity_type,entity_id) VALUES(?,'leave.approval_required',?,?,'leave_request',?)`, userID, title, message, requestID)
		return err
	case "position_hierarchy":
		approverUser, err := resolvePositionApproverUserTx(ctx, tx, employeeID, positionHierarchyDepth(levels, levelIndex))
		if err != nil {
			return err
		}
		if approverUser.Valid {
			_, err := tx.ExecContext(ctx, `INSERT INTO hr_notifications(recipient_user_id,notification_type,title,message,entity_type,entity_id) VALUES(?,'leave.approval_required',?,?,'leave_request',?)`, approverUser.Int64, title, message, requestID)
			return err
		}
		// No resolvable approver up the chain: fall back to the HR broadcast so the
		// request is still seen (matches the approval fallback).
		fallthrough
	default:
		_, err := tx.ExecContext(ctx, `INSERT INTO hr_notifications(recipient_user_id,notification_type,title,message,entity_type,entity_id) SELECT u.id,'leave.approval_required',?,?,'leave_request',? FROM users u LEFT JOIN admin_roles ar ON ar.id=u.admin_role_id WHERE u.role='admin' AND u.is_active=TRUE AND (u.is_super_admin=TRUE OR JSON_CONTAINS(ar.permissions,JSON_QUOTE('hr')) OR JSON_CONTAINS(ar.permissions,JSON_QUOTE('hr_leave')))`, title, message, requestID)
		return err
	}
}

func nullableInt(value sql.NullInt64) any {
	if value.Valid {
		return value.Int64
	}
	return nil
}

func (s *Service) ListNotifications(ctx context.Context, userID int64, unreadOnly bool, limit, offset int) ([]map[string]any, int, error) {
	where := ` WHERE (recipient_user_id=? OR employee_id=(SELECT e.id FROM hr_employees e WHERE e.user_id=?))`
	args := []any{userID, userID}
	if unreadOnly {
		where += ` AND is_read=FALSE`
	}
	var total int
	if err := s.repo.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM hr_notifications`+where, args...); err != nil {
		return nil, 0, err
	}
	rows, err := s.repo.db.QueryxContext(ctx, `SELECT id,recipient_user_id,employee_id,notification_type,title,message,entity_type,entity_id,is_read,read_at,created_at FROM hr_notifications`+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanPlainMaps(rows)
	return items, total, err
}

func (s *Service) MarkNotificationRead(ctx context.Context, id, userID int64) (map[string]any, error) {
	result, err := s.repo.db.ExecContext(ctx, `UPDATE hr_notifications SET is_read=TRUE,read_at=UTC_TIMESTAMP() WHERE id=? AND (recipient_user_id=? OR employee_id=(SELECT e.id FROM hr_employees e WHERE e.user_id=?))`, id, userID, userID)
	if err != nil {
		return nil, err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, ErrNotFound
	}
	rowsx, err := s.repo.db.QueryxContext(ctx, `SELECT id,recipient_user_id,employee_id,notification_type,title,message,entity_type,entity_id,is_read,read_at,created_at FROM hr_notifications WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rowsx.Close()
	items, err := scanPlainMaps(rowsx)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		// The row was concurrently removed between the update and the re-read.
		return nil, ErrNotFound
	}
	return items[0], nil
}

func (s *Service) Dashboard(ctx context.Context, actorUserID int64) (map[string]any, error) {
	access, err := s.resolveActor(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if !access.CanHR("dashboard", "view") {
		return nil, ErrForbidden
	}
	ids, unrestricted, err := s.access.AllowedEmployeeIDs(ctx, access, "dashboard", "view")
	if err != nil {
		return nil, err
	}
	employeeWhere, employeeArgs := idScopeClause("id", ids, unrestricted)
	queries := map[string]string{
		"employees_total":      `SELECT COUNT(*) FROM hr_employees WHERE ` + employeeWhere,
		"employees_active":     `SELECT COUNT(*) FROM hr_employees WHERE employment_status='active' AND ` + employeeWhere,
		"employees_onboarding": `SELECT COUNT(*) FROM hr_employees WHERE employment_status IN ('onboarding','probation') AND ` + employeeWhere,
		"employees_offboarded": `SELECT COUNT(*) FROM hr_employees WHERE employment_status='offboarded' AND ` + employeeWhere,
	}
	result := map[string]any{}
	for key, query := range queries {
		var value any
		if err := s.repo.db.GetContext(ctx, &value, query, employeeArgs...); err != nil {
			return nil, err
		}
		if b, ok := value.([]byte); ok {
			result[key] = string(b)
		} else {
			result[key] = value
		}
	}
	scopedJoin, scopedArgs := idScopeClause("e.id", ids, unrestricted)
	rows, err := s.repo.db.QueryxContext(ctx, `SELECT d.id,d.name,COUNT(e.id) employee_count FROM hr_departments d JOIN hr_employees e ON e.department_id=d.id AND e.employment_status<>'offboarded' WHERE d.is_active=TRUE AND `+scopedJoin+` GROUP BY d.id,d.name ORDER BY employee_count DESC,d.name`, scopedArgs...)
	if err != nil {
		return nil, err
	}
	result["headcount_by_department"], err = scanPlainMaps(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = s.repo.db.QueryxContext(ctx, `SELECT employment_status status,COUNT(*) count FROM hr_employees WHERE `+employeeWhere+` GROUP BY employment_status ORDER BY employment_status`, employeeArgs...)
	if err != nil {
		return nil, err
	}
	result["headcount_by_status"], err = scanPlainMaps(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) EmployeeLookup(ctx context.Context, actorUserID int64, search string) ([]map[string]any, error) {
	access, err := s.resolveActor(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if !access.CanHR("employees", "view") {
		return nil, ErrForbidden
	}
	ids, unrestricted, err := s.access.AllowedEmployeeIDs(ctx, access, "employees", "view")
	if err != nil {
		return nil, err
	}
	args := []any{}
	scope, scopeArgs := idScopeClause("id", ids, unrestricted)
	where := ` WHERE employment_status<>'offboarded' AND ` + scope
	args = append(args, scopeArgs...)
	if strings.TrimSpace(search) != "" {
		where += ` AND (employee_number LIKE ? OR first_name LIKE ? OR last_name LIKE ?)`
		like := "%" + strings.TrimSpace(search) + "%"
		args = append(args, like, like, like)
	}
	rows, err := s.repo.db.QueryxContext(ctx, `SELECT id,employee_number,CONCAT(first_name,' ',last_name) full_name,employment_status,department_id,position_id FROM hr_employees`+where+` ORDER BY last_name,first_name LIMIT 250`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlainMaps(rows)
}

func (s *Service) OrganizationLookup(ctx context.Context, actorUserID int64, kind, search string) ([]map[string]any, error) {
	access, err := s.resolveActor(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if !access.CanHR("organization", "view") {
		return nil, ErrForbidden
	}
	like := "%" + strings.TrimSpace(search) + "%"
	var query string
	var args []any
	switch kind {
	case "departments":
		query = `SELECT id,name,code FROM hr_departments WHERE is_active=TRUE`
		if search != "" {
			query += ` AND (name LIKE ? OR code LIKE ?)`
			args = []any{like, like}
		}
		query += ` ORDER BY name LIMIT 250`
	case "positions":
		query = `SELECT p.id,p.title,p.code,p.department_id,d.name department_name FROM hr_positions p LEFT JOIN hr_departments d ON d.id=p.department_id WHERE p.is_active=TRUE`
		if search != "" {
			query += ` AND (p.title LIKE ? OR p.code LIKE ?)`
			args = []any{like, like}
		}
		query += ` ORDER BY p.title LIMIT 250`
	default:
		return nil, ErrNotFound
	}
	rows, err := s.repo.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlainMaps(rows)
}

func (s *Service) AdminUserLookup(ctx context.Context, actorUserID int64, search string) ([]map[string]any, error) {
	if _, err := s.requireSuper(ctx, actorUserID); err != nil {
		return nil, err
	}
	query := `SELECT id,full_name,email FROM users WHERE role='admin' AND is_active=TRUE`
	args := []any{}
	if strings.TrimSpace(search) != "" {
		query += ` AND (full_name LIKE ? OR email LIKE ?)`
		like := "%" + strings.TrimSpace(search) + "%"
		args = []any{like, like}
	}
	query += ` ORDER BY full_name LIMIT 250`
	rows, err := s.repo.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlainMaps(rows)
}

func idScopeClause(column string, ids []int64, unrestricted bool) (string, []any) {
	if unrestricted {
		return "1=1", nil
	}
	if len(ids) == 0 {
		return "1=0", nil
	}
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i] = "?"
		args[i] = id
	}
	return column + " IN (" + strings.Join(marks, ",") + ")", args
}

// reportColumns fixes the CSV column order per report so exports read in a
// logical order (identity → context → metrics) instead of the alphabetical map
// iteration order. Keys match the SELECT aliases in Report.
var reportColumns = map[string][]string{
	"workforce":    {"employee_number", "employee_name", "work_email", "department", "position", "employment_status", "employment_type", "hire_date", "end_date"},
	"leave":        {"employee_number", "employee_name", "policy", "start_date", "end_date", "requested_days", "status", "submitted_at"},
	"attendance":   {"attendance_date", "employee_number", "employee_name", "status", "clock_in", "clock_out", "worked_minutes", "late_minutes", "overtime_minutes"},
	"recruitment":  {"vacancy_code", "vacancy", "first_name", "last_name", "email", "source", "candidate_status", "rating", "applied_at"},
	"expenses":     {"expense_date", "employee_number", "employee_name", "category", "description", "amount", "currency", "status", "payment_reference"},
	"performance":  {"employee_number", "employee_name", "review_type", "review_period_start", "review_period_end", "status", "overall_rating"},
	"compensation": {"employee_number", "employee_name", "effective_date", "base_salary", "currency", "pay_frequency", "bonus_target", "allowances", "reason", "is_current"},
}

func (s *Service) Report(ctx context.Context, name, start, end string) ([]map[string]any, error) {
	if start == "" {
		start = time.Now().UTC().AddDate(0, -1, 0).Format("2006-01-02")
	}
	if end == "" {
		end = time.Now().UTC().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", start); err != nil {
		return nil, fmt.Errorf("%w: invalid start date", ErrValidation)
	}
	if _, err := time.Parse("2006-01-02", end); err != nil {
		return nil, fmt.Errorf("%w: invalid end date", ErrValidation)
	}
	if start > end {
		return nil, fmt.Errorf("%w: start date must not be after end date", ErrValidation)
	}
	var query string
	var args []any
	switch name {
	case "workforce":
		query = `SELECT e.employee_number,CONCAT(e.first_name,' ',e.last_name) employee_name,e.work_email,d.name department,p.title position,e.employment_status,e.employment_type,e.hire_date,e.end_date FROM hr_employees e LEFT JOIN hr_departments d ON d.id=e.department_id LEFT JOIN hr_positions p ON p.id=e.position_id ORDER BY e.last_name,e.first_name`
	case "leave":
		query = `SELECT e.employee_number,CONCAT(e.first_name,' ',e.last_name) employee_name,p.name policy,r.start_date,r.end_date,CAST(r.requested_days AS CHAR) requested_days,r.status,r.submitted_at FROM hr_leave_requests r JOIN hr_employees e ON e.id=r.employee_id JOIN hr_leave_policies p ON p.id=r.policy_id WHERE r.start_date<=? AND r.end_date>=? ORDER BY r.start_date`
		args = []any{end, start}
	case "attendance":
		query = `SELECT a.attendance_date,e.employee_number,CONCAT(e.first_name,' ',e.last_name) employee_name,a.status,a.clock_in,a.clock_out,a.worked_minutes,a.late_minutes,a.overtime_minutes FROM hr_attendance a JOIN hr_employees e ON e.id=a.employee_id WHERE a.attendance_date BETWEEN ? AND ? ORDER BY a.attendance_date,e.last_name`
		args = []any{start, end}
	case "recruitment":
		query = `SELECT v.code vacancy_code,v.title vacancy,c.first_name,c.last_name,c.email,c.source,c.status candidate_status,c.rating,c.applied_at FROM hr_candidates c JOIN hr_vacancies v ON v.id=c.vacancy_id WHERE DATE(c.applied_at) BETWEEN ? AND ? ORDER BY c.applied_at`
		args = []any{start, end}
	case "expenses":
		query = `SELECT x.expense_date,e.employee_number,CONCAT(e.first_name,' ',e.last_name) employee_name,x.category,x.description,CAST(x.amount AS CHAR) amount,x.currency,x.status,x.payment_reference FROM hr_expenses x JOIN hr_employees e ON e.id=x.employee_id WHERE x.expense_date BETWEEN ? AND ? ORDER BY x.expense_date`
		args = []any{start, end}
	case "performance":
		query = `SELECT e.employee_number,CONCAT(e.first_name,' ',e.last_name) employee_name,r.review_type,r.review_period_start,r.review_period_end,r.status,CAST(r.overall_rating AS CHAR) overall_rating FROM hr_performance_reviews r JOIN hr_employees e ON e.id=r.employee_id WHERE r.review_period_end BETWEEN ? AND ? ORDER BY r.review_period_end`
		args = []any{start, end}
	case "compensation":
		query = `SELECT e.employee_number,CONCAT(e.first_name,' ',e.last_name) employee_name,c.effective_date,CAST(c.base_salary AS CHAR) base_salary,c.currency,c.pay_frequency,CAST(c.bonus_target AS CHAR) bonus_target,c.allowances,c.reason,c.is_current FROM hr_compensation c JOIN hr_employees e ON e.id=c.employee_id WHERE c.effective_date BETWEEN ? AND ? ORDER BY c.effective_date,e.last_name`
		args = []any{start, end}
	default:
		return nil, ErrNotFound
	}
	rows, err := s.repo.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlainMaps(rows)
}

func (s *Service) AuditHistory(ctx context.Context, actorUserID int64, limit, offset int) ([]map[string]any, int, error) {
	access, err := s.resolveActor(ctx, actorUserID)
	if err != nil {
		return nil, 0, err
	}
	if !access.IsSuper && (!access.CanHR("audit", "view") || !containsString(access.Scopes("audit", "view"), hraccess.ScopeAll)) {
		return nil, 0, ErrForbidden
	}
	var total int
	if err := s.repo.db.GetContext(ctx, &total, `SELECT (SELECT COUNT(*) FROM audit_logs WHERE path LIKE '/api/v1/admin/hr/%')+(SELECT COUNT(*) FROM hr_workflow_actions)+(SELECT COUNT(*) FROM hr_expense_actions)+(SELECT COUNT(*) FROM hr_leave_approvals)+(SELECT COUNT(*) FROM hr_access_events)`); err != nil {
		return nil, 0, err
	}
	rows, err := s.repo.db.QueryxContext(ctx, `SELECT * FROM (
		SELECT CONCAT('http-',a.id) record_id,a.actor_user_id,u.full_name actor_name,a.method,a.path,a.target_type,a.target_id,a.status_code,NULL from_status,NULL to_status,NULL note,a.created_at,'http' source FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_user_id WHERE a.path LIKE '/api/v1/admin/hr/%'
		UNION ALL SELECT CONCAT('workflow-',w.id),w.actor_user_id,u.full_name,UPPER(w.action),CONCAT('/api/v1/admin/hr/',w.resource_type,'/',w.resource_id,'/',w.action),w.resource_type,w.resource_id,200,w.from_status,w.to_status,w.note,w.created_at,'workflow' FROM hr_workflow_actions w LEFT JOIN users u ON u.id=w.actor_user_id
		UNION ALL SELECT CONCAT('expense-',x.id),x.actor_user_id,u.full_name,UPPER(x.action),CONCAT('/api/v1/admin/hr/expenses/',x.expense_id,'/',x.action),'expenses',x.expense_id,200,NULL,NULL,x.comment,x.created_at,'expense_workflow' FROM hr_expense_actions x LEFT JOIN users u ON u.id=x.actor_user_id
		UNION ALL SELECT CONCAT('leave-',l.id),l.approver_user_id,u.full_name,UPPER(l.action),CONCAT('/api/v1/admin/hr/leave-requests/',l.request_id,'/',l.action),'leave_requests',l.request_id,200,NULL,NULL,l.comment,l.created_at,'leave_workflow' FROM hr_leave_approvals l LEFT JOIN users u ON u.id=l.approver_user_id
		UNION ALL SELECT CONCAT('access-',s.id),s.actor_user_id,u.full_name,UPPER(s.action_key),CONCAT('/api/v1/admin/hr/',s.resource_type,IF(s.resource_id IS NULL,'',CONCAT('/',s.resource_id))),s.resource_type,s.resource_id,200,NULL,NULL,CONCAT('scope=',COALESCE(s.scope_source,'unknown'),IF(s.target_employee_id IS NULL,'',CONCAT('; employee_id=',s.target_employee_id))),s.created_at,'sensitive_access' FROM hr_access_events s LEFT JOIN users u ON u.id=s.actor_user_id
	) history ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanPlainMaps(rows)
	return items, total, err
}

func scanPlainMaps(rows *sqlx.Rows) ([]map[string]any, error) {
	items := []map[string]any{}
	for rows.Next() {
		row := map[string]any{}
		if err := rows.MapScan(row); err != nil {
			return nil, err
		}
		for k, v := range row {
			if b, ok := v.([]byte); ok {
				row[k] = string(b)
			}
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

type DocumentFile struct {
	Name string `db:"file_name"`
	MIME string `db:"mime_type"`
	Data []byte `db:"file_data"`
}

func (s *Service) UploadDocument(ctx context.Context, actorID int64, meta map[string]any, fileName, mime string, data []byte) (map[string]any, map[string]string, error) {
	def := resources["documents"]
	values, problems := normalizeInput(def, meta, true)
	// Upload supplies these server-controlled fields, while file_url stays optional.
	delete(problems, "file_url")
	if len(data) == 0 {
		problems["file"] = "is required"
	}
	if len(data) > 5<<20 {
		problems["file"] = "must not exceed 5 MiB"
	}
	if len(problems) > 0 {
		return nil, problems, nil
	}
	result, err := s.repo.db.ExecContext(ctx, `INSERT INTO hr_documents(employee_id,document_type,name,file_name,mime_type,size_bytes,file_data,expires_at,is_confidential,notes,uploaded_by) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, values["employee_id"], values["document_type"], values["name"], fileName, mime, len(data), data, values["expires_at"], boolValue(values["is_confidential"], true), values["notes"], actorID)
	if err != nil {
		return nil, nil, mapDBError(err)
	}
	id, _ := result.LastInsertId()
	item, err := s.repo.Get(ctx, def, id)
	return item, nil, err
}

func (s *Service) DownloadDocument(ctx context.Context, id int64) (*DocumentFile, error) {
	var file DocumentFile
	if err := s.repo.db.GetContext(ctx, &file, `SELECT COALESCE(file_name,name) file_name,COALESCE(mime_type,'application/octet-stream') mime_type,file_data FROM hr_documents WHERE id=?`, id); err != nil {
		return nil, mapDBError(err)
	}
	if len(file.Data) == 0 {
		return nil, ErrNotFound
	}
	return &file, nil
}

func sanitizeCSVCell(value any) string {
	s := fmt.Sprint(value)
	if strings.HasPrefix(s, "=") || strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "@") {
		return "'" + s
	}
	return s
}
