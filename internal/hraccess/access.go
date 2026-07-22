// Package hraccess resolves organization-derived business capabilities and HR
// data scopes. It deliberately has no dependency on auth or HR handlers so the
// same calculation can be used by authorization middleware, /auth/me, and the
// HR services without import cycles.
package hraccess

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
)

const (
	ScopeSelf           = "self"
	ScopeReportingTree  = "reporting_tree"
	ScopeDepartmentTree = "department_tree"
	ScopeAll            = "all"

	LevelRead   = "read"
	LevelManage = "manage"
)

type EmployeeIdentity struct {
	ID               int64  `json:"id"`
	EmployeeNumber   string `json:"employee_number"`
	DepartmentID     *int64 `json:"department_id,omitempty"`
	PositionID       *int64 `json:"position_id,omitempty"`
	ManagerID        *int64 `json:"manager_id,omitempty"`
	EmploymentStatus string `json:"employment_status"`
}

type BusinessGrant struct {
	Key         string `json:"key" db:"capability_key"`
	AccessLevel string `json:"access_level" db:"access_level"`
	Source      string `json:"source" db:"source"`
	Effect      string `json:"-" db:"effect"`
}

type HRPolicy struct {
	Feature string `json:"feature" db:"feature_key"`
	Action  string `json:"action" db:"action_key"`
	Scope   string `json:"scope" db:"employee_scope"`
	Source  string `json:"source" db:"source"`
	Effect  string `json:"-" db:"effect"`
}

type Access struct {
	UserID               int64             `json:"-"`
	IsSuper              bool              `json:"is_super_admin"`
	IsActive             bool              `json:"-"`
	Employee             *EmployeeIdentity `json:"employee,omitempty"`
	IsManager            bool              `json:"is_manager"`
	IsDepartmentHead     bool              `json:"is_department_head"`
	LegacyPermissions    []string          `json:"-"`
	BusinessCapabilities []BusinessGrant   `json:"business_capabilities"`
	HRPolicies           []HRPolicy        `json:"hr_policies"`
}

type Feature struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Actions []string `json:"actions"`
	Scopes  []string `json:"scopes"`
}

var StandardScopes = []string{ScopeSelf, ScopeReportingTree, ScopeDepartmentTree, ScopeAll}
var AllOnlyScopes = []string{ScopeAll}

var Features = []Feature{
	{Key: "dashboard", Label: "HR dashboard", Actions: []string{"view"}, Scopes: StandardScopes},
	{Key: "employees", Label: "Employees", Actions: []string{"view", "create", "update"}, Scopes: StandardScopes},
	{Key: "emergency_contacts", Label: "Emergency contacts", Actions: []string{"view", "create", "update", "delete"}, Scopes: StandardScopes},
	{Key: "organization", Label: "Departments and positions", Actions: []string{"view", "create", "update"}, Scopes: AllOnlyScopes},
	{Key: "lifecycle", Label: "Employee lifecycle", Actions: []string{"view", "create", "complete"}, Scopes: StandardScopes},
	{Key: "contracts", Label: "Contracts", Actions: []string{"view", "create", "update", "download"}, Scopes: StandardScopes},
	{Key: "documents", Label: "Documents", Actions: []string{"view", "upload", "update", "delete", "download"}, Scopes: StandardScopes},
	{Key: "leave", Label: "Leave", Actions: []string{"view", "create", "update", "approve", "cancel"}, Scopes: StandardScopes},
	{Key: "attendance", Label: "Attendance and timesheets", Actions: []string{"view", "create", "update", "submit", "approve"}, Scopes: StandardScopes},
	{Key: "performance", Label: "Performance", Actions: []string{"view", "create", "update", "complete"}, Scopes: StandardScopes},
	{Key: "recruitment", Label: "Recruitment", Actions: []string{"view", "create", "update", "approve", "hire"}, Scopes: AllOnlyScopes},
	{Key: "expenses", Label: "Expenses", Actions: []string{"view", "create", "update", "approve", "reimburse"}, Scopes: StandardScopes},
	{Key: "compensation", Label: "Compensation", Actions: []string{"view", "create", "update"}, Scopes: StandardScopes},
	{Key: "benefits", Label: "Benefits", Actions: []string{"view", "create", "update"}, Scopes: StandardScopes},
	{Key: "reports", Label: "Reports and exports", Actions: []string{"view", "export"}, Scopes: AllOnlyScopes},
	{Key: "audit", Label: "HR audit history", Actions: []string{"view"}, Scopes: AllOnlyScopes},
}

