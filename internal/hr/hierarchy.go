package hr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"
)

// basePositionLadder is the canonical, department-agnostic reporting ladder used
// to scaffold a department's base position hierarchy. Applying it to a department
// creates (or re-links) one position per rung, wiring parent_position_id and
// hierarchy_rank top-to-bottom so leave approvals and onboarding have a template
// to walk. Rank 1 is the most senior rung. Codes are prefixed with the
// department code so they stay globally unique (uq_hr_positions_code).
//
// TitleFR seeds the position's `translations` bucket so a scaffolded ladder
// reads correctly in the French console without anyone retyping it. The wording
// matches the ladder named in the apply-template confirmation copy.
var basePositionLadder = []struct {
	Suffix  string
	Title   string
	TitleFR string
	Grade   string
}{
	{"HEAD", "Department Head", "Chef de département", "L6"},
	{"MGR", "Manager", "Manager", "L5"},
	{"LEAD", "Team Lead", "Chef d’équipe", "L4"},
	{"SR", "Senior Officer", "Cadre senior", "L3"},
	{"OFF", "Officer", "Cadre", "L2"},
	{"JR", "Junior Officer", "Cadre junior", "L1"},
}

// ladderTranslations renders the per-locale title bucket stored on a template
// position, in the same shape the admin console reads and writes.
func ladderTranslations(titleEN, titleFR string) (string, error) {
	raw, err := json.Marshal(map[string]map[string]string{
		"title": {"en": titleEN, "fr": titleFR},
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

type positionNode struct {
	ID                  int64           `db:"id"`
	Title               string          `db:"title"`
	Code                string          `db:"code"`
	Grade               sql.NullString  `db:"grade"`
	HierarchyRank       int             `db:"hierarchy_rank"`
	ParentPositionID    sql.NullInt64   `db:"parent_position_id"`
	ParentPositionTitle sql.NullString  `db:"parent_position_title"`
	IsActive            int64           `db:"is_active"`
	EmployeeCount       int             `db:"employee_count"`
	Holders             json.RawMessage `db:"holders"`
	Translations        json.RawMessage `db:"translations"`
}

// COALESCE on translations is required, not cosmetic: a SQL NULL cannot scan
// into json.RawMessage (a named []byte), which fails the whole row.
const positionHierarchyQuery = `SELECT p.id, p.title, p.code, p.grade, p.hierarchy_rank, p.parent_position_id, p.is_active,
	COALESCE(p.translations, JSON_OBJECT()) AS translations,
	(SELECT p2.title FROM hr_positions p2 WHERE p2.id = p.parent_position_id) AS parent_position_title,
	(SELECT COUNT(*) FROM hr_employees e WHERE e.position_id = p.id AND e.employment_status <> 'offboarded') AS employee_count,
	COALESCE((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', e.id, 'name', CONCAT(e.first_name, ' ', e.last_name), 'employment_status', e.employment_status))
		FROM hr_employees e WHERE e.position_id = p.id AND e.employment_status <> 'offboarded'), JSON_ARRAY()) AS holders
	FROM hr_positions p WHERE p.department_id = ? ORDER BY p.hierarchy_rank, p.title`

// PositionHierarchy returns a department's positions ordered by rank, each with
// its parent, headcount, and the employees currently holding it. The frontend
// renders these as an indented tree.
func (s *Service) PositionHierarchy(ctx context.Context, departmentID int64) ([]map[string]any, error) {
	var exists bool
	if err := s.repo.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM hr_departments WHERE id=?)`, departmentID); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	var rows []positionNode
	if err := s.repo.db.SelectContext(ctx, &rows, positionHierarchyQuery, departmentID); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var holders any
		if json.Unmarshal(row.Holders, &holders) != nil {
			holders = []any{}
		}
		node := map[string]any{
			"id":                    row.ID,
			"title":                 row.Title,
			"code":                  row.Code,
			"hierarchy_rank":        row.HierarchyRank,
			"is_active":             row.IsActive != 0,
			"employee_count":        row.EmployeeCount,
			"holders":               holders,
			"parent_position_id":    nullableInt(row.ParentPositionID),
			"parent_position_title": nullableSQLString(row.ParentPositionTitle),
			"grade":                 nullableSQLString(row.Grade),
			// Per-locale titles travel with the node so the tree reads in the
			// console language, the same as the generic position list.
			"translations": row.Translations,
		}
		out = append(out, node)
	}
	return out, nil
}

// ApplyBaseTemplate scaffolds (or re-links) the canonical position ladder for a
// department. Missing rungs are created; existing rungs (matched by code) have
// their parent and rank re-wired to the template chain. It is safe to run more
// than once and never touches positions outside the ladder.
func (s *Service) ApplyBaseTemplate(ctx context.Context, departmentID int64) ([]map[string]any, error) {
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var deptCode string
		if err := tx.GetContext(ctx, &deptCode, `SELECT code FROM hr_departments WHERE id=? FOR UPDATE`, departmentID); err != nil {
			return mapDBError(err)
		}
		var prev sql.NullInt64
		for i, rung := range basePositionLadder {
			code := deptCode + "-" + rung.Suffix
			translations, err := ladderTranslations(rung.Title, rung.TitleFR)
			if err != nil {
				return err
			}
			var posID int64
			err = tx.GetContext(ctx, &posID, `SELECT id FROM hr_positions WHERE code=?`, code)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				result, err := tx.ExecContext(ctx, `INSERT INTO hr_positions (department_id, parent_position_id, hierarchy_rank, title, code, grade, translations, is_active) VALUES (?,?,?,?,?,?,?,TRUE)`,
					departmentID, nullableInt(prev), i+1, rung.Title, code, rung.Grade, translations)
				if err != nil {
					return mapDBError(err)
				}
				if posID, err = result.LastInsertId(); err != nil {
					return err
				}
			case err != nil:
				return err
			default:
				// Re-applying never overwrites an admin's own wording: the title is
				// left alone, and translations are only backfilled on rungs created
				// before the ladder carried them.
				if _, err := tx.ExecContext(ctx, `UPDATE hr_positions SET department_id=?, parent_position_id=?, hierarchy_rank=?, translations=COALESCE(translations, CAST(? AS JSON)) WHERE id=?`,
					departmentID, nullableInt(prev), i+1, translations, posID); err != nil {
					return mapDBError(err)
				}
			}
			prev = sql.NullInt64{Int64: posID, Valid: true}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.PositionHierarchy(ctx, departmentID)
}

// resolveManagerFromPositionTx walks up the position parent chain from positionID
// and returns the id of the first active employee holding an ancestor position —
// the base reporting line used to seed a new hire's manager. Vacant ancestor
// positions are skipped. Returns 0 when the chain has no filled rung.
func resolveManagerFromPositionTx(ctx context.Context, tx *sqlx.Tx, positionID, excludeEmployeeID int64) (int64, error) {
	seen := map[int64]bool{}
	for positionID > 0 && !seen[positionID] {
		seen[positionID] = true
		var parent sql.NullInt64
		if err := tx.GetContext(ctx, &parent, `SELECT parent_position_id FROM hr_positions WHERE id=?`, positionID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, nil
			}
			return 0, err
		}
		if !parent.Valid {
			return 0, nil
		}
		var manager sql.NullInt64
		if err := tx.GetContext(ctx, &manager, `SELECT (SELECT e.id FROM hr_employees e WHERE e.position_id=? AND e.employment_status NOT IN ('suspended','offboarded') AND e.id<>? ORDER BY e.id LIMIT 1)`, parent.Int64, excludeEmployeeID); err != nil {
			return 0, err
		}
		if manager.Valid {
			return manager.Int64, nil
		}
		positionID = parent.Int64
	}
	return 0, nil
}

// resolvePositionApproverUserTx returns the login user id of the approver sitting
// `depth` filled rungs up the position chain from the requesting employee. Vacant
// ancestor positions (and holders without a login account) are skipped, so depth
// counts only rungs that can actually approve. An invalid result means the chain
// is exhausted — callers fall back to HR so a request never gets stuck.
func resolvePositionApproverUserTx(ctx context.Context, tx *sqlx.Tx, employeeID int64, depth int) (sql.NullInt64, error) {
	var none sql.NullInt64
	if depth < 1 {
		return none, nil
	}
	var positionID sql.NullInt64
	if err := tx.GetContext(ctx, &positionID, `SELECT position_id FROM hr_employees WHERE id=?`, employeeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return none, nil
		}
		return none, err
	}
	if !positionID.Valid {
		return none, nil
	}
	current := positionID.Int64
	seen := map[int64]bool{}
	for current > 0 && !seen[current] {
		seen[current] = true
		var parent sql.NullInt64
		if err := tx.GetContext(ctx, &parent, `SELECT parent_position_id FROM hr_positions WHERE id=?`, current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return none, nil
			}
			return none, err
		}
		if !parent.Valid {
			return none, nil
		}
		var approver sql.NullInt64
		if err := tx.GetContext(ctx, &approver, `SELECT (SELECT COALESCE(e.user_id,(SELECT u.id FROM users u WHERE u.role='admin' AND u.is_active=TRUE AND u.email=e.work_email LIMIT 1)) FROM hr_employees e WHERE e.position_id=? AND e.employment_status NOT IN ('suspended','offboarded') ORDER BY (e.user_id IS NULL), e.id LIMIT 1)`, parent.Int64); err != nil {
			return none, err
		}
		if approver.Valid {
			depth--
			if depth == 0 {
				return approver, nil
			}
		}
		current = parent.Int64
	}
	return none, nil
}

// approverOfLevel extracts the approver key from an approval-chain entry, which
// is either a bare role string or a { "approver": ... } object.
func approverOfLevel(level any) string {
	switch value := level.(type) {
	case string:
		return value
	case map[string]any:
		if approver, ok := value["approver"].(string); ok {
			return approver
		}
	}
	return ""
}

// positionHierarchyDepth counts how many position_hierarchy steps occur in the
// approval chain up to and including currentIndex. That ordinal is how far up the
// position chain the current step's approver sits, so successive position_hierarchy
// steps escalate one rung at a time.
func positionHierarchyDepth(levels []any, currentIndex int) int {
	depth := 0
	for i := 0; i <= currentIndex && i < len(levels); i++ {
		if approverOfLevel(levels[i]) == "position_hierarchy" {
			depth++
		}
	}
	return depth
}

func positionHierarchyDepthForRequest(request leaveRequestLock) int {
	var levels []any
	if json.Unmarshal(request.ApprovalLevels, &levels) != nil || request.Current < 1 || request.Current > len(levels) {
		return 0
	}
	return positionHierarchyDepth(levels, request.Current-1)
}
