package hr

import (
	"mime/multipart"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/trimo/backend/internal/hraccess"
	"time"
)

func TestPositionCapabilityInputHasScanTags(t *testing.T) {
	// positionAccess scans `capability_key AS key, access_level` into this struct.
	// sqlx's default mapper lowercases field names without underscores, so without
	// db tags AccessLevel maps to "accesslevel" and every read of a position that
	// has capabilities 500s. Guard the tags so that regression can't return.
	typ := reflect.TypeOf(PositionCapabilityInput{})
	for name, want := range map[string]string{"Key": "key", "AccessLevel": "access_level"} {
		field, _ := typ.FieldByName(name)
		if got := field.Tag.Get("db"); got != want {
			t.Errorf("PositionCapabilityInput.%s needs db:%q for sqlx scanning, got %q", name, want, got)
		}
	}
}

func TestNormalizeInputDatesMoneyAndImmutable(t *testing.T) {
	def := resources["expenses"]
	values, problems := normalizeInput(def, map[string]any{
		"employee_id": 3.0, "expense_date": "2026-07-18", "category": "Travel",
		"description": "Taxi", "amount": "1250.50", "status": "reimbursed",
	}, true)
	if values["amount"] != "1250.50" || values["expense_date"] != "2026-07-18" {
		t.Fatalf("unexpected normalized values: %#v", values)
	}
	if problems["status"] == "" {
		t.Fatal("workflow-controlled status must be rejected on create")
	}

	_, problems = normalizeInput(def, map[string]any{
		"employee_id": 3.0, "expense_date": "18/07/2026", "category": "Travel",
		"description": "Taxi", "amount": "-1",
	}, true)
	if problems["expense_date"] == "" || problems["amount"] == "" {
		t.Fatalf("expected date and money validation errors, got %#v", problems)
	}
}

func TestEmployeeSelfUpdateCannotChangeOfficialAccessFields(t *testing.T) {
	svc := &Service{}
	employeeID := int64(7)
	access := &hraccess.Access{Employee: &hraccess.EmployeeIdentity{ID: employeeID}}
	target := map[string]any{"id": "7"}
	if err := svc.ValidateSafeUpdate(access, "employees", target, map[string]any{"phone": "+261"}); err != nil {
		t.Fatalf("safe personal update rejected: %v", err)
	}
	for _, field := range []string{"user_id", "department_id", "position_id", "manager_id", "employment_status", "notes"} {
		if err := svc.ValidateSafeUpdate(access, "employees", target, map[string]any{field: 1}); err == nil {
			t.Fatalf("official field %s must be protected", field)
		}
	}
	if err := svc.ValidateSafeUpdate(access, "performance-reviews", map[string]any{"employee_id": "7"}, map[string]any{"overall_rating": "5"}); err == nil {
		t.Fatal("employee must not edit their official review")
	}
}

func TestEmployeeOwnershipCannotChangeOnUpdate(t *testing.T) {
	svc := &Service{}
	access := &hraccess.Access{Employee: &hraccess.EmployeeIdentity{ID: 7}}
	for _, resource := range []string{"emergency-contacts", "documents", "attendance", "timesheets", "goals", "feedback", "expenses"} {
		if err := svc.ValidateSafeUpdate(access, resource, map[string]any{"employee_id": "7"}, map[string]any{"employee_id": 8.0}); err == nil {
			t.Errorf("%s must not permit employee_id reassignment", resource)
		}
	}
	super := &hraccess.Access{IsSuper: true}
	if err := svc.ValidateSafeUpdate(super, "expenses", map[string]any{"employee_id": "7"}, map[string]any{"employee_id": 8.0}); err == nil {
		t.Fatal("ownership immutability must also apply to super-admin generic updates")
	}
}

func TestGlobalMutationScopeMatrix(t *testing.T) {
	for _, resource := range []string{"departments", "positions", "leave-policies", "shifts", "benefits", "vacancies", "candidates"} {
		if !resourceMutationRequiresAllScope(resource) {
			t.Errorf("%s mutation must require all scope", resource)
		}
	}
	for _, resource := range []string{"leave-balances", "shift-assignments"} {
		if !resourceMutationRequiresAllScope(resource) {
			t.Errorf("%s authoritative assignment must require all scope", resource)
		}
	}
	for _, resource := range []string{"leave-requests", "attendance", "timesheets", "goals", "feedback", "expenses", "emergency-contacts"} {
		if resourceMutationRequiresAllScope(resource) {
			t.Errorf("%s should retain scoped self-service mutations", resource)
		}
	}
	if !resourceRequiresAllScopeAccess("candidates") || !resourceRequiresAllScopeAccess("positions") {
		t.Fatal("unowned recruitment and organization reads must require all scope")
	}
}

