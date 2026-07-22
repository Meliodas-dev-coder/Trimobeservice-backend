package hr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

type Repository struct{ db *sqlx.DB }

func NewRepository(db *sqlx.DB) *Repository { return &Repository{db: db} }

type ListOptions struct {
	Search             string
	Filters            map[string]any
	Limit              int
	Offset             int
	RestrictEmployee   bool
	AllowedEmployeeIDs []int64
	ActorEmployeeID    *int64
	FeedbackManager    bool
	FeedbackPrivate    bool
	// HideConfidentialForEmployeeID, when set, excludes documents flagged
	// is_confidential that belong to this employee (the subject must not see
	// HR-internal files about themselves via self-service).
	HideConfidentialForEmployeeID *int64
	// HideDraftDocuments excludes contract documents that have not been issued.
	// Set for anyone below all scope: a draft is HR's working copy.
	HideDraftDocuments bool
}

func (r *Repository) InTx(ctx context.Context, fn func(*sqlx.Tx) error) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func selectColumns(def resourceDef) string {
	cols := make([]string, 0, len(def.Fields)+3)
	cols = append(cols, "id")
	for _, field := range def.Fields {
		cols = append(cols, field.Name)
	}
	cols = append(cols, "created_at")
	if !def.NoUpdated {
		cols = append(cols, "updated_at")
	}
	cols = append(cols, extraColumns(def.Path)...)
	return strings.Join(cols, ", ")
}

// extraColumns keeps the generic CRUD surface pleasant for tables and selects:
// list responses carry human-readable relationship labels as well as stable IDs.
func extraColumns(path string) []string {
	employeeName := func(column string) string {
		return "(SELECT CONCAT(e.first_name, ' ', e.last_name) FROM hr_employees e WHERE e.id = " + column + ") AS employee_name"
	}
	switch path {
	case "departments":
		return []string{"(SELECT CONCAT(e.first_name, ' ', e.last_name) FROM hr_employees e WHERE e.id = manager_employee_id) AS manager_name", "(SELECT d.name FROM hr_departments d WHERE d.id = parent_department_id) AS parent_department_name", "(SELECT COUNT(*) FROM hr_employees e WHERE e.department_id = hr_departments.id AND e.employment_status <> 'offboarded') AS employee_count"}
	case "positions":
		return []string{"(SELECT d.name FROM hr_departments d WHERE d.id = department_id) AS department_name", "(SELECT p2.title FROM hr_positions p2 WHERE p2.id = parent_position_id) AS parent_position_title", "(SELECT COUNT(*) FROM hr_employees e WHERE e.position_id = hr_positions.id AND e.employment_status <> 'offboarded') AS employee_count"}
	case "employees":
		return []string{"CONCAT(first_name, ' ', last_name) AS full_name", "(SELECT d.name FROM hr_departments d WHERE d.id = department_id) AS department_name", "(SELECT p.title FROM hr_positions p WHERE p.id = position_id) AS position_title", "(SELECT CONCAT(m.first_name, ' ', m.last_name) FROM hr_employees m WHERE m.id = manager_id) AS manager_name", "CASE WHEN hr_employees.user_id IS NULL THEN 'not_provisioned' WHEN hr_employees.employment_status NOT IN ('suspended','offboarded') AND (SELECT u.is_active FROM users u WHERE u.id=hr_employees.user_id)=TRUE THEN 'active' ELSE 'inactive' END AS account_status"}
	case "emergency-contacts", "contracts", "documents", "lifecycle-events", "leave-balances", "leave-requests", "shift-assignments", "attendance", "timesheets", "performance-reviews", "goals", "feedback", "one-to-ones", "expenses", "compensation", "benefit-enrollments":
		extra := []string{employeeName("employee_id")}
		switch path {
		case "lifecycle-events":
			extra = append(extra, "(SELECT d.name FROM hr_departments d WHERE d.id = to_department_id) AS to_department_name", "(SELECT p.title FROM hr_positions p WHERE p.id = to_position_id) AS to_position_title")
		case "leave-balances", "leave-requests":
			extra = append(extra, "(SELECT p.name FROM hr_leave_policies p WHERE p.id = policy_id) AS policy_name")
			if path == "leave-balances" {
				extra = append(extra, "CAST(allocated_days+carried_days+adjustment_days-used_days-pending_days AS CHAR) AS remaining_days")
			}
			if path == "leave-requests" {
				extra = append(extra, "COALESCE((SELECT JSON_ARRAYAGG(JSON_OBJECT('approval_level',a.approval_level,'approver_user_id',a.approver_user_id,'approver_name',(SELECT u.full_name FROM users u WHERE u.id=a.approver_user_id),'action',a.action,'comment',a.comment,'created_at',a.created_at)) FROM hr_leave_approvals a WHERE a.request_id=hr_leave_requests.id),JSON_ARRAY()) AS approvals")
			}
		case "shift-assignments", "attendance":
			extra = append(extra, "(SELECT s.name FROM hr_shifts s WHERE s.id = shift_id) AS shift_name")
		case "performance-reviews":
			extra = append(extra, "(SELECT CONCAT(e.first_name, ' ', e.last_name) FROM hr_employees e WHERE e.id = reviewer_employee_id) AS reviewer_name")
		case "feedback":
			extra = append(extra, "(SELECT CONCAT(e.first_name, ' ', e.last_name) FROM hr_employees e WHERE e.id = author_employee_id) AS author_name")
		case "one-to-ones":
			extra = append(extra, "(SELECT CONCAT(e.first_name, ' ', e.last_name) FROM hr_employees e WHERE e.id = manager_employee_id) AS manager_name")
		case "benefit-enrollments":
			extra = append(extra, "(SELECT b.name FROM hr_benefits b WHERE b.id = benefit_id) AS benefit_name")
		}
		return extra
	case "vacancies":
		return []string{"(SELECT d.name FROM hr_departments d WHERE d.id = department_id) AS department_name", "(SELECT p.title FROM hr_positions p WHERE p.id = position_id) AS position_title", "(SELECT CONCAT(e.first_name,' ',e.last_name) FROM hr_employees e WHERE e.id=hiring_manager_employee_id) AS hiring_manager_name", "(SELECT COUNT(*) FROM hr_candidates c WHERE c.vacancy_id = hr_vacancies.id) AS candidate_count"}
	case "candidates":
		return []string{"CONCAT(first_name, ' ', last_name) AS full_name", "(SELECT v.title FROM hr_vacancies v WHERE v.id = vacancy_id) AS vacancy_title"}
	case "interviews":
		return []string{"(SELECT CONCAT(c.first_name, ' ', c.last_name) FROM hr_candidates c WHERE c.id = candidate_id) AS candidate_name", "(SELECT v.title FROM hr_candidates c JOIN hr_vacancies v ON v.id=c.vacancy_id WHERE c.id=candidate_id) AS vacancy_title"}
	case "offers":
		return []string{"(SELECT CONCAT(c.first_name, ' ', c.last_name) FROM hr_candidates c WHERE c.id = candidate_id) AS candidate_name", "(SELECT v.title FROM hr_candidates c JOIN hr_vacancies v ON v.id=c.vacancy_id WHERE c.id=candidate_id) AS vacancy_title", "(SELECT p.title FROM hr_positions p WHERE p.id = position_id) AS position_title"}
	default:
		return nil
	}
}

