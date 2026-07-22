package hr

import (
	"context"
	"fmt"

	"github.com/trimo/backend/internal/hraccess"
)

// resourceFeature is intentionally broader than the table names. A policy is
// understandable to an HR administrator (leave, attendance, performance) while
// handlers still apply resource-specific self-service safety rules.
func resourceFeature(resource string) string {
	switch resource {
	case "departments", "positions", "contract-templates":
		// Templates are ownerless configuration, not a per-employee record, so
		// they follow the all-scope organization rule rather than `contracts`.
		return "organization"
	case "employees":
		return "employees"
	case "emergency-contacts":
		return "emergency_contacts"
	case "contracts", "contract-documents":
		return "contracts"
	case "documents":
		return "documents"
	case "lifecycle-events":
		return "lifecycle"
	case "leave-policies", "leave-balances", "leave-requests", "leave-calendar":
		return "leave"
	case "shifts", "shift-assignments", "attendance", "timesheets":
		return "attendance"
	case "performance-reviews", "goals", "feedback", "one-to-ones":
		return "performance"
	case "vacancies", "candidates", "interviews", "offers":
		return "recruitment"
	case "expenses":
		return "expenses"
	case "compensation":
		return "compensation"
	case "benefits", "benefit-enrollments":
		return "benefits"
	default:
		return resource
	}
}

func resourceEmployeeColumn(resource string) string {
	switch resource {
	case "employees":
		return "id"
	case "emergency-contacts", "contracts", "contract-documents", "documents", "lifecycle-events", "leave-balances", "leave-requests", "shift-assignments", "attendance", "timesheets", "performance-reviews", "goals", "feedback", "one-to-ones", "expenses", "compensation", "benefit-enrollments":
		return "employee_id"
	default:
		return ""
	}
}

func targetEmployeeID(resource string, item map[string]any) (*int64, bool) {
	column := resourceEmployeeColumn(resource)
	if column == "" {
		return nil, false
	}
	id, ok := mapInt64(item[column])
	if !ok {
		return nil, true
	}
	return &id, true
}

