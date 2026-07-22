package hr

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"

	"github.com/trimo/backend/internal/auth"
	"github.com/trimo/backend/internal/authz"
	"github.com/trimo/backend/internal/hraccess"
)

type DepartmentAccess struct {
	DepartmentID int64    `json:"department_id"`
	Modules      []string `json:"modules"`
}

type PositionCapabilityInput struct {
	// db tags are required: this struct is scanned from `capability_key AS key,
	// access_level`, and sqlx's default mapper lowercases field names without
	// inserting underscores (AccessLevel -> "accesslevel"), so access_level would
	// otherwise fail to map once a position has any capability row.
	Key         string `json:"key" db:"key"`
	AccessLevel string `json:"access_level" db:"access_level"`
}

type PositionPolicyInput struct {
	Feature string   `json:"feature"`
	Actions []string `json:"actions"`
	Scope   string   `json:"scope"`
}

type PositionAccessRequest struct {
	Capabilities []PositionCapabilityInput `json:"capabilities"`
	HRPolicies   []PositionPolicyInput     `json:"hr_policies"`
}

type PositionAccess struct {
	PositionID   int64                     `json:"position_id"`
	Capabilities []PositionCapabilityInput `json:"capabilities"`
	HRPolicies   []PositionPolicyInput     `json:"hr_policies"`
}

type AccountRequest struct {
	TemporaryPassword string `json:"temporary_password"`
	IsActive          *bool  `json:"is_active"`
}

type EmployeeAccount struct {
	UserID    *int64  `json:"user_id,omitempty" db:"user_id"`
	Email     string  `json:"email" db:"email"`
	IsActive  bool    `json:"is_active" db:"is_active"`
	Status    string  `json:"status" db:"status"`
	LastLogin *string `json:"last_login,omitempty"`
}

type EmployeeAccessOverview struct {
	Employee              map[string]any            `json:"employee"`
	Account               EmployeeAccount           `json:"account"`
	DepartmentModules     []string                  `json:"department_modules"`
	PositionCapabilities  []PositionCapabilityInput `json:"position_capabilities"`
	PositionHRPolicies    []PositionPolicyInput     `json:"position_hr_policies"`
	EffectiveCapabilities []hraccess.BusinessGrant  `json:"effective_business_capabilities"`
	EffectiveHRPolicies   []hraccess.HRPolicy       `json:"effective_hr_policies"`
	IsManager             bool                      `json:"is_manager"`
	IsDepartmentHead      bool                      `json:"is_department_head"`
}

func (s *Service) AccessCatalog() map[string]any {
	return map[string]any{
		"modules":       authz.BusinessModules,
		"access_levels": []string{hraccess.LevelRead, hraccess.LevelManage},
		"hr_features":   hraccess.Features,
		"scopes":        []string{hraccess.ScopeSelf, hraccess.ScopeReportingTree, hraccess.ScopeDepartmentTree, hraccess.ScopeAll},
	}
}