func (r *Repository) List(ctx context.Context, def resourceDef, opts ListOptions) ([]map[string]any, int, error) {
	where := make([]string, 0, len(opts.Filters)+1)
	args := make([]any, 0, len(opts.Filters)+len(def.Search))
	if opts.Search != "" && len(def.Search) > 0 {
		likes := make([]string, 0, len(def.Search))
		for _, col := range def.Search {
			likes = append(likes, col+" LIKE ?")
			args = append(args, "%"+opts.Search+"%")
		}
		where = append(where, "("+strings.Join(likes, " OR ")+")")
	}
	for _, col := range def.Filters {
		if v, ok := opts.Filters[col]; ok {
			where = append(where, col+" = ?")
			args = append(args, v)
		}
	}
	if def.Path == "attendance" {
		if opts.Filters["__late"] == true {
			where = append(where, "late_minutes > 0")
		}
		if opts.Filters["__overtime"] == true {
			where = append(where, "overtime_minutes > 0")
		}
	}
	if opts.RestrictEmployee {
		column := resourceEmployeeColumn(def.Path)
		if column == "" || len(opts.AllowedEmployeeIDs) == 0 {
			where = append(where, "1 = 0")
		} else {
			marks := make([]string, len(opts.AllowedEmployeeIDs))
			for i, id := range opts.AllowedEmployeeIDs {
				marks[i] = "?"
				args = append(args, id)
			}
			where = append(where, column+" IN ("+strings.Join(marks, ",")+")")
		}
	}
	if def.Path == contractDocumentsResource && opts.HideDraftDocuments {
		where = append(where, "status <> 'draft'")
	}
	if def.Path == "documents" && opts.HideConfidentialForEmployeeID != nil {
		where = append(where, "NOT (is_confidential = TRUE AND employee_id = ?)")
		args = append(args, *opts.HideConfidentialForEmployeeID)
	}
	if def.Path == "feedback" && opts.ActorEmployeeID != nil {
		// A subject never sees manager-only/HR-private feedback about themself.
		// Authors retain access to their own entries. Manager scope unlocks
		// manager feedback for subordinates; only all-scope HR unlocks hr_private.
		where = append(where, `(employee_id<>? OR author_employee_id=? OR visibility NOT IN ('manager','hr_private'))`)
		args = append(args, *opts.ActorEmployeeID, *opts.ActorEmployeeID)
		if !opts.FeedbackManager {
			where = append(where, `(author_employee_id=? OR visibility<>'manager')`)
			args = append(args, *opts.ActorEmployeeID)
		}
		if !opts.FeedbackPrivate {
			where = append(where, `(author_employee_id=? OR visibility<>'hr_private')`)
			args = append(args, *opts.ActorEmployeeID)
		}
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM "+def.Table+clause, args...); err != nil {
		return nil, 0, err
	}
	order := def.OrderBy
	if order == "" {
		order = "id DESC"
	}
	query := "SELECT " + selectColumns(def) + " FROM " + def.Table + clause + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	queryArgs := append(append([]any{}, args...), opts.Limit, opts.Offset)
	rows, err := r.db.QueryxContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanMaps(rows, def)
	return items, total, err
}

