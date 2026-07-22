package hr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/trimo/backend/internal/hraccess"
)

type Service struct {
	repo   *Repository
	access *hraccess.Resolver
}

var decimalPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]{1,2})?$`)

func NewService(repo *Repository) *Service {
	return &Service{repo: repo, access: hraccess.NewResolver(repo.db)}
}

func (s *Service) List(ctx context.Context, resource string, opts ListOptions) ([]map[string]any, int, error) {
	def, ok := resources[resource]
	if !ok {
		return nil, 0, ErrNotFound
	}
	return s.repo.List(ctx, def, opts)
}

func (s *Service) Get(ctx context.Context, resource string, id int64) (map[string]any, error) {
	def, ok := resources[resource]
	if !ok {
		return nil, ErrNotFound
	}
	return s.repo.Get(ctx, def, id)
}

func (s *Service) Create(ctx context.Context, resource string, input map[string]any) (map[string]any, map[string]string, error) {
	def, ok := resources[resource]
	if !ok || def.NoCreate {
		return nil, nil, ErrNotFound
	}
	values, problems := normalizeInput(def, input, true)
	if len(problems) > 0 {
		return nil, problems, nil
	}
	if extra := s.validateCross(ctx, resource, 0, values); len(extra) > 0 {
		return nil, extra, nil
	}
	if resource == "candidates" {
		var id int64
		err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
			var status string
			if err := tx.GetContext(ctx, &status, `SELECT status FROM hr_vacancies WHERE id=? FOR UPDATE`, values["vacancy_id"]); err != nil {
				return mapDBError(err)
			}
			if status != "open" && status != "paused" {
				return fmt.Errorf("%w: vacancy is not accepting candidates", ErrInvalidTransition)
			}
			var err error
			id, err = insertResource(ctx, tx, def, values)
			return err
		})
		if err != nil {
			return nil, nil, err
		}
		item, err := s.repo.Get(ctx, def, id)
		return item, nil, err
	}
	if resource == "interviews" {
		var id int64
		err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
			var status string
			if err := tx.GetContext(ctx, &status, `SELECT status FROM hr_candidates WHERE id=? FOR UPDATE`, values["candidate_id"]); err != nil {
				return mapDBError(err)
			}
			if status != "screening" && status != "interview" && status != "offer" {
				return fmt.Errorf("%w: candidate is not in an interviewable stage", ErrInvalidTransition)
			}
			var err error
			id, err = insertResource(ctx, tx, def, values)
			return err
		})
		if err != nil {
			return nil, nil, err
		}
		item, err := s.repo.Get(ctx, def, id)
		return item, nil, err
	}
	if resource == "offers" {
		var id int64
		err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
			var status string
			if err := tx.GetContext(ctx, &status, `SELECT status FROM hr_candidates WHERE id=? FOR UPDATE`, values["candidate_id"]); err != nil {
				return mapDBError(err)
			}
			if status != "offer" {
				return fmt.Errorf("%w: candidate is not at offer stage", ErrInvalidTransition)
			}
			var count int
			if err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM hr_offers WHERE candidate_id=? AND status IN ('draft','sent','accepted')`, values["candidate_id"]); err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("%w: candidate already has a live offer", ErrConflict)
			}
			var err error
			id, err = insertResource(ctx, tx, def, values)
			return err
		})
		if err != nil {
			return nil, nil, err
		}
		item, err := s.repo.Get(ctx, def, id)
		return item, nil, err
	}
	if isExclusiveResource(resource) {
		var id int64
		err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
			if err := lockEmployeeForValues(ctx, tx, values); err != nil {
				return err
			}
			if err := ensureNoOverlap(ctx, tx, resource, 0, values); err != nil {
				return err
			}
			var err error
			id, err = insertResource(ctx, tx, def, values)
			return err
		})
		if err != nil {
			return nil, nil, err
		}
		item, err := s.repo.Get(ctx, def, id)
		return item, nil, err
	}

	// Compensation changes are history, not destructive edits: installing a new
	// current record atomically retires the previous current record.
	if resource == "compensation" && boolValue(values["is_current"], true) {
		var id int64
		err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
			if err := lockEmployeeForValues(ctx, tx, values); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_compensation SET is_current = FALSE WHERE employee_id = ? AND is_current = TRUE`, values["employee_id"]); err != nil {
				return err
			}
			var err error
			id, err = insertResource(ctx, tx, def, values)
			return err
		})
		if err != nil {
			return nil, nil, err
		}
		item, err := s.repo.Get(ctx, def, id)
		return item, nil, err
	}
	// New hires without an explicit manager inherit the base reporting line from
	// their department's position ladder: the nearest filled position above theirs.
	if resource == "employees" {
		if _, hasManager := values["manager_id"]; !hasManager {
			if positionID, ok := values["position_id"].(int64); ok {
				if err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
					manager, err := resolveManagerFromPositionTx(ctx, tx, positionID, 0)
					if err == nil && manager > 0 {
						values["manager_id"] = manager
					}
					return err
				}); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	item, err := s.repo.Create(ctx, def, values)
	return item, nil, err
}

func (s *Service) Update(ctx context.Context, resource string, id int64, input map[string]any) (map[string]any, map[string]string, error) {
	def, ok := resources[resource]
	if !ok || def.NoUpdate {
		return nil, nil, ErrNotFound
	}
	values, problems := normalizeInput(def, input, false)
	if len(problems) > 0 {
		return nil, problems, nil
	}
	if len(values) == 0 {
		return nil, map[string]string{"body": "provide at least one writable field"}, nil
	}
	merged := make(map[string]any, len(values)+8)
	current, err := s.repo.Get(ctx, def, id)
	if err != nil {
		return nil, nil, err
	}
	if !resourceEditable(resource, current) {
		return nil, nil, ErrInvalidTransition
	}
	for key, value := range current {
		merged[key] = value
	}
	for key, value := range values {
		merged[key] = value
	}
	if extra := s.validateCross(ctx, resource, id, merged); len(extra) > 0 {
		return nil, extra, nil
	}
	if resource == "employees" {
		err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
			if err := updateResource(ctx, tx, def, id, values); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `UPDATE users u JOIN hr_employees e ON e.user_id=u.id SET u.email=e.work_email,u.full_name=CONCAT(e.first_name,' ',e.last_name),u.phone=e.phone WHERE e.id=?`, id)
			return mapDBError(err)
		})
		if err != nil {
			return nil, nil, err
		}
		item, err := s.repo.Get(ctx, def, id)
		return item, nil, err
	}
	if isExclusiveResource(resource) {
		err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
			if err := lockEmployeeForValues(ctx, tx, merged); err != nil {
				return err
			}
			if err := ensureNoOverlap(ctx, tx, resource, id, merged); err != nil {
				return err
			}
			return updateResource(ctx, tx, def, id, values)
		})
		if err != nil {
			return nil, nil, err
		}
		item, err := s.repo.Get(ctx, def, id)
		return item, nil, err
	}
	item, err := s.repo.Update(ctx, def, id, values)
	return item, nil, err
}

func resourceEditable(resource string, current map[string]any) bool {
	status, _ := current["status"].(string)
	switch resource {
	case "contracts":
		return status == "draft" || status == "active"
	case "expenses":
		return status == "pending"
	case "timesheets":
		return status == "draft" || status == "rejected"
	case "performance-reviews":
		return status == "draft" || status == "in_progress" || status == "employee_acknowledged"
	case "one-to-ones":
		return status == "scheduled"
	case "vacancies":
		return status == "draft" || status == "open" || status == "paused"
	case "candidates":
		return status == "applied" || status == "screening" || status == "interview" || status == "offer"
	case "interviews":
		return status == "scheduled"
	case "offers":
		return status == "draft"
	case "benefit-enrollments":
		return status == "active"
	case "shift-assignments":
		return status == "active"
	default:
		return true
	}
}

func (s *Service) Delete(ctx context.Context, resource string, id int64) error {
	def, ok := resources[resource]
	if !ok || def.NoDelete {
		return ErrInvalidTransition
	}
	return s.repo.Delete(ctx, def, id)
}

func normalizeInput(def resourceDef, input map[string]any, creating bool) (map[string]any, map[string]string) {
	fields := fieldMap(def)
	problems := map[string]string{}
	values := map[string]any{}
	for name := range input {
		if _, ok := fields[name]; !ok {
			problems[name] = "unknown field"
		}
	}
	for _, field := range def.Fields {
		raw, exists := input[field.Name]
		if !exists {
			if creating && field.Required {
				problems[field.Name] = "is required"
			}
			continue
		}
		if field.Immutable || (!creating && field.CreateOnly) {
			problems[field.Name] = "is controlled by its workflow"
			continue
		}
		if raw == nil {
			if field.Required {
				problems[field.Name] = "must not be null"
			} else {
				values[field.Name] = nil
			}
			continue
		}
		value, err := normalizeValue(field, raw)
		if err != nil {
			problems[field.Name] = err.Error()
			continue
		}
		values[field.Name] = value
	}
	return values, problems
}

func normalizeValue(field fieldDef, raw any) (any, error) {
	switch field.Kind {
	case kString:
		value, ok := raw.(string)
		if !ok {
			return nil, errors.New("must be a string")
		}
		value = strings.TrimSpace(value)
		if field.Required && value == "" {
			return nil, errors.New("must not be blank")
		}
		if len(field.Allowed) > 0 && !contains(field.Allowed, value) {
			return nil, fmt.Errorf("must be one of %s", strings.Join(field.Allowed, ", "))
		}
		return value, nil
	case kInt:
		value, ok := numericInt(raw)
		if !ok {
			return nil, errors.New("must be a whole number")
		}
		if strings.HasSuffix(field.Name, "_id") && value <= 0 {
			return nil, errors.New("must be a positive id")
		}
		if !strings.HasSuffix(field.Name, "_id") && value < 0 {
			return nil, errors.New("must be non-negative")
		}
		return value, nil
	case kBool:
		value, ok := raw.(bool)
		if !ok {
			return nil, errors.New("must be true or false")
		}
		return value, nil
	case kDate:
		value, ok := raw.(string)
		if !ok {
			return nil, errors.New("must use YYYY-MM-DD")
		}
		t, err := time.Parse("2006-01-02", value)
		if err != nil {
			return nil, errors.New("must use YYYY-MM-DD")
		}
		return t.Format("2006-01-02"), nil
	case kDateTime:
		value, ok := raw.(string)
		if !ok {
			return nil, errors.New("must be an RFC3339 timestamp")
		}
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return nil, errors.New("must be an RFC3339 timestamp")
		}
		return t.UTC().Format("2006-01-02 15:04:05"), nil
	case kDecimal:
		value, ok := decimalString(raw, field.Name == "adjustment_days")
		if !ok {
			return nil, errors.New("must be a non-negative decimal string")
		}
		return value, nil
	case kJSON:
		bytes, err := json.Marshal(raw)
		if err != nil {
			return nil, errors.New("must be valid JSON")
		}
		return bytes, nil
	default:
		return nil, errors.New("unsupported field type")
	}
}

func numericInt(raw any) (int64, bool) {
	switch value := raw.(type) {
	case float64:
		if value != math.Trunc(value) || value > math.MaxInt64 || value < math.MinInt64 {
			return 0, false
		}
		return int64(value), true
	case string:
		n, err := strconv.ParseInt(value, 10, 64)
		return n, err == nil
	case json.Number:
		n, err := value.Int64()
		return n, err == nil
	case int:
		return int64(value), true
	case int64:
		return value, true
	default:
		return 0, false
	}
}

func decimalString(raw any, signed bool) (string, bool) {
	var value string
	switch v := raw.(type) {
	case string:
		value = strings.TrimSpace(v)
	case float64:
		value = strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		value = v.String()
	default:
		return "", false
	}
	pattern := decimalPattern
	if signed {
		pattern = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]{1,2})?$`)
	}
	if !pattern.MatchString(value) {
		return "", false
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || (!signed && n < 0) {
		return "", false
	}
	return value, true
}