func (s *Service) requireSuper(ctx context.Context, actorUserID int64) (*hraccess.Access, error) {
	access, err := s.access.Resolve(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if !access.IsSuper {
		return nil, ErrForbidden
	}
	return access, nil
}

func (s *Service) GetDepartmentAccess(ctx context.Context, actorUserID, departmentID int64) (*DepartmentAccess, error) {
	if _, err := s.requireSuper(ctx, actorUserID); err != nil {
		return nil, err
	}
	var exists bool
	if err := s.repo.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM hr_departments WHERE id=?)`, departmentID); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	modules := []string{}
	if err := s.repo.db.SelectContext(ctx, &modules, `SELECT module_key FROM hr_department_modules WHERE department_id=? ORDER BY module_key`, departmentID); err != nil {
		return nil, err
	}
	return &DepartmentAccess{DepartmentID: departmentID, Modules: modules}, nil
}

func (s *Service) SetDepartmentAccess(ctx context.Context, actorUserID, departmentID int64, modules []string) (*DepartmentAccess, map[string]string, error) {
	if _, err := s.requireSuper(ctx, actorUserID); err != nil {
		return nil, nil, err
	}
	valid := map[string]bool{}
	for _, module := range authz.BusinessModules {
		valid[module.Key] = true
	}
	clean := []string{}
	seen := map[string]bool{}
	for _, module := range modules {
		module = strings.TrimSpace(module)
		if !valid[module] {
			return nil, map[string]string{"modules": "contains unknown module: " + module}, nil
		}
		if !seen[module] {
			seen[module] = true
			clean = append(clean, module)
		}
	}
	sort.Strings(clean)
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var exists bool
		if err := tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM hr_departments WHERE id=?)`, departmentID); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM hr_department_modules WHERE department_id=?`, departmentID); err != nil {
			return err
		}
		for _, module := range clean {
			if _, err := tx.ExecContext(ctx, `INSERT INTO hr_department_modules(department_id,module_key) VALUES (?,?)`, departmentID, module); err != nil {
				return err
			}
		}
		// Narrowing a department boundary also removes now-invalid grants from
		// every position in that department. Keeping them merely hidden in the
		// resolver would leave stale access that can unexpectedly return later.
		if _, err := tx.ExecContext(ctx, `
			DELETE pc FROM hr_position_capabilities pc
			JOIN hr_positions p ON p.id=pc.position_id
			LEFT JOIN hr_department_modules dm
			  ON dm.department_id=p.department_id
			 AND dm.module_key=SUBSTRING_INDEX(pc.capability_key,'.',1)
			WHERE p.department_id=? AND dm.department_id IS NULL`, departmentID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	_ = s.access.RecordAccessEvent(ctx, actorUserID, nil, "department_access", &departmentID, "manage", "super_admin", map[string]any{"modules": clean})
	return &DepartmentAccess{DepartmentID: departmentID, Modules: clean}, nil, nil
}

func (s *Service) GetPositionAccess(ctx context.Context, actorUserID, positionID int64) (*PositionAccess, error) {
	if _, err := s.requireSuper(ctx, actorUserID); err != nil {
		return nil, err
	}
	return s.positionAccess(ctx, positionID)
}

func (s *Service) positionAccess(ctx context.Context, positionID int64) (*PositionAccess, error) {
	var exists bool
	if err := s.repo.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM hr_positions WHERE id=?)`, positionID); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	capabilities := []PositionCapabilityInput{}
	if err := s.repo.db.SelectContext(ctx, &capabilities, `SELECT capability_key AS `+"`key`"+`, access_level FROM hr_position_capabilities WHERE position_id=? ORDER BY capability_key`, positionID); err != nil {
		return nil, err
	}
	type row struct {
		Feature string `db:"feature_key"`
		Action  string `db:"action_key"`
		Scope   string `db:"employee_scope"`
	}
	rows := []row{}
	if err := s.repo.db.SelectContext(ctx, &rows, `SELECT feature_key,action_key,employee_scope FROM hr_position_hr_policies WHERE position_id=? ORDER BY feature_key,employee_scope,action_key`, positionID); err != nil {
		return nil, err
	}
	grouped := map[string]*PositionPolicyInput{}
	keys := []string{}
	for _, item := range rows {
		key := item.Feature + "\x00" + item.Scope
		if grouped[key] == nil {
			grouped[key] = &PositionPolicyInput{Feature: item.Feature, Scope: item.Scope}
			keys = append(keys, key)
		}
		grouped[key].Actions = append(grouped[key].Actions, item.Action)
	}
	policies := []PositionPolicyInput{}
	for _, key := range keys {
		policies = append(policies, *grouped[key])
	}
	return &PositionAccess{PositionID: positionID, Capabilities: capabilities, HRPolicies: policies}, nil
}