func (r *Repository) Get(ctx context.Context, def resourceDef, id int64) (map[string]any, error) {
	rows, err := r.db.QueryxContext(ctx, "SELECT "+selectColumns(def)+" FROM "+def.Table+" WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanMaps(rows, def)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return items[0], nil
}

func scanMaps(rows *sqlx.Rows, def resourceDef) ([]map[string]any, error) {
	jsonFields := map[string]bool{}
	jsonFields["approvals"] = true
	for _, field := range def.Fields {
		if field.Kind == kJSON {
			jsonFields[field.Name] = true
		}
	}
	items := []map[string]any{}
	for rows.Next() {
		row := map[string]any{}
		if err := rows.MapScan(row); err != nil {
			return nil, err
		}
		for key, value := range row {
			if instant, ok := value.(time.Time); ok {
				kind := fieldKind("")
				for _, field := range def.Fields {
					if field.Name == key {
						kind = field.Kind
						break
					}
				}
				if kind == kDate {
					row[key] = instant.Format("2006-01-02")
				} else {
					row[key] = instant.UTC().Format(time.RFC3339)
				}
				continue
			}
			bytes, ok := value.([]byte)
			if !ok {
				continue
			}
			if jsonFields[key] && len(bytes) > 0 {
				var decoded any
				if json.Unmarshal(bytes, &decoded) == nil {
					row[key] = decoded
					continue
				}
			}
			row[key] = string(bytes)
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (r *Repository) Create(ctx context.Context, def resourceDef, values map[string]any) (map[string]any, error) {
	id, err := insertResource(ctx, r.db, def, values)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, def, id)
}

func insertResource(ctx context.Context, exec sqlx.ExtContext, def resourceDef, values map[string]any) (int64, error) {
	cols := make([]string, 0, len(values))
	marks := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for _, field := range def.Fields {
		if value, ok := values[field.Name]; ok {
			cols = append(cols, field.Name)
			marks = append(marks, "?")
			args = append(args, value)
		}
	}
	if len(cols) == 0 {
		return 0, errors.New("no values")
	}
	result, err := exec.ExecContext(ctx, "INSERT INTO "+def.Table+" ("+strings.Join(cols, ", ")+") VALUES ("+strings.Join(marks, ", ")+")", args...)
	if err != nil {
		return 0, mapDBError(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Repository) Update(ctx context.Context, def resourceDef, id int64, values map[string]any) (map[string]any, error) {
	if err := updateResource(ctx, r.db, def, id, values); err != nil {
		return nil, err
	}
	return r.Get(ctx, def, id)
}

func updateResource(ctx context.Context, exec sqlx.ExtContext, def resourceDef, id int64, values map[string]any) error {
	sets := make([]string, 0, len(values))
	args := make([]any, 0, len(values)+1)
	for _, field := range def.Fields {
		if field.Immutable {
			continue
		}
		if value, ok := values[field.Name]; ok {
			sets = append(sets, field.Name+" = ?")
			args = append(args, value)
		}
	}
	if len(sets) == 0 {
		return errors.New("no writable fields")
	}
	args = append(args, id)
	result, err := exec.ExecContext(ctx, "UPDATE "+def.Table+" SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return mapDBError(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		var exists bool
		if err := sqlx.GetContext(ctx, exec, &exists, "SELECT EXISTS(SELECT 1 FROM "+def.Table+" WHERE id=?)", id); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, def resourceDef, id int64) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM "+def.Table+" WHERE id = ?", id)
	if err != nil {
		return mapDBError(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func mapDBError(err error) error {
	var my *mysql.MySQLError
	if errors.As(err, &my) {
		switch my.Number {
		case 1062:
			return fmt.Errorf("%w: duplicate value", ErrConflict)
		case 1451:
			return fmt.Errorf("%w: resource is in use", ErrReferenced)
		case 1452:
			return fmt.Errorf("%w: related resource does not exist", ErrConflict)
		case 3819, 4025:
			return fmt.Errorf("%w: value violates an HR data rule", ErrValidation)
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