func TestAccessBearingOrganizationFieldsAreProtected(t *testing.T) {
	svc := &Service{}
	ordinary := &hraccess.Access{Employee: &hraccess.EmployeeIdentity{ID: 7}}
	for _, field := range []string{"manager_employee_id", "parent_department_id"} {
		if err := svc.ValidateSafeUpdate(ordinary, "departments", map[string]any{"id": "1"}, map[string]any{field: 7.0}); err == nil {
			t.Errorf("department %s must be super-only", field)
		}
	}
	if err := svc.ValidateSafeUpdate(&hraccess.Access{IsSuper: true}, "positions", map[string]any{"id": "1"}, map[string]any{"department_id": 2.0}); err == nil {
		t.Fatal("position department must be immutable even for generic super update")
	}
}

func TestSuspendedAndOffboardedEmployeesCannotActivateAccounts(t *testing.T) {
	for _, status := range []string{"suspended", "offboarded"} {
		if employmentAllowsAccount(status) {
			t.Errorf("%s employee must not be activatable", status)
		}
	}
	for _, status := range []string{"onboarding", "probation", "active", "leave"} {
		if !employmentAllowsAccount(status) {
			t.Errorf("%s employee should be eligible for an account", status)
		}
	}
}

func TestScopedAttendanceDerivesShiftAndSource(t *testing.T) {
	svc := &Service{}
	access := &hraccess.Access{
		Employee:   &hraccess.EmployeeIdentity{ID: 7},
		HRPolicies: []hraccess.HRPolicy{{Feature: "attendance", Action: "create", Scope: hraccess.ScopeSelf}},
	}
	if err := svc.PrepareAttendanceInput(access, map[string]any{"employee_id": 7.0}, "create", map[string]any{"shift_id": 3.0}); err == nil {
		t.Fatal("scoped attendance must reject client-selected shift")
	}
	input := map[string]any{}
	if err := svc.PrepareAttendanceInput(access, map[string]any{"employee_id": 7.0}, "create", input); err != nil {
		t.Fatalf("server-derived attendance rejected: %v", err)
	}
	if _, ok := input["shift_id"]; !ok || input["shift_id"] != nil || input["source"] != "self_service" {
		t.Fatalf("unexpected prepared attendance: %#v", input)
	}
}

func TestTwoStageLeaveApprovalDoesNotLetManagerBecomeHRAdmin(t *testing.T) {
	request := leaveRequestLock{Current: 1, ApprovalLevels: []byte(`["manager","hr_admin"]`)}
	stage, _, err := leaveApproverRequirement(request)
	if err != nil || stage != "manager" {
		t.Fatalf("first approval stage=%q err=%v", stage, err)
	}
	request.Current = 2
	stage, _, err = leaveApproverRequirement(request)
	if err != nil || stage != "hr_admin" {
		t.Fatalf("second approval stage=%q err=%v", stage, err)
	}
	manager := &hraccess.Access{HRPolicies: []hraccess.HRPolicy{{Feature: "leave", Action: "approve", Scope: hraccess.ScopeReportingTree}}}
	if canApproveLeaveHRStage(manager, true) {
		t.Fatal("reporting-tree manager must not approve the HR-admin stage")
	}
	head := &hraccess.Access{HRPolicies: []hraccess.HRPolicy{{Feature: "leave", Action: "approve", Scope: hraccess.ScopeDepartmentTree}}}
	if !canApproveLeaveHRStage(head, false) || canApproveLeaveHRStage(head, true) {
		t.Fatal("department-tree approval should cover HR but not HR-admin stage")
	}
	admin := &hraccess.Access{HRPolicies: []hraccess.HRPolicy{{Feature: "leave", Action: "approve", Scope: hraccess.ScopeAll}}}
	if !canApproveLeaveHRStage(admin, true) {
		t.Fatal("all-scope leave approver must cover HR-admin stage")
	}
}

func TestEmployeeLifecycleFieldsAreCreateOnly(t *testing.T) {
	def := resources["employees"]
	// Employment status and end date stay workflow-controlled (lifecycle only).
	_, problems := normalizeInput(def, map[string]any{"employment_status": "offboarded", "end_date": "2026-07-18"}, false)
	for _, field := range []string{"employment_status", "end_date"} {
		if problems[field] == "" {
			t.Errorf("%s must be rejected on direct employee update", field)
		}
	}
	// Department, position, and manager are now directly correctable (an admin
	// reassignment); ValidateSafeUpdate still restricts who may set them.
	values, problems := normalizeInput(def, map[string]any{"department_id": 2.0, "position_id": 3.0, "manager_id": 4.0}, false)
	for _, field := range []string{"department_id", "position_id", "manager_id"} {
		if problems[field] != "" {
			t.Errorf("%s should be updatable on reassignment, got %q", field, problems[field])
		}
		if _, ok := values[field]; !ok {
			t.Errorf("%s should survive normalization on update", field)
		}
	}
	_, problems = normalizeInput(def, map[string]any{"employee_number": "EMP-1", "first_name": "A", "last_name": "B", "work_email": "a@example.com", "hire_date": "2026-01-01", "department_id": 2.0, "employment_status": "onboarding"}, true)
	if problems["department_id"] != "" || problems["employment_status"] != "" {
		t.Fatalf("initial lifecycle fields should be accepted on create: %#v", problems)
	}
}