func (s *Service) validateCross(ctx context.Context, resource string, id int64, values map[string]any) map[string]string {
	problems := map[string]string{}
	datePairs := [][2]string{{"start_date", "end_date"}, {"review_period_start", "review_period_end"}, {"effective_date", "end_date"}, {"hire_date", "end_date"}, {"start_date", "probation_end_date"}, {"start_date", "due_date"}}
	for _, pair := range datePairs {
		start, sok := values[pair[0]].(string)
		end, eok := values[pair[1]].(string)
		if sok && eok && start > end {
			problems[pair[1]] = "must be on or after " + pair[0]
		}
	}
	if resource == "offers" {
		if expiry, ok := values["expires_at"].(string); ok {
			if start, ok := values["start_date"].(string); ok && expiry > start {
				problems["expires_at"] = "must be on or before start_date"
			}
		}
	}
	if resource == "positions" {
		min, max := decimalFloat(values["min_salary"]), decimalFloat(values["max_salary"])
		if values["min_salary"] != nil && values["max_salary"] != nil && min > max {
			problems["max_salary"] = "must be greater than or equal to min_salary"
		}
		if parentID, ok := values["parent_position_id"].(int64); ok {
			// A position's parent defines the base reporting line for its department:
			// it must live in the same department and never form a cycle. On update,
			// values is the merged current+input row, so department_id is present.
			var parentDept sql.NullInt64
			switch err := s.repo.db.GetContext(ctx, &parentDept, `SELECT department_id FROM hr_positions WHERE id=?`, parentID); {
			case parentID == id:
				problems["parent_position_id"] = "a position cannot report to itself"
			case id > 0 && s.positionChainContains(ctx, parentID, id):
				problems["parent_position_id"] = "would create a position cycle"
			case err != nil:
				problems["parent_position_id"] = "must reference an existing position"
			default:
				var deptID int64
				if v, ok := values["department_id"].(int64); ok {
					deptID = v
				}
				var pDept int64
				if parentDept.Valid {
					pDept = parentDept.Int64
				}
				if pDept != deptID {
					problems["parent_position_id"] = "must belong to the same department as the position"
				}
			}
		}
	}
	if resource == "vacancies" {
		if openings, ok := values["openings"].(int64); ok && openings < 1 {
			problems["openings"] = "must be at least 1"
		}
	}
	if resource == "interviews" {
		if duration, ok := values["duration_minutes"].(int64); ok && duration < 1 {
			problems["duration_minutes"] = "must be at least 1"
		}
	}
	if resource == "leave-policies" {
		if maximum, ok := values["max_consecutive_days"].(int64); ok && maximum < 1 {
			problems["max_consecutive_days"] = "must be at least 1"
		}
	}
	for _, name := range []string{"progress_percent", "weight_percent"} {
		if v, ok := values[name].(int64); ok && v > 100 {
			problems[name] = "must be between 0 and 100"
		}
	}
	for _, name := range []string{"rating", "overall_rating", "score"} {
		if raw, ok := values[name].(string); ok {
			v, _ := strconv.ParseFloat(raw, 64)
			if v > 5 {
				problems[name] = "must be between 0 and 5"
			}
		}
	}
	if resource == "employees" {
		if userID, ok := values["user_id"].(int64); ok {
			var valid bool
			if err := s.repo.db.GetContext(ctx, &valid, `SELECT EXISTS(SELECT 1 FROM users WHERE id=? AND role='admin' AND is_active=TRUE)`, userID); err != nil || !valid {
				problems["user_id"] = "must reference an active admin team member"
			}
		}
		if email, ok := values["work_email"].(string); ok {
			if _, err := mail.ParseAddress(email); err != nil {
				problems["work_email"] = "must be a valid email"
			}
		}
		if manager, ok := values["manager_id"].(int64); ok {
			var managerExists bool
			if err := s.repo.db.GetContext(ctx, &managerExists, `SELECT EXISTS(SELECT 1 FROM hr_employees WHERE id=?)`, manager); err != nil || !managerExists {
				problems["manager_id"] = "must reference an existing employee"
			} else if manager == id || (id > 0 && s.managerChainContains(ctx, manager, id)) {
				problems["manager_id"] = "would create a reporting cycle"
			}
		}
	}
	if resource == "departments" {
		if parentID, ok := values["parent_department_id"].(int64); ok {
			if parentID == id || (id > 0 && s.departmentChainContains(ctx, parentID, id)) {
				problems["parent_department_id"] = "would create a department cycle"
			} else {
				var exists bool
				if err := s.repo.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM hr_departments WHERE id=?)`, parentID); err != nil || !exists {
					problems["parent_department_id"] = "must reference an existing department"
				}
			}
		}
	}
	if positionID, ok := values["position_id"].(int64); ok {
		var positionDepartment sql.NullInt64
		if err := s.repo.db.GetContext(ctx, &positionDepartment, `SELECT department_id FROM hr_positions WHERE id=?`, positionID); err == nil && positionDepartment.Valid {
			if departmentID, ok := values["department_id"].(int64); ok && departmentID != positionDepartment.Int64 {
				problems["position_id"] = "does not belong to the selected department"
			}
		}
	}
	if resource == "leave-policies" {
		if raw, ok := values["approval_levels"].([]byte); ok {
			if _, err := approvalLevelCount(raw); err != nil {
				problems["approval_levels"] = "must contain between 1 and 10 approval steps"
			}
		}
	}
	if resource == "leave-balances" {
		allocated := decimalFloat(values["allocated_days"])
		carried := decimalFloat(values["carried_days"])
		adjustment := decimalFloat(values["adjustment_days"])
		used := decimalFloat(values["used_days"])
		pending := decimalFloat(values["pending_days"])
		if allocated+carried+adjustment-used-pending < -0.0001 {
			problems["adjustment_days"] = "cannot make the available balance negative"
		}
	}
	if resource == "shifts" {
		if raw, ok := values["work_days"].([]byte); ok {
			var days []int
			if json.Unmarshal(raw, &days) != nil || len(days) == 0 {
				problems["work_days"] = "must be a non-empty array of ISO weekdays 1-7"
			} else {
				seen := map[int]bool{}
				for _, day := range days {
					if day < 1 || day > 7 || seen[day] {
						problems["work_days"] = "must contain unique ISO weekdays 1-7"
						break
					}
					seen[day] = true
				}
			}
		}
	}
	return problems
}

func decimalFloat(value any) float64 {
	switch v := value.(type) {
	case string:
		n, _ := strconv.ParseFloat(v, 64)
		return n
	case []byte:
		n, _ := strconv.ParseFloat(string(v), 64)
		return n
	case float64:
		return v
	}
	return 0
}

func isExclusiveResource(resource string) bool {
	return resource == "contracts" || resource == "shift-assignments" || resource == "benefit-enrollments"
}
func lockEmployeeForValues(ctx context.Context, tx *sqlx.Tx, values map[string]any) error {
	employeeID, ok := values["employee_id"].(int64)
	if !ok {
		return fmt.Errorf("%w: employee is required", ErrConflict)
	}
	var id int64
	return mapDBError(tx.GetContext(ctx, &id, `SELECT id FROM hr_employees WHERE id=? FOR UPDATE`, employeeID))
}
func ensureNoOverlap(ctx context.Context, tx *sqlx.Tx, resource string, excludeID int64, values map[string]any) error {
	employeeID := values["employee_id"]
	start := values["start_date"]
	end := values["end_date"]
	var count int
	var query string
	args := []any{employeeID, end, start, excludeID}
	if end == nil {
		args[1] = "9999-12-31"
	}
	switch resource {
	case "contracts":
		query = `SELECT COUNT(*) FROM hr_contracts WHERE employee_id=? AND status NOT IN ('expired','terminated') AND start_date<=? AND COALESCE(end_date,'9999-12-31')>=? AND id<>? FOR UPDATE`
	case "shift-assignments":
		query = `SELECT COUNT(*) FROM hr_shift_assignments WHERE employee_id=? AND status='active' AND start_date<=? AND COALESCE(end_date,'9999-12-31')>=? AND id<>? FOR UPDATE`
	case "benefit-enrollments":
		query = `SELECT COUNT(*) FROM hr_benefit_enrollments WHERE employee_id=? AND benefit_id=? AND status='active' AND start_date<=? AND COALESCE(end_date,'9999-12-31')>=? AND id<>? FOR UPDATE`
	default:
		return nil
	}
	if resource == "benefit-enrollments" {
		args = []any{employeeID, values["benefit_id"], args[1], start, excludeID}
	}
	if err := tx.GetContext(ctx, &count, query, args...); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: date range overlaps an existing active record", ErrConflict)
	}
	return nil
}

func (s *Service) managerChainContains(ctx context.Context, managerID, employeeID int64) bool {
	seen := map[int64]bool{}
	for managerID > 0 && !seen[managerID] {
		if managerID == employeeID {
			return true
		}
		seen[managerID] = true
		var next sql.NullInt64
		if err := s.repo.db.GetContext(ctx, &next, `SELECT manager_id FROM hr_employees WHERE id = ?`, managerID); err != nil || !next.Valid {
			return false
		}
		managerID = next.Int64
	}
	return false
}

func (s *Service) positionChainContains(ctx context.Context, positionID, targetID int64) bool {
	seen := map[int64]bool{}
	for positionID > 0 && !seen[positionID] {
		if positionID == targetID {
			return true
		}
		seen[positionID] = true
		var next sql.NullInt64
		if err := s.repo.db.GetContext(ctx, &next, `SELECT parent_position_id FROM hr_positions WHERE id=?`, positionID); err != nil || !next.Valid {
			return false
		}
		positionID = next.Int64
	}
	return false
}

func (s *Service) departmentChainContains(ctx context.Context, departmentID, childID int64) bool {
	seen := map[int64]bool{}
	for departmentID > 0 && !seen[departmentID] {
		if departmentID == childID {
			return true
		}
		seen[departmentID] = true
		var next sql.NullInt64
		if err := s.repo.db.GetContext(ctx, &next, `SELECT parent_department_id FROM hr_departments WHERE id=?`, departmentID); err != nil || !next.Valid {
			return false
		}
		departmentID = next.Int64
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func boolValue(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	b, ok := value.(bool)
	if !ok {
		return fallback
	}
	return b
}

// isTruthy interprets a boolean column read back through MapScan, which may
// surface a MySQL TINYINT(1) as a bool, an integer, or raw bytes.
func isTruthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case int:
		return v != 0
	case float64:
		return v != 0
	case []byte:
		s := strings.TrimSpace(string(v))
		return s == "1" || strings.EqualFold(s, "true")
	case string:
		s := strings.TrimSpace(v)
		return s == "1" || strings.EqualFold(s, "true")
	}
	return false
}