type Resolver struct{ db *sqlx.DB }

func NewResolver(db *sqlx.DB) *Resolver { return &Resolver{db: db} }

type identityRow struct {
	IsSuper         bool           `db:"is_super_admin"`
	IsActive        bool           `db:"is_active"`
	LegacyRaw       []byte         `db:"legacy_permissions"`
	EmployeeID      sql.NullInt64  `db:"employee_id"`
	EmployeeNumber  sql.NullString `db:"employee_number"`
	DepartmentID    sql.NullInt64  `db:"department_id"`
	PositionID      sql.NullInt64  `db:"position_id"`
	ManagerID       sql.NullInt64  `db:"manager_id"`
	EmploymentState sql.NullString `db:"employment_status"`
	IsManager       bool           `db:"is_manager"`
	IsHead          bool           `db:"is_department_head"`
}

type requestCacheKey struct{}

type requestCache struct {
	mu      sync.Mutex
	entries map[int64]*Access
}

// WithRequestCache returns a context that memoizes Resolve results for the life
// of one request. An actor's effective access does not change mid-request, so
// caching the read-only Access avoids re-running the identity, capability,
// policy, and reporting/department-tree queries on every authorization check.
// HR routes install this once so the several Resolve calls a single request
// makes (route guard, handler, workflow) collapse to one.
func WithRequestCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestCacheKey{}, &requestCache{entries: map[int64]*Access{}})
}

// Resolve returns the actor's effective access, served from the per-request
// cache when one is installed (see WithRequestCache).
func (r *Resolver) Resolve(ctx context.Context, userID int64) (*Access, error) {
	cache, ok := ctx.Value(requestCacheKey{}).(*requestCache)
	if !ok {
		return r.resolve(ctx, userID)
	}
	cache.mu.Lock()
	if access, hit := cache.entries[userID]; hit {
		cache.mu.Unlock()
		return access, nil
	}
	cache.mu.Unlock()
	access, err := r.resolve(ctx, userID)
	if err != nil {
		return nil, err
	}
	cache.mu.Lock()
	cache.entries[userID] = access
	cache.mu.Unlock()
	return access, nil
}

func (r *Resolver) resolve(ctx context.Context, userID int64) (*Access, error) {
	var row identityRow
	err := r.db.GetContext(ctx, &row, `
		SELECT u.is_super_admin, u.is_active,
		       COALESCE(ar.permissions, JSON_ARRAY()) AS legacy_permissions,
		       e.id AS employee_id, e.employee_number, e.department_id,
		       e.position_id, e.manager_id, e.employment_status,
		       EXISTS(SELECT 1 FROM hr_employees child
		              WHERE child.manager_id=e.id
		                AND child.employment_status NOT IN ('offboarded','suspended')) AS is_manager,
		       EXISTS(SELECT 1 FROM hr_departments d
		              WHERE d.manager_employee_id=e.id AND d.is_active=TRUE) AS is_department_head
		FROM users u
		LEFT JOIN admin_roles ar ON ar.id=u.admin_role_id
		LEFT JOIN hr_employees e ON e.user_id=u.id
		WHERE u.id=?`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return &Access{UserID: userID}, nil
	}
	if err != nil {
		return nil, err
	}

	access := &Access{UserID: userID, IsSuper: row.IsSuper, IsActive: row.IsActive, IsManager: row.IsManager, IsDepartmentHead: row.IsHead}
	_ = json.Unmarshal(row.LegacyRaw, &access.LegacyPermissions)
	if row.EmployeeID.Valid {
		access.Employee = &EmployeeIdentity{
			ID: row.EmployeeID.Int64, EmployeeNumber: row.EmployeeNumber.String,
			DepartmentID: nullInt(row.DepartmentID), PositionID: nullInt(row.PositionID),
			ManagerID: nullInt(row.ManagerID), EmploymentStatus: row.EmploymentState.String,
		}
	}
	if access.IsSuper {
		access.HRPolicies = []HRPolicy{{Feature: "*", Action: "*", Scope: ScopeAll, Source: "super_admin"}}
		return access, nil
	}
	if !access.IsActive || access.Employee == nil || access.Employee.EmploymentStatus == "offboarded" || access.Employee.EmploymentStatus == "suspended" {
		return access, nil
	}

	if err := r.loadBusiness(ctx, access); err != nil {
		return nil, err
	}
	if err := r.loadHRPolicies(ctx, access); err != nil {
		return nil, err
	}
	return access, nil
}