func TestBusinessLeaveDays(t *testing.T) {
	friday, _ := time.Parse("2006-01-02", "2026-07-17")
	monday, _ := time.Parse("2006-01-02", "2026-07-20")
	if got := businessLeaveDays(friday, monday, "full", "full"); got != 2 {
		t.Fatalf("weekend should be excluded; got %v", got)
	}
	if got := businessLeaveDays(friday, friday, "half", "half"); got != .5 {
		t.Fatalf("same-day half leave should be .5; got %v", got)
	}
}

func TestLeaveAccrualModes(t *testing.T) {
	asOf, _ := time.Parse("2006-01-02", "2026-07-18")
	if got := leaveAllocation(leavePolicyRow{Days: "24", AccrualMode: "annual"}, asOf, "2026-03-01"); got != 24 {
		t.Fatalf("annual allocation=%v", got)
	}
	if got := leaveAllocation(leavePolicyRow{Days: "24", AccrualMode: "manual"}, asOf, "2026-03-01"); got != 0 {
		t.Fatalf("manual allocation=%v", got)
	}
	if got := leaveAllocation(leavePolicyRow{Days: "24", AccrualMode: "monthly"}, asOf, "2026-03-01"); got != 10 {
		t.Fatalf("March-July monthly allocation=%v", got)
	}
}

func TestTerminalResourcesAreFrozen(t *testing.T) {
	for _, tc := range []struct{ resource, status string }{{"expenses", "reimbursed"}, {"timesheets", "approved"}, {"offers", "accepted"}, {"candidates", "hired"}, {"performance-reviews", "completed"}, {"interviews", "completed"}, {"one-to-ones", "completed"}} {
		if resourceEditable(tc.resource, map[string]any{"status": tc.status}) {
			t.Errorf("%s/%s should be frozen", tc.resource, tc.status)
		}
	}
	if !resourceEditable("contracts", map[string]any{"status": "active"}) {
		t.Fatal("active contracts must remain amendable/terminable by authorized HR")
	}
	for _, status := range []string{"expired", "terminated"} {
		if resourceEditable("contracts", map[string]any{"status": status}) {
			t.Errorf("contract/%s should be frozen", status)
		}
	}
}

func TestSignedLeaveAdjustment(t *testing.T) {
	def := resources["leave-balances"]
	values, problems := normalizeInput(def, map[string]any{"employee_id": 1.0, "policy_id": 2.0, "balance_year": 2026.0, "allocated_days": "10", "adjustment_days": "-2.50"}, true)
	if problems["adjustment_days"] != "" || values["adjustment_days"] != "-2.50" {
		t.Fatalf("signed adjustment rejected: %#v %#v", values, problems)
	}
}

func TestParseAttendanceTimestampAcceptsAPIAndDatabaseFormats(t *testing.T) {
	for _, value := range []string{"2026-07-18T09:15:00Z", "2026-07-18 09:15:00"} {
		got, present, err := parseAttendanceTimestamp(value)
		if err != nil || !present || got.UTC().Format("2006-01-02 15:04:05") != "2026-07-18 09:15:00" {
			t.Fatalf("could not parse %q: %v %v %v", value, got, present, err)
		}
	}
	if _, present, err := parseAttendanceTimestamp(nil); err != nil || present {
		t.Fatalf("nil timestamp should be absent, got present=%v err=%v", present, err)
	}
	if _, _, err := parseAttendanceTimestamp("not-a-time"); err == nil {
		t.Fatal("invalid timestamp must fail")
	}
}

func TestApprovalLevelParsing(t *testing.T) {
	if got, err := approvalLevelCount([]byte(`[{"approver":"manager"},{"approver":"hr"}]`)); err != nil || got != 2 {
		t.Fatalf("expected two valid levels, got %d, %v", got, err)
	}
	for _, invalid := range []string{`[]`, `{}`, `null`} {
		if _, err := approvalLevelCount([]byte(invalid)); err == nil {
			t.Fatalf("expected %s to be rejected", invalid)
		}
	}
}