func (s *Service) resolveActor(ctx context.Context, actorUserID int64) (*hraccess.Access, error) {
	access, err := s.access.Resolve(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if !access.IsSuper && (access.Employee == nil || !access.IsActive || access.Employee.EmploymentStatus == "offboarded" || access.Employee.EmploymentStatus == "suspended") {
		return nil, ErrForbidden
	}
	return access, nil
}

func (s *Service) ScopeList(ctx context.Context, actorUserID int64, resource string, opts ListOptions) (ListOptions, error) {
	access, err := s.resolveActor(ctx, actorUserID)
	if err != nil {
		return opts, err
	}
	feature := resourceFeature(resource)
	if !access.CanHR(feature, "view") {
		return opts, ErrForbidden
	}
	if resourceRequiresAllScopeAccess(resource) && !access.IsSuper && !containsString(access.Scopes(feature, "view"), hraccess.ScopeAll) {
		return opts, ErrForbidden
	}
	if resource == contractDocumentsResource && !access.IsSuper &&
		!containsString(access.Scopes(feature, "view"), hraccess.ScopeAll) {
		// Mirrors the per-item rule in AuthorizeItem: below all scope a document
		// only exists once issued. Rows with no employee (external-party memos)
		// are excluded automatically by the employee filter below.
		opts.HideDraftDocuments = true
	}
	if resourceEmployeeColumn(resource) == "" {
		return opts, nil
	}
	ids, unrestricted, err := s.access.AllowedEmployeeIDs(ctx, access, feature, "view")
	if err != nil {
		return opts, err
	}
	opts.RestrictEmployee = !unrestricted
	opts.AllowedEmployeeIDs = ids
	if resource == "feedback" && access.Employee != nil {
		opts.ActorEmployeeID = &access.Employee.ID
		scopes := access.Scopes(feature, "view")
		opts.FeedbackManager = containsString(scopes, hraccess.ScopeReportingTree) || containsString(scopes, hraccess.ScopeDepartmentTree) || containsString(scopes, hraccess.ScopeAll)
		opts.FeedbackPrivate = containsString(scopes, hraccess.ScopeAll)
	}
	// Hide the actor's own confidential documents from listings unless they hold
	// documents access at the all scope. Mirrors the per-item rule in
	// AuthorizeItem so a subject never sees an HR-internal file about themselves.
	if resource == "documents" && access.Employee != nil && !access.IsSuper &&
		!containsString(access.Scopes(feature, "view"), hraccess.ScopeAll) {
		id := access.Employee.ID
		opts.HideConfidentialForEmployeeID = &id
	}
	return opts, nil
}

// AuthorizeItem checks both the feature/action and the target employee scope.
// Scoped read misses become 404 to avoid confirming another employee's record;
// writes use 403 so callers understand that the action itself is disallowed.
func (s *Service) AuthorizeItem(ctx context.Context, actorUserID int64, resource, action string, item map[string]any) (*hraccess.Access, string, error) {
	access, err := s.resolveActor(ctx, actorUserID)
	if err != nil {
		return nil, "", err
	}
	feature := resourceFeature(resource)
	if !access.CanHR(feature, action) {
		return access, "", ErrForbidden
	}
	if !access.IsSuper && resourceRequiresAllScopeAccess(resource) && !containsString(access.Scopes(feature, action), hraccess.ScopeAll) {
		return access, "", ErrForbidden
	}
	if !access.IsSuper && action != "view" && action != "download" && resourceMutationRequiresAllScope(resource) && !containsString(access.Scopes(feature, action), hraccess.ScopeAll) {
		return access, "", ErrForbidden
	}
	target, scoped := targetEmployeeID(resource, item)
	if !scoped {
		return access, "feature", nil
	}
	if target == nil {
		// A memo signed with an external party has no employee at all. Such a row
		// is ownerless, so no employee scope can reach it — only all-scope HR.
		if resource == contractDocumentsResource &&
			(access.IsSuper || containsString(access.Scopes(feature, action), hraccess.ScopeAll)) {
			return access, hraccess.ScopeAll, nil
		}
		return access, "", ErrForbidden
	}
	// An employee sees the contracts addressed to them, but only once issued. A
	// draft is HR's working copy and must not surface as though it were real.
	if resource == contractDocumentsResource && accessStringValue(item["status"]) == "draft" &&
		!access.IsSuper && !containsString(access.Scopes(feature, action), hraccess.ScopeAll) {
		return access, "", ErrNotFound
	}
	if !access.IsSuper && access.Employee != nil && *target == access.Employee.ID && isSeparatedDuty(resource, action) {
		return access, "", ErrForbidden
	}
	if resource == "feedback" && access.Employee != nil {
		visibility := accessStringValue(item["visibility"])
		authorID, authored := mapInt64(item["author_employee_id"])
		scopes := access.Scopes(feature, action)
		allScope := containsString(scopes, hraccess.ScopeAll)
		if visibility == "hr_private" && !allScope {
			if action == "view" || action == "download" {
				return access, "", ErrNotFound
			}
			return access, "", ErrForbidden
		}
		if action != "view" && action != "download" && !allScope && (!authored || authorID != access.Employee.ID) {
			return access, "", ErrForbidden
		}
		if action == "view" && (authorID != access.Employee.ID || !authored) {
			if visibility == "manager" && !containsString(scopes, hraccess.ScopeReportingTree) && !containsString(scopes, hraccess.ScopeDepartmentTree) && !containsString(scopes, hraccess.ScopeAll) {
				return access, "", ErrNotFound
			}
		}
		if *target == access.Employee.ID && authorID != access.Employee.ID && (visibility == "manager" || visibility == "hr_private") {
			return access, "", ErrNotFound
		}
	}
	// Confidential documents are HR-internal, not employee self-service. The
	// subject must never read their own via self scope; only a non-self
	// (manager/HR) relationship over the target unlocks them. For a target that
	// is not the actor, the scope match below already proves such a relationship,
	// so this only has to stop the subject reaching their own confidential files.
	if resource == "documents" && (action == "view" || action == "download") &&
		access.Employee != nil && *target == access.Employee.ID &&
		isTruthy(item["is_confidential"]) &&
		!containsString(access.Scopes(feature, action), hraccess.ScopeAll) {
		return access, "", ErrNotFound
	}
	ids, unrestricted, err := s.access.AllowedEmployeeIDs(ctx, access, feature, action)
	if err != nil {
		return access, "", err
	}
	if unrestricted {
		return access, hraccess.ScopeAll, nil
	}
	for _, id := range ids {
		if id == *target {
			return access, matchingScope(access, *target, feature, action), nil
		}
	}
	if action == "view" || action == "download" {
		return access, "", ErrNotFound
	}
	return access, "", ErrForbidden
}

// Approval and official-completion actions must always have a second person.
// Tree and all scopes include the actor for ordinary read/update convenience,
// so enforce this separation explicitly before scope expansion.
func isSeparatedDuty(resource, action string) bool {
	switch resource {
	case "leave-requests", "timesheets":
		return action == "approve"
	case "expenses":
		return action == "approve" || action == "reimburse"
	case "performance-reviews", "lifecycle-events":
		return action == "complete"
	default:
		return false
	}
}

// Recruitment and organization records do not have an employee owner that can
// be reduced to self/reporting/department scope. Until those domains gain an
// explicit hiring-manager/department relation, only all-scope policies are
// meaningful and safe for reads or writes.
func resourceRequiresAllScopeAccess(resource string) bool {
	switch resourceFeature(resource) {
	case "organization", "recruitment":
		return true
	default:
		return false
	}
}

// Mutating an ownerless configuration object affects everyone, so employee or
// manager scopes cannot authorize it. Leave balances and shift assignments do
// have an employee_id, but are authoritative HR allocations rather than
// employee self-service records and follow the same all-scope rule.
func resourceMutationRequiresAllScope(resource string) bool {
	if resourceEmployeeColumn(resource) == "" {
		return true
	}
	// A contract document is addressed to an employee but is never theirs to
	// write: drafting, issuing, signing and voiding stay with HR.
	return resource == "leave-balances" || resource == "shift-assignments" || resource == contractDocumentsResource
}

func matchingScope(access *hraccess.Access, targetID int64, feature, action string) string {
	if access.Employee != nil && access.Employee.ID == targetID {
		return hraccess.ScopeSelf
	}
	scopes := access.Scopes(feature, action)
	if len(scopes) > 0 {
		return scopes[len(scopes)-1]
	}
	return "policy"
}

func (s *Service) AuthorizeID(ctx context.Context, actorUserID int64, resource, action string, id int64) (*hraccess.Access, string, map[string]any, error) {
	def, ok := resources[resource]
	if !ok {
		return nil, "", nil, ErrNotFound
	}
	item, err := s.repo.Get(ctx, def, id)
	if err != nil {
		return nil, "", nil, err
	}
	access, source, err := s.AuthorizeItem(ctx, actorUserID, resource, action, item)
	return access, source, item, err
}

func (s *Service) AuthorizeInput(ctx context.Context, actorUserID int64, resource, action string, input map[string]any) (*hraccess.Access, string, error) {
	item := map[string]any{}
	column := resourceEmployeeColumn(resource)
	if column != "" {
		value, ok := input[column]
		if !ok && resource == "employees" {
			// A not-yet-created employee has no target scope. Only an all-scope HR
			// administrator/super-admin may create employee identities.
			access, err := s.resolveActor(ctx, actorUserID)
			if err != nil {
				return nil, "", err
			}
			if access.IsSuper || containsString(access.Scopes("employees", action), hraccess.ScopeAll) {
				return access, hraccess.ScopeAll, nil
			}
			return access, "", ErrForbidden
		}
		item[column] = value
	}
	access, source, err := s.AuthorizeItem(ctx, actorUserID, resource, action, item)
	if err != nil {
		return access, source, err
	}
	if !access.IsSuper && resource == "departments" && action == "create" {
		for _, key := range []string{"manager_employee_id", "parent_department_id"} {
			if _, set := input[key]; set {
				return access, source, fmt.Errorf("%w: %s is managed by super-admin", ErrForbidden, key)
			}
		}
	}
	if !access.IsSuper && access.Employee != nil {
		target, _ := targetEmployeeID(resource, item)
		isSelf := target != nil && *target == access.Employee.ID
		if isSelf && (resource == "performance-reviews" || resource == "one-to-ones") {
			return access, source, ErrForbidden
		}
		if resource == "feedback" {
			if author, ok := mapInt64(input["author_employee_id"]); ok && author != access.Employee.ID {
				return access, source, ErrForbidden
			}
			input["author_employee_id"] = float64(access.Employee.ID)
			visibility := accessStringValue(input["visibility"])
			if visibility == "hr_private" && !containsString(access.Scopes("performance", action), hraccess.ScopeAll) {
				return access, source, ErrForbidden
			}
		}
	}
	return access, source, nil
}

func (s *Service) ValidateSafeUpdate(access *hraccess.Access, resource string, target map[string]any, input map[string]any) error {
	if column := resourceEmployeeColumn(resource); column != "" && column != "id" {
		if _, set := input[column]; set {
			return fmt.Errorf("%w: %s is immutable after creation", ErrForbidden, column)
		}
	}
	if resource == "positions" {
		if _, set := input["department_id"]; set {
			return fmt.Errorf("%w: department_id is immutable after position creation", ErrForbidden)
		}
	}
	if resource == "departments" && !access.IsSuper {
		for _, key := range []string{"manager_employee_id", "parent_department_id"} {
			if _, set := input[key]; set {
				return fmt.Errorf("%w: %s is managed by super-admin", ErrForbidden, key)
			}
		}
	}
	if access.IsSuper {
		return nil
	}
	targetID, _ := targetEmployeeID(resource, target)
	isSelf := access.Employee != nil && targetID != nil && access.Employee.ID == *targetID
	if resource == "employees" {
		// Organization/status/user changes are authorization changes. Only the
		// lifecycle/access workflows may make them.
		for _, key := range []string{"user_id", "department_id", "position_id", "manager_id", "employment_status"} {
			if _, set := input[key]; set {
				return fmt.Errorf("%w: %s is an official employment field", ErrForbidden, key)
			}
		}
		if isSelf {
			for _, key := range []string{"employee_number", "work_email", "hire_date", "end_date", "employment_type", "notes"} {
				if _, set := input[key]; set {
					return fmt.Errorf("%w: %s is maintained by HR", ErrForbidden, key)
				}
			}
			allowed := map[string]bool{"first_name": true, "last_name": true, "personal_email": true, "phone": true, "date_of_birth": true, "gender": true, "nationality": true, "address": true, "work_location": true, "photo_url": true}
			for key := range input {
				if !allowed[key] {
					return ErrForbidden
				}
			}
		}
	}
	if isSelf && resource == "performance-reviews" {
		// Employees may add employee_comments through a dedicated future action;
		// they never edit ratings, strengths, improvements, reviewer comments,
		// reviewer identity, periods, or workflow state via generic CRUD.
		return ErrForbidden
	}
	if isSelf && resource == "one-to-ones" {
		return ErrForbidden
	}
	if resource == "feedback" {
		if _, set := input["author_employee_id"]; set {
			return ErrForbidden
		}
		if accessStringValue(input["visibility"]) == "hr_private" && !containsString(access.Scopes("performance", "update"), hraccess.ScopeAll) {
			return ErrForbidden
		}
		if isSelf {
			authorID, ok := mapInt64(target["author_employee_id"])
			if !ok || access.Employee == nil || authorID != access.Employee.ID {
				return ErrForbidden
			}
		}
	}
	return nil
}

// PrepareAttendanceInput prevents scoped users from selecting a convenient
// shift/source to alter computed lateness or overtime. Their assigned active
// shift is derived by RecordAttendance for the effective attendance date.
func (s *Service) PrepareAttendanceInput(access *hraccess.Access, target map[string]any, action string, input map[string]any) error {
	if access == nil || access.IsSuper || containsString(access.Scopes("attendance", action), hraccess.ScopeAll) {
		return nil
	}
	if _, supplied := input["shift_id"]; supplied {
		return fmt.Errorf("%w: shift_id is derived from the active shift assignment", ErrForbidden)
	}
	if _, supplied := input["source"]; supplied {
		return fmt.Errorf("%w: attendance source is server-managed", ErrForbidden)
	}
	input["shift_id"] = nil
	targetEmployee, _ := targetEmployeeID("attendance", target)
	if access.Employee != nil && targetEmployee != nil && access.Employee.ID == *targetEmployee {
		input["source"] = "self_service"
	} else {
		input["source"] = "manager_entry"
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s *Service) AuthorizeReport(ctx context.Context, actorUserID int64, report, action string) error {
	access, err := s.resolveActor(ctx, actorUserID)
	if err != nil {
		return err
	}
	feature := map[string]string{"workforce": "employees", "leave": "leave", "attendance": "attendance", "performance": "performance", "recruitment": "recruitment", "expenses": "expenses", "compensation": "compensation"}[report]
	if feature == "" {
		return ErrNotFound
	}
	if access.IsSuper {
		return nil
	}
	if !access.CanHR("reports", action) || !access.CanHR(feature, "view") {
		return ErrForbidden
	}
	if !containsString(access.Scopes("reports", action), hraccess.ScopeAll) || !containsString(access.Scopes(feature, "view"), hraccess.ScopeAll) {
		return ErrForbidden
	}
	return nil
}