func (r *Resolver) loadBusiness(ctx context.Context, access *Access) error {
	rows := []BusinessGrant{}
	err := r.db.SelectContext(ctx, &rows, `
		SELECT pc.capability_key, pc.access_level, 'position' AS source, '' AS effect
		FROM hr_employees e
		JOIN hr_position_capabilities pc ON pc.position_id=e.position_id
		JOIN hr_department_modules dm
		  ON dm.department_id=e.department_id
		 AND dm.module_key=SUBSTRING_INDEX(pc.capability_key,'.',1)
		WHERE e.id=?
		UNION ALL
		SELECT ea.permission_key AS capability_key, ea.access_level,
		       'employee_assignment' AS source, ea.effect
		FROM hr_employee_access_assignments ea
		WHERE ea.employee_id=? AND ea.permission_key NOT LIKE 'hr.%'
		  AND (ea.starts_at IS NULL OR ea.starts_at<=CURRENT_DATE)
		  AND (ea.ends_at IS NULL OR ea.ends_at>=CURRENT_DATE)`, access.Employee.ID, access.Employee.ID)
	if err != nil {
		return err
	}
	merged := map[string]BusinessGrant{}
	denied := map[string]bool{}
	for _, grant := range rows {
		if grant.Effect == "deny" {
			denied[grant.Key] = true
			delete(merged, grant.Key)
			continue
		}
		if denied[grant.Key] {
			continue
		}
		current, ok := merged[grant.Key]
		if !ok || grant.AccessLevel == LevelManage || current.AccessLevel != LevelManage {
			grant.Effect = ""
			merged[grant.Key] = grant
		}
	}
	for _, grant := range merged {
		access.BusinessCapabilities = append(access.BusinessCapabilities, grant)
	}
	sort.Slice(access.BusinessCapabilities, func(i, j int) bool { return access.BusinessCapabilities[i].Key < access.BusinessCapabilities[j].Key })
	return nil
}

func (r *Resolver) loadHRPolicies(ctx context.Context, access *Access) error {
	policies := baselinePolicies()
	if access.IsManager {
		policies = append(policies, managerPolicies(ScopeReportingTree, "manager")...)
	}
	if access.IsDepartmentHead {
		policies = append(policies, managerPolicies(ScopeDepartmentTree, "department_head")...)
	}
	policies = append(policies, legacyPolicies(access.LegacyPermissions)...)

	position := []HRPolicy{}
	if err := r.db.SelectContext(ctx, &position, `
		SELECT hp.feature_key, hp.action_key, hp.employee_scope, 'position' AS source, '' AS effect
		FROM hr_position_hr_policies hp
		WHERE hp.position_id=?`, nullableID(access.Employee.PositionID)); err != nil {
		return err
	}
	policies = append(policies, position...)

	assignments := []HRPolicy{}
	if err := r.db.SelectContext(ctx, &assignments, `
		SELECT SUBSTRING_INDEX(SUBSTRING(permission_key,4),'.',1) AS feature_key,
		       SUBSTRING_INDEX(permission_key,'.',-1) AS action_key,
		       COALESCE(employee_scope,'self') AS employee_scope,
		       'employee_assignment' AS source, effect
		FROM hr_employee_access_assignments
		WHERE employee_id=? AND permission_key LIKE 'hr.%.%'
		  AND (starts_at IS NULL OR starts_at<=CURRENT_DATE)
		  AND (ends_at IS NULL OR ends_at>=CURRENT_DATE)`, access.Employee.ID); err != nil {
		return err
	}
	policies = append(policies, assignments...)

	denied := map[string]bool{}
	for _, policy := range assignments {
		if policy.Effect == "deny" {
			denied[policy.Feature+"\x00"+policy.Action] = true
		}
	}
	seen := map[string]bool{}
	for _, policy := range policies {
		key := policy.Feature + "\x00" + policy.Action + "\x00" + policy.Scope
		if policy.Effect == "deny" {
			continue
		}
		if denied[policy.Feature+"\x00"+policy.Action] || seen[key] {
			continue
		}
		policy.Effect = ""
		seen[key] = true
		access.HRPolicies = append(access.HRPolicies, policy)
	}
	sort.Slice(access.HRPolicies, func(i, j int) bool {
		a, b := access.HRPolicies[i], access.HRPolicies[j]
		if a.Feature != b.Feature {
			return a.Feature < b.Feature
		}
		if a.Action != b.Action {
			return a.Action < b.Action
		}
		return a.Scope < b.Scope
	})
	return nil
}