func TestIsTruthyHandlesMapScanForms(t *testing.T) {
	// MySQL BOOLEAN (TINYINT(1)) comes back through MapScan as bytes/int, so the
	// confidential-document check must treat each form consistently.
	for _, truthy := range []any{true, int64(1), int(1), 1.0, []byte("1"), "1", []byte("true"), "TRUE"} {
		if !isTruthy(truthy) {
			t.Errorf("expected %#v to be truthy", truthy)
		}
	}
	for _, falsy := range []any{false, int64(0), 0.0, []byte("0"), "0", "", nil, []byte("")} {
		if isTruthy(falsy) {
			t.Errorf("expected %#v to be falsy", falsy)
		}
	}
}

func TestReportColumnsCoverEveryReport(t *testing.T) {
	// exportReport falls back to sorted keys only for unknown reports; every named
	// report should have a curated column order so CSV output stays stable.
	for _, name := range []string{"workforce", "leave", "attendance", "recruitment", "expenses", "performance", "compensation"} {
		if len(reportColumns[name]) == 0 {
			t.Errorf("report %q has no curated CSV column order", name)
		}
	}
}

func TestPositionHierarchyDepth(t *testing.T) {
	// The depth of a position_hierarchy step is its ordinal among position_hierarchy
	// steps in the chain, so successive steps escalate one rung up at a time and
	// other approver types don't shift the count.
	levels := []any{
		"manager",
		"position_hierarchy",
		map[string]any{"approver": "hr"},
		map[string]any{"approver": "position_hierarchy"},
	}
	cases := map[int]int{0: 0, 1: 1, 2: 1, 3: 2}
	for index, want := range cases {
		if got := positionHierarchyDepth(levels, index); got != want {
			t.Errorf("positionHierarchyDepth(levels, %d) = %d, want %d", index, got, want)
		}
	}
	if got := approverOfLevel(map[string]any{"approver": "position_hierarchy"}); got != "position_hierarchy" {
		t.Errorf("approverOfLevel(object) = %q, want position_hierarchy", got)
	}
	if got := approverOfLevel("manager"); got != "manager" {
		t.Errorf("approverOfLevel(string) = %q, want manager", got)
	}
}

func TestResourceRegistryMatchesMigration(t *testing.T) {
	// Read every migration from the HR suite onward rather than naming them one
	// by one: the hardcoded list silently went stale each time a migration was
	// added, and the failure surfaced as a confusing "table missing" error.
	paths, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, path := range paths {
		if filepath.Base(path) < "000029" {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		all.WriteString(string(body) + "\n")
	}
	// CREATE TABLE blocks and later ALTER ... ADD COLUMN both count as schema.
	sqlText := all.String()
	alterText := sqlText
	seenPaths := map[string]bool{}
	for key, def := range resources {
		if seenPaths[def.Path] {
			t.Fatalf("duplicate resource path %q", def.Path)
		}
		seenPaths[def.Path] = true
		start := strings.Index(sqlText, "CREATE TABLE "+def.Table+" (")
		if start < 0 {
			t.Fatalf("resource %s table %s missing from migration", key, def.Table)
		}
		end := strings.Index(sqlText[start:], ") ENGINE=InnoDB")
		if end < 0 {
			t.Fatalf("could not isolate schema for %s", def.Table)
		}
		segment := sqlText[start : start+end]
		for _, field := range def.Fields {
			if !strings.Contains(segment, field.Name) && !strings.Contains(alterText, "ADD COLUMN "+field.Name) {
				t.Errorf("resource %s field %s missing from %s", key, field.Name, def.Table)
			}
		}
	}
}

func TestEmployeeManagerForeignKeyHasNoConflictingCheck(t *testing.T) {
	migration, err := os.ReadFile("../../migrations/000029_hr_management.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	// MySQL rejects a CHECK that reads manager_id when the same self-referencing
	// foreign key uses ON DELETE SET NULL (Error 3823). The service layer already
	// rejects self-management and longer reporting cycles.
	if strings.Contains(string(migration), "manager_id <> id") {
		t.Fatal("manager_id CHECK conflicts with the self-referencing SET NULL foreign key")
	}
}

func TestSafeFilenameAndDocumentMIME(t *testing.T) {
	if got := safeFilename("../bad\r\n\"name.pdf"); strings.ContainsAny(got, "\r\n\"") || strings.Contains(got, "..") {
		t.Fatalf("unsafe filename survived: %q", got)
	}
	pdf := []byte("%PDF-1.7\n")
	name, mime, ok := validatedDocument(&multipart.FileHeader{Filename: "review.pdf"}, pdf)
	if !ok || name != "review.pdf" || mime != "application/pdf" {
		t.Fatalf("valid PDF rejected: %q %q %v", name, mime, ok)
	}
	if _, _, ok := validatedDocument(&multipart.FileHeader{Filename: "payload.exe"}, []byte("MZ executable")); ok {
		t.Fatal("executable upload must be rejected")
	}
}