func (s *Service) SetPositionAccess(ctx context.Context, actorUserID, positionID int64, req PositionAccessRequest) (*PositionAccess, map[string]string, error) {
	if _, err := s.requireSuper(ctx, actorUserID); err != nil {
		return nil, nil, err
	}
	var departmentID sql.NullInt64
	if err := s.repo.db.GetContext(ctx, &departmentID, `SELECT department_id FROM hr_positions WHERE id=?`, positionID); errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	} else if err != nil {
		return nil, nil, err
	}
	allowedModules := map[string]bool{}
	if departmentID.Valid {
		modules := []string{}
		if err := s.repo.db.SelectContext(ctx, &modules, `SELECT module_key FROM hr_department_modules WHERE department_id=?`, departmentID.Int64); err != nil {
			return nil, nil, err
		}
		for _, module := range modules {
			allowedModules[module] = true
		}
	}
	cleanCaps := []PositionCapabilityInput{}
	seenCaps := map[string]bool{}
	for _, capability := range req.Capabilities {
		capability.Key = strings.TrimSpace(capability.Key)
		capability.AccessLevel = strings.TrimSpace(capability.AccessLevel)
		if !authz.IsCapabilityKey(capability.Key) {
			return nil, map[string]string{"capabilities": "contains unknown capability: " + capability.Key}, nil
		}
		module, _ := authz.CapabilityModule(capability.Key)
		if !allowedModules[module] {
			return nil, map[string]string{"capabilities": capability.Key + " is outside the department module scope"}, nil
		}
		if capability.AccessLevel != hraccess.LevelRead && capability.AccessLevel != hraccess.LevelManage {
			return nil, map[string]string{"capabilities": "access_level must be read or manage"}, nil
		}
		if !seenCaps[capability.Key] {
			seenCaps[capability.Key] = true
			cleanCaps = append(cleanCaps, capability)
		}
	}
	validFeatures := map[string]map[string]bool{}
	featureScopes := map[string]map[string]bool{}
	for _, feature := range hraccess.Features {
		actions := map[string]bool{}
		for _, action := range feature.Actions {
			actions[action] = true
		}
		validFeatures[feature.Key] = actions
		scopes := map[string]bool{}
		for _, scope := range feature.Scopes {
			scopes[scope] = true
		}
		featureScopes[feature.Key] = scopes
	}
	cleanPolicies := []PositionPolicyInput{}
	for _, policy := range req.HRPolicies {
		policy.Feature = strings.TrimSpace(policy.Feature)
		policy.Scope = strings.TrimSpace(policy.Scope)
		actions, ok := validFeatures[policy.Feature]
		if !ok {
			return nil, map[string]string{"hr_policies": "contains unknown feature: " + policy.Feature}, nil
		}
		if !featureScopes[policy.Feature][policy.Scope] {
			return nil, map[string]string{"hr_policies": policy.Feature + " does not support scope " + policy.Scope}, nil
		}
		seen := map[string]bool{}
		cleanActions := []string{}
		for _, action := range policy.Actions {
			action = strings.TrimSpace(action)
			if !actions[action] {
				return nil, map[string]string{"hr_policies": "contains invalid action " + action + " for " + policy.Feature}, nil
			}
			if !seen[action] {
				seen[action] = true
				cleanActions = append(cleanActions, action)
			}
		}
		if len(cleanActions) > 0 {
			sort.Strings(cleanActions)
			policy.Actions = cleanActions
			cleanPolicies = append(cleanPolicies, policy)
		}
	}
	sort.Slice(cleanCaps, func(i, j int) bool { return cleanCaps[i].Key < cleanCaps[j].Key })
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM hr_position_capabilities WHERE position_id=?`, positionID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM hr_position_hr_policies WHERE position_id=?`, positionID); err != nil {
			return err
		}
		for _, capability := range cleanCaps {
			if _, err := tx.ExecContext(ctx, `INSERT INTO hr_position_capabilities(position_id,capability_key,access_level) VALUES (?,?,?)`, positionID, capability.Key, capability.AccessLevel); err != nil {
				return err
			}
		}
		for _, policy := range cleanPolicies {
			for _, action := range policy.Actions {
				if _, err := tx.ExecContext(ctx, `INSERT INTO hr_position_hr_policies(position_id,feature_key,action_key,employee_scope) VALUES (?,?,?,?)`, positionID, policy.Feature, action, policy.Scope); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	_ = s.access.RecordAccessEvent(ctx, actorUserID, nil, "position_access", &positionID, "manage", "super_admin", map[string]any{"capabilities": cleanCaps, "hr_policies": cleanPolicies})
	result, err := s.positionAccess(ctx, positionID)
	return result, nil, err
}

func (s *Service) EmployeeAccess(ctx context.Context, actorUserID, employeeID int64) (*EmployeeAccessOverview, error) {
	actor, err := s.resolveActor(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if !actor.IsSuper && (actor.Employee == nil || actor.Employee.ID != employeeID) {
		return nil, ErrForbidden
	}
	employee, err := s.repo.Get(ctx, resources["employees"], employeeID)
	if err != nil {
		return nil, err
	}
	account := EmployeeAccount{Email: accessStringValue(employee["work_email"]), Status: "not_provisioned"}
	if userID, ok := mapInt64(employee["user_id"]); ok {
		var row struct {
			UserID   int64  `db:"user_id"`
			Email    string `db:"email"`
			IsActive bool   `db:"is_active"`
		}
		if err := s.repo.db.GetContext(ctx, &row, `SELECT id AS user_id,email,is_active FROM users WHERE id=?`, userID); err == nil {
			account.UserID = &row.UserID
			account.Email = row.Email
			account.IsActive = row.IsActive && employmentAllowsAccount(accessStringValue(employee["employment_status"]))
			if account.IsActive {
				account.Status = "active"
			} else {
				account.Status = "disabled"
			}
		}
	}
	modules := []string{}
	positionAccess := &PositionAccess{}
	if departmentID, ok := mapInt64(employee["department_id"]); ok {
		_ = s.repo.db.SelectContext(ctx, &modules, `SELECT module_key FROM hr_department_modules WHERE department_id=? ORDER BY module_key`, departmentID)
	}
	if positionID, ok := mapInt64(employee["position_id"]); ok {
		positionAccess, _ = s.positionAccess(ctx, positionID)
	}
	overview := &EmployeeAccessOverview{Employee: employee, Account: account, DepartmentModules: modules, PositionCapabilities: positionAccess.Capabilities, PositionHRPolicies: positionAccess.HRPolicies}
	if account.UserID != nil {
		resolved, err := s.access.Resolve(ctx, *account.UserID)
		if err != nil {
			return nil, err
		}
		overview.EffectiveCapabilities = resolved.BusinessCapabilities
		overview.EffectiveHRPolicies = resolved.HRPolicies
		overview.IsManager = resolved.IsManager
		overview.IsDepartmentHead = resolved.IsDepartmentHead
	}
	return overview, nil
}

func (s *Service) ProvisionEmployeeAccount(ctx context.Context, actorUserID, employeeID int64, req AccountRequest) (*EmployeeAccount, map[string]string, error) {
	if _, err := s.requireSuper(ctx, actorUserID); err != nil {
		return nil, nil, err
	}
	password := req.TemporaryPassword
	if password != "" && (utf8.RuneCountInString(password) < 8 || len(password) > 72) {
		return nil, map[string]string{"temporary_password": "must be between 8 characters and 72 bytes"}, nil
	}
	var account EmployeeAccount
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var employee struct {
			UserID    sql.NullInt64  `db:"user_id"`
			Email     string         `db:"work_email"`
			FirstName string         `db:"first_name"`
			LastName  string         `db:"last_name"`
			Phone     sql.NullString `db:"phone"`
			Status    string         `db:"employment_status"`
		}
		if err := tx.GetContext(ctx, &employee, `SELECT user_id,work_email,first_name,last_name,phone,employment_status FROM hr_employees WHERE id=? FOR UPDATE`, employeeID); err != nil {
			return mapDBError(err)
		}
		active := employmentAllowsAccount(employee.Status)
		if req.IsActive != nil {
			active = *req.IsActive
		}
		if !employmentAllowsAccount(employee.Status) && active {
			return fmt.Errorf("%w: suspended or offboarded employee account cannot be activated", ErrInvalidTransition)
		}
		userID := int64(0)
		if employee.UserID.Valid {
			userID = employee.UserID.Int64
		} else {
			var existing struct {
				ID   int64  `db:"id"`
				Role string `db:"role"`
			}
			err := tx.GetContext(ctx, &existing, `SELECT id,role FROM users WHERE email=? LIMIT 1 FOR UPDATE`, strings.ToLower(strings.TrimSpace(employee.Email)))
			if errors.Is(err, sql.ErrNoRows) {
				if password == "" {
					return &accountPasswordRequiredError{}
				}
				hash, err := auth.HashPassword(password)
				if err != nil {
					return err
				}
				result, err := tx.ExecContext(ctx, `INSERT INTO users(role,is_super_admin,admin_role_id,email,password_hash,full_name,phone,is_active) VALUES ('admin',FALSE,NULL,?,?,?,?,?)`, strings.ToLower(strings.TrimSpace(employee.Email)), hash, strings.TrimSpace(employee.FirstName+" "+employee.LastName), nullableString(employee.Phone), active)
				if err != nil {
					return mapDBError(err)
				}
				userID, err = result.LastInsertId()
				if err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				userID = existing.ID
				// A public customer knows the account's current password. Promotion
				// therefore requires a super-admin supplied replacement password;
				// all refresh sessions are revoked below.
				if existing.Role != "admin" && password == "" {
					return &accountPasswordRequiredError{}
				}
			}
			var linkedCount int
			if err := tx.GetContext(ctx, &linkedCount, `SELECT COUNT(*) FROM hr_employees WHERE user_id=? AND id<>?`, userID, employeeID); err != nil {
				return err
			}
			if linkedCount > 0 {
				return fmt.Errorf("%w: account is already linked to another employee", ErrConflict)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hr_employees SET user_id=? WHERE id=?`, userID, employeeID); err != nil {
				return mapDBError(err)
			}
		}
		if req.IsActive == nil {
			if err := tx.GetContext(ctx, &active, `SELECT is_active FROM users WHERE id=?`, userID); err != nil {
				return err
			}
		}
		if !employmentAllowsAccount(employee.Status) && active {
			return fmt.Errorf("%w: suspended or offboarded employee account cannot be activated", ErrInvalidTransition)
		}
		var identity struct {
			IsSuper bool   `db:"is_super_admin"`
			Role    string `db:"role"`
		}
		if err := tx.GetContext(ctx, &identity, `SELECT is_super_admin,role FROM users WHERE id=?`, userID); err != nil {
			return err
		}
		if identity.IsSuper {
			return ErrForbidden
		}
		if identity.Role != "admin" && password == "" {
			return &accountPasswordRequiredError{}
		}
		var hash any = nil
		if password != "" {
			value, err := auth.HashPassword(password)
			if err != nil {
				return err
			}
			hash = value
		}
		if hash != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE users SET role='admin',password_hash=?,full_name=?,phone=?,is_active=? WHERE id=?`, hash, strings.TrimSpace(employee.FirstName+" "+employee.LastName), nullableString(employee.Phone), active, userID); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE users SET role='admin',full_name=?,phone=?,is_active=? WHERE id=?`, strings.TrimSpace(employee.FirstName+" "+employee.LastName), nullableString(employee.Phone), active, userID); err != nil {
				return err
			}
		}
		if !active || password != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=COALESCE(revoked_at,NOW()) WHERE user_id=?`, userID); err != nil {
				return err
			}
		}
		account = EmployeeAccount{UserID: &userID, Email: strings.ToLower(strings.TrimSpace(employee.Email)), IsActive: active}
		if active {
			account.Status = "active"
		} else {
			account.Status = "disabled"
		}
		return nil
	})
	var required *accountPasswordRequiredError
	if errors.As(err, &required) {
		return nil, map[string]string{"temporary_password": "is required when creating or promoting an account"}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	_ = s.access.RecordAccessEvent(ctx, actorUserID, &employeeID, "employee_account", account.UserID, "manage", "super_admin", map[string]any{"is_active": account.IsActive, "password_reset": password != ""})
	return &account, nil, nil
}

type accountPasswordRequiredError struct{}

func employmentAllowsAccount(status string) bool {
	return status != "offboarded" && status != "suspended"
}

func (*accountPasswordRequiredError) Error() string { return "temporary password required" }

func nullableString(value sql.NullString) any {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	return strings.TrimSpace(value.String)
}
func accessStringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return ""
}
func mapInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		if v == float64(int64(v)) {
			return int64(v), true
		}
	case int64:
		return v, true
	case int:
		return int64(v), true
	case uint64:
		if v <= uint64(^uint64(0)>>1) {
			return int64(v), true
		}
	case string:
		var n int64
		if _, err := fmt.Sscan(v, &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

func isSensitiveReadResource(resource string) bool {
	switch resource {
	case "contracts", "contract-documents", "documents", "compensation":
		return true
	default:
		return false
	}
}