func baselinePolicies() []HRPolicy {
	return policyRows(ScopeSelf, "employee_self_service", map[string][]string{
		"dashboard": {"view"}, "employees": {"view"},
		"emergency_contacts": {"view"},
		"contracts":          {"view", "download"}, "documents": {"view", "download"},
		"leave": {"view"}, "attendance": {"view"},
		"performance": {"view"}, "expenses": {"view"},
		"compensation": {"view"}, "benefits": {"view"},
	})
}

// Managers cover the complete reporting tree. Department heads cover their
// department and all child departments. Contracts intentionally include the
// complete document view/download actions approved by the product owner.
func managerPolicies(scope, source string) []HRPolicy {
	return policyRows(scope, source, map[string][]string{
		"dashboard": {"view"}, "employees": {"view"},
		"contracts": {"view", "download"}, "leave": {"view"},
		"attendance": {"view"}, "performance": {"view"}, "expenses": {"view"},
	})
}

func policyRows(scope, source string, values map[string][]string) []HRPolicy {
	out := []HRPolicy{}
	for feature, actions := range values {
		for _, action := range actions {
			out = append(out, HRPolicy{Feature: feature, Action: action, Scope: scope, Source: source})
		}
	}
	return out
}

func legacyPolicies(keys []string) []HRPolicy {
	features := map[string][]string{
		"hr_dashboard": {"dashboard"}, "hr_employees": {"employees", "emergency_contacts"},
		"hr_organization": {"organization"}, "hr_lifecycle": {"lifecycle"},
		"hr_leave": {"leave"}, "hr_attendance": {"attendance"},
		"hr_performance": {"performance"}, "hr_recruitment": {"recruitment"},
		"hr_expenses": {"expenses"}, "hr_compensation": {"contracts", "compensation", "benefits"},
		"hr_documents": {"documents"}, "hr_reports": {"reports"},
	}
	out := []HRPolicy{}
	for _, key := range keys {
		if key == "hr" {
			out = append(out, HRPolicy{Feature: "*", Action: "*", Scope: ScopeAll, Source: "legacy_role"})
			continue
		}
		for _, feature := range features[key] {
			out = append(out, HRPolicy{Feature: feature, Action: "*", Scope: ScopeAll, Source: "legacy_role"})
		}
	}
	return out
}

func (a *Access) HasLegacy(key string) bool {
	if a.IsSuper {
		return true
	}
	for _, held := range a.LegacyPermissions {
		if held == key {
			return true
		}
	}
	return false
}

func (a *Access) CanBusiness(key string, manage bool) bool {
	if a.IsSuper {
		return true
	}
	module := strings.SplitN(key, ".", 2)[0]
	if a.HasLegacy(module) {
		return true
	}
	for _, grant := range a.BusinessCapabilities {
		if grant.Key == key && (!manage || grant.AccessLevel == LevelManage) {
			return true
		}
	}
	return false
}

func (a *Access) CanHR(feature, action string) bool {
	if a.IsSuper {
		return true
	}
	for _, policy := range a.HRPolicies {
		if (policy.Feature == feature || policy.Feature == "*") && (policy.Action == action || policy.Action == "*") {
			return true
		}
	}
	return false
}

func (a *Access) Scopes(feature, action string) []string {
	if a.IsSuper {
		return []string{ScopeAll}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, policy := range a.HRPolicies {
		if (policy.Feature == feature || policy.Feature == "*") && (policy.Action == action || policy.Action == "*") && !seen[policy.Scope] {
			seen[policy.Scope] = true
			out = append(out, policy.Scope)
		}
	}
	return out
}

// AllowedEmployeeIDs expands every applicable scope and returns the union.
// unrestricted=true represents the all scope and avoids generating a huge IN
// clause. Both recursive trees include the actor employee.
func (r *Resolver) AllowedEmployeeIDs(ctx context.Context, access *Access, feature, action string) (ids []int64, unrestricted bool, err error) {
	if access.IsSuper {
		return nil, true, nil
	}
	if access.Employee == nil {
		return []int64{}, false, nil
	}
	seen := map[int64]bool{}
	for _, scope := range access.Scopes(feature, action) {
		switch scope {
		case ScopeAll:
			return nil, true, nil
		case ScopeSelf:
			seen[access.Employee.ID] = true
		case ScopeReportingTree:
			rows := []int64{}
			if err := r.db.SelectContext(ctx, &rows, `
				WITH RECURSIVE reporting_tree AS (
					SELECT id FROM hr_employees WHERE id=?
					UNION ALL
					SELECT e.id FROM hr_employees e JOIN reporting_tree rt ON e.manager_id=rt.id
					WHERE e.employment_status<>'offboarded'
				) SELECT id FROM reporting_tree`, access.Employee.ID); err != nil {
				return nil, false, err
			}
			for _, id := range rows {
				seen[id] = true
			}
		case ScopeDepartmentTree:
			rows := []int64{}
			if access.IsDepartmentHead {
				if err := r.db.SelectContext(ctx, &rows, `
				WITH RECURSIVE department_tree AS (
					SELECT id FROM hr_departments WHERE manager_employee_id=? AND is_active=TRUE
					UNION ALL
					SELECT d.id FROM hr_departments d JOIN department_tree dt ON d.parent_department_id=dt.id
				) SELECT e.id FROM hr_employees e JOIN department_tree dt ON dt.id=e.department_id
				WHERE e.employment_status<>'offboarded'`, access.Employee.ID); err != nil {
					return nil, false, err
				}
			} else if access.Employee.DepartmentID != nil {
				// Explicit position/temporary department_tree policies for a
				// non-head are rooted at that employee's department.
				if err := r.db.SelectContext(ctx, &rows, `
				WITH RECURSIVE department_tree AS (
					SELECT id FROM hr_departments WHERE id=?
					UNION ALL
					SELECT d.id FROM hr_departments d JOIN department_tree dt ON d.parent_department_id=dt.id
				) SELECT e.id FROM hr_employees e JOIN department_tree dt ON dt.id=e.department_id
				WHERE e.employment_status<>'offboarded'`, *access.Employee.DepartmentID); err != nil {
					return nil, false, err
				}
			}
			for _, id := range rows {
				seen[id] = true
			}
		}
	}
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, false, nil
}

func (r *Resolver) RecordAccessEvent(ctx context.Context, actorUserID int64, targetEmployeeID *int64, resourceType string, resourceID *int64, action, scopeSource string, metadata any) error {
	var raw any
	if metadata != nil {
		b, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO hr_access_events
		(actor_user_id,target_employee_id,resource_type,resource_id,action_key,scope_source,metadata)
		VALUES (?,?,?,?,?,?,?)`, actorUserID, targetEmployeeID, resourceType, resourceID, action, nullString(scopeSource), raw)
	return err
}

func nullInt(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}
func nullableID(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
