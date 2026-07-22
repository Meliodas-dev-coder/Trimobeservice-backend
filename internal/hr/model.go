// Package hr implements Trimo's native HR information system. HR employees are
// deliberately independent from authentication users; the nullable user_id
// bridge is reserved for a later employee self-service portal.
package hr

import "errors"

var (
	ErrNotFound          = errors.New("HR resource not found")
	ErrConflict          = errors.New("HR resource conflicts with an existing record")
	ErrReferenced        = errors.New("HR resource is referenced by another record")
	ErrInvalidTransition = errors.New("invalid HR workflow transition")
	ErrInsufficientLeave = errors.New("insufficient leave balance")
	ErrLeaveOverlap      = errors.New("leave request overlaps an existing request")
	ErrApprovalOrder     = errors.New("leave approval levels must be completed in order")
	ErrForbidden         = errors.New("not authorized for this HR workflow step")
	ErrValidation        = errors.New("invalid HR request")
)

type fieldKind string

const (
	kString   fieldKind = "string"
	kInt      fieldKind = "int"
	kBool     fieldKind = "bool"
	kDate     fieldKind = "date"
	kDateTime fieldKind = "datetime"
	kDecimal  fieldKind = "decimal"
	kJSON     fieldKind = "json"
)

type fieldDef struct {
	Name       string
	Kind       fieldKind
	Required   bool
	Immutable  bool
	CreateOnly bool
	Allowed    []string
}

type resourceDef struct {
	Path      string
	Table     string
	Envelope  string
	Singular  string
	Fields    []fieldDef
	Search    []string
	Filters   []string
	NoDelete  bool
	NoUpdate  bool
	NoCreate  bool
	NoUpdated bool
	OrderBy   string
}

func f(name string, kind fieldKind) fieldDef { return fieldDef{Name: name, Kind: kind} }
func req(name string, kind fieldKind) fieldDef {
	return fieldDef{Name: name, Kind: kind, Required: true}
}
func imm(name string, kind fieldKind) fieldDef {
	return fieldDef{Name: name, Kind: kind, Immutable: true}
}
func initial(name string, kind fieldKind) fieldDef {
	return fieldDef{Name: name, Kind: kind, CreateOnly: true}
}
func initialEnum(name string, values ...string) fieldDef {
	return fieldDef{Name: name, Kind: kString, CreateOnly: true, Allowed: values}
}
func enum(name string, required bool, values ...string) fieldDef {
	return fieldDef{Name: name, Kind: kString, Required: required, Allowed: values}
}

var resources = map[string]resourceDef{
	"departments": {
		Path: "departments", Table: "hr_departments", Envelope: "departments", Singular: "department",
		Fields: []fieldDef{req("name", kString), req("code", kString), f("description", kString), f("translations", kJSON), f("manager_employee_id", kInt), f("parent_department_id", kInt), f("is_active", kBool)},
		Search: []string{"name", "code"}, Filters: []string{"is_active", "manager_employee_id", "parent_department_id"}, NoDelete: true, OrderBy: "name",
	},
	"positions": {
		Path: "positions", Table: "hr_positions", Envelope: "positions", Singular: "position",
		Fields: []fieldDef{f("department_id", kInt), f("parent_position_id", kInt), f("hierarchy_rank", kInt), req("title", kString), req("code", kString), f("description", kString), f("translations", kJSON), f("grade", kString), f("min_salary", kDecimal), f("max_salary", kDecimal), f("currency", kString), f("is_active", kBool)},
		Search: []string{"title", "code", "grade"}, Filters: []string{"department_id", "parent_position_id", "is_active"}, NoDelete: true, OrderBy: "hierarchy_rank, title",
	},
	"contract-templates": {
		Path: "contract-templates", Table: "hr_contract_templates", Envelope: "contract_templates", Singular: "contract_template",
		Fields: []fieldDef{enum("kind", true, "employment", "memo_deal"), req("code", kString), req("name", kString), f("description", kString), req("body", kString), f("field_labels", kJSON), f("version", kInt), f("is_active", kBool)},
		Search: []string{"name", "code"}, Filters: []string{"kind", "is_active"}, OrderBy: "kind, name",
	},
	// Reads only. Creating, editing, issuing, signing and voiding a document all
	// go through the workflow endpoints so the frozen body is never hand-set.
	"contract-documents": {
		Path: "contract-documents", Table: "hr_contract_documents", Envelope: "contract_documents", Singular: "contract_document",
		Fields: []fieldDef{
			enum("kind", true, "employment", "memo_deal"), enum("status", false, "draft", "issued", "signed", "void"),
			f("reference", kString), f("template_id", kInt), f("template_code", kString), f("template_name", kString), f("template_version", kInt),
			f("employee_id", kInt), f("contract_id", kInt), f("party_name", kString), f("party_address", kString),
			f("party_id_number", kString), f("party_phone", kString), f("party_email", kString),
			f("place", kString), f("issue_date", kDate), f("token_values", kJSON), f("body_rendered", kString),
			f("issued_at", kDateTime), f("signed_at", kDateTime), f("voided_at", kDateTime), f("void_reason", kString),
		},
		Search:  []string{"reference", "party_name", "template_name"},
		Filters: []string{"kind", "status", "employee_id", "contract_id"},
		NoCreate: true, NoUpdate: true, NoDelete: true, OrderBy: "created_at DESC",
	},
	"employees": {
		Path: "employees", Table: "hr_employees", Envelope: "employees", Singular: "employee",
		// department_id/position_id/manager_id are directly correctable (an admin
		// reassignment). ValidateSafeUpdate keeps that to super-admins; everyone
		// else moves people through lifecycle events. employment_status/end_date
		// stay workflow-controlled (lifecycle only).
		Fields: []fieldDef{imm("user_id", kInt), req("employee_number", kString), req("first_name", kString), req("last_name", kString), req("work_email", kString), f("personal_email", kString), f("phone", kString), f("date_of_birth", kDate), f("gender", kString), f("nationality", kString), f("address", kString), req("hire_date", kDate), initial("end_date", kDate), f("department_id", kInt), f("position_id", kInt), f("manager_id", kInt), initialEnum("employment_status", "onboarding", "probation", "active", "leave", "suspended", "offboarded"), enum("employment_type", false, "permanent", "fixed_term", "contract", "intern", "temporary"), f("work_location", kString), f("photo_url", kString), f("notes", kString)},
		Search: []string{"employee_number", "first_name", "last_name", "work_email", "phone"}, Filters: []string{"department_id", "position_id", "manager_id", "employment_status", "employment_type"}, NoDelete: true, OrderBy: "last_name, first_name",
	},
	"emergency-contacts": {
		Path: "emergency-contacts", Table: "hr_emergency_contacts", Envelope: "emergency_contacts", Singular: "emergency_contact",
		Fields: []fieldDef{req("employee_id", kInt), req("name", kString), req("relationship", kString), req("phone", kString), f("alternate_phone", kString), f("email", kString), f("is_primary", kBool)},
		Search: []string{"name", "phone", "relationship"}, Filters: []string{"employee_id", "is_primary"}, OrderBy: "is_primary DESC, name",
	},
	"contracts": {
		Path: "contracts", Table: "hr_contracts", Envelope: "contracts", Singular: "contract",
		Fields: []fieldDef{req("employee_id", kInt), req("contract_type", kString), req("start_date", kDate), f("end_date", kDate), f("probation_end_date", kDate), req("salary", kDecimal), f("currency", kString), enum("pay_frequency", false, "weekly", "biweekly", "monthly"), enum("status", false, "draft", "active", "expired", "terminated"), f("document_url", kString), f("terms", kString)},
		Search: []string{"contract_type", "status"}, Filters: []string{"employee_id", "status", "contract_type"}, NoDelete: true, OrderBy: "start_date DESC",
	},
	"documents": {
		Path: "documents", Table: "hr_documents", Envelope: "documents", Singular: "document",
		Fields: []fieldDef{req("employee_id", kInt), req("document_type", kString), req("name", kString), imm("file_url", kString), imm("file_key", kString), imm("file_name", kString), imm("mime_type", kString), imm("size_bytes", kInt), f("expires_at", kDate), f("is_confidential", kBool), f("notes", kString), imm("uploaded_by", kInt)},
		Search: []string{"name", "document_type", "file_name"}, Filters: []string{"employee_id", "document_type", "is_confidential"}, NoCreate: true, OrderBy: "created_at DESC",
	},
	"lifecycle-events": {
		Path: "lifecycle-events", Table: "hr_lifecycle_events", Envelope: "lifecycle_events", Singular: "lifecycle_event",
		Fields: []fieldDef{req("employee_id", kInt), enum("event_type", true, "onboarding", "probation_started", "probation_completed", "transfer", "promotion", "offboarding"), req("effective_date", kDate), imm("status", kString), f("from_department_id", kInt), f("to_department_id", kInt), f("from_position_id", kInt), f("to_position_id", kInt), f("from_manager_id", kInt), f("to_manager_id", kInt), f("probation_end_date", kDate), f("title", kString), f("notes", kString), imm("created_by", kInt), imm("completed_at", kDateTime)},
		Search: []string{"event_type", "title"}, Filters: []string{"employee_id", "event_type", "status"}, NoCreate: true, NoUpdate: true, NoDelete: true, OrderBy: "effective_date DESC",
	},
	"leave-policies": {
		Path: "leave-policies", Table: "hr_leave_policies", Envelope: "leave_policies", Singular: "leave_policy",
		Fields: []fieldDef{req("name", kString), req("code", kString), req("leave_type", kString), f("description", kString), f("translations", kJSON), req("days_per_year", kDecimal), enum("accrual_mode", false, "annual", "monthly", "manual"), f("carry_over_days", kDecimal), f("minimum_notice_days", kInt), f("max_consecutive_days", kInt), f("requires_attachment", kBool), req("approval_levels", kJSON), f("is_active", kBool)},
		Search: []string{"name", "code", "leave_type"}, Filters: []string{"leave_type", "is_active"}, NoDelete: true, OrderBy: "name",
	},
	"leave-balances": {
		Path: "leave-balances", Table: "hr_leave_balances", Envelope: "leave_balances", Singular: "leave_balance",
		Fields:  []fieldDef{req("employee_id", kInt), req("policy_id", kInt), req("balance_year", kInt), req("allocated_days", kDecimal), f("carried_days", kDecimal), f("adjustment_days", kDecimal), imm("used_days", kDecimal), imm("pending_days", kDecimal)},
		Filters: []string{"employee_id", "policy_id", "balance_year"}, NoDelete: true, OrderBy: "balance_year DESC",
	},
	"leave-requests": {
		Path: "leave-requests", Table: "hr_leave_requests", Envelope: "leave_requests", Singular: "leave_request",
		Fields: []fieldDef{req("employee_id", kInt), req("policy_id", kInt), req("start_date", kDate), req("end_date", kDate), imm("requested_days", kDecimal), enum("start_portion", false, "full", "half"), enum("end_portion", false, "full", "half"), f("reason", kString), f("document_id", kInt), f("attachment_url", kString), imm("status", kString), imm("current_approval_level", kInt), imm("total_approval_levels", kInt), imm("approval_levels_snapshot", kJSON), imm("submitted_at", kDateTime), imm("decided_at", kDateTime), imm("decided_by", kInt)},
		Search: []string{"reason", "status"}, Filters: []string{"employee_id", "policy_id", "status"}, NoCreate: true, NoUpdate: true, NoDelete: true, OrderBy: "submitted_at DESC",
	},
	"shifts": {
		Path: "shifts", Table: "hr_shifts", Envelope: "shifts", Singular: "shift",
		// shift_type is the fixed category; day/night shifts carry a canonical name,
		// only `special` shifts keep a bespoke name (composed client-side).
		Fields: []fieldDef{enum("shift_type", true, "day", "night", "special"), req("name", kString), req("code", kString), req("start_time", kString), req("end_time", kString), f("break_minutes", kInt), f("grace_minutes", kInt), req("work_days", kJSON), f("is_active", kBool)},
		Search: []string{"name", "code"}, Filters: []string{"is_active", "shift_type"}, NoDelete: true, OrderBy: "shift_type, name",
	},
	"shift-assignments": {
		Path: "shift-assignments", Table: "hr_shift_assignments", Envelope: "shift_assignments", Singular: "shift_assignment",
		Fields:  []fieldDef{req("employee_id", kInt), req("shift_id", kInt), req("start_date", kDate), f("end_date", kDate), enum("status", false, "active", "ended")},
		Filters: []string{"employee_id", "shift_id"}, NoDelete: true, OrderBy: "start_date DESC",
	},
	"attendance": {
		Path: "attendance", Table: "hr_attendance", Envelope: "attendance", Singular: "attendance_record",
		Fields: []fieldDef{req("employee_id", kInt), f("shift_id", kInt), req("attendance_date", kDate), f("clock_in", kDateTime), f("clock_out", kDateTime), enum("status", false, "present", "absent", "leave", "holiday", "remote"), imm("worked_minutes", kInt), imm("late_minutes", kInt), imm("overtime_minutes", kInt), f("source", kString), f("notes", kString)},
		Search: []string{"status", "notes"}, Filters: []string{"employee_id", "shift_id", "attendance_date", "status"}, NoCreate: true, NoDelete: true, OrderBy: "attendance_date DESC",
	},
	"timesheets": {
		Path: "timesheets", Table: "hr_timesheets", Envelope: "timesheets", Singular: "timesheet",
		Fields:  []fieldDef{req("employee_id", kInt), req("week_start", kDate), req("regular_hours", kDecimal), f("overtime_hours", kDecimal), f("entries", kJSON), imm("status", kString), imm("submitted_at", kDateTime), imm("approved_at", kDateTime), imm("approved_by", kInt), f("notes", kString)},
		Filters: []string{"employee_id", "week_start", "status"}, NoDelete: true, OrderBy: "week_start DESC",
	},
	"performance-reviews": {
		Path: "performance-reviews", Table: "hr_performance_reviews", Envelope: "performance_reviews", Singular: "performance_review",
		Fields: []fieldDef{req("employee_id", kInt), f("reviewer_employee_id", kInt), req("review_period_start", kDate), req("review_period_end", kDate), f("review_type", kString), imm("status", kString), f("overall_rating", kDecimal), f("strengths", kString), f("improvements", kString), f("employee_comments", kString), f("reviewer_comments", kString), imm("completed_at", kDateTime)},
		Search: []string{"review_type", "status"}, Filters: []string{"employee_id", "reviewer_employee_id", "status"}, NoDelete: true, OrderBy: "review_period_end DESC",
	},
	"goals": {
		Path: "goals", Table: "hr_goals", Envelope: "goals", Singular: "goal",
		Fields: []fieldDef{req("employee_id", kInt), f("review_id", kInt), req("title", kString), f("description", kString), f("start_date", kDate), f("due_date", kDate), enum("status", false, "not_started", "in_progress", "completed", "cancelled"), f("progress_percent", kInt), f("weight_percent", kInt)},
		Search: []string{"title", "status"}, Filters: []string{"employee_id", "review_id", "status"}, NoDelete: true, OrderBy: "due_date",
	},
	"feedback": {
		Path: "feedback", Table: "hr_feedback", Envelope: "feedback", Singular: "feedback",
		Fields: []fieldDef{req("employee_id", kInt), f("author_employee_id", kInt), f("feedback_type", kString), f("visibility", kString), req("content", kString), f("rating", kDecimal), req("feedback_date", kDate)},
		Search: []string{"content", "feedback_type"}, Filters: []string{"employee_id", "author_employee_id", "feedback_type", "visibility"}, NoDelete: true, OrderBy: "feedback_date DESC",
	},
	"one-to-ones": {
		Path: "one-to-ones", Table: "hr_one_to_ones", Envelope: "one_to_ones", Singular: "one_to_one",
		Fields: []fieldDef{req("employee_id", kInt), f("manager_employee_id", kInt), req("scheduled_at", kDateTime), imm("completed_at", kDateTime), imm("status", kString), f("agenda", kString), f("notes", kString), f("action_items", kJSON)},
		Search: []string{"agenda", "notes"}, Filters: []string{"employee_id", "manager_employee_id", "status"}, NoDelete: true, OrderBy: "scheduled_at DESC",
	},
	"vacancies": {
		Path: "vacancies", Table: "hr_vacancies", Envelope: "vacancies", Singular: "vacancy",
		Fields: []fieldDef{f("position_id", kInt), f("department_id", kInt), req("title", kString), req("code", kString), f("description", kString), f("employment_type", kString), f("location", kString), f("openings", kInt), imm("status", kString), imm("published_at", kDateTime), f("closes_at", kDateTime), f("hiring_manager_employee_id", kInt)},
		Search: []string{"title", "code", "description"}, Filters: []string{"position_id", "department_id", "status"}, NoDelete: true, OrderBy: "created_at DESC",
	},
	"candidates": {
		Path: "candidates", Table: "hr_candidates", Envelope: "candidates", Singular: "candidate",
		Fields: []fieldDef{req("vacancy_id", kInt), req("first_name", kString), req("last_name", kString), req("email", kString), f("phone", kString), f("source", kString), f("resume_url", kString), imm("status", kString), f("rating", kDecimal), f("notes", kString), imm("applied_at", kDateTime), imm("hired_employee_id", kInt)},
		Search: []string{"first_name", "last_name", "email", "phone"}, Filters: []string{"vacancy_id", "status"}, NoDelete: true, OrderBy: "applied_at DESC",
	},
	"interviews": {
		Path: "interviews", Table: "hr_interviews", Envelope: "interviews", Singular: "interview",
		Fields: []fieldDef{req("candidate_id", kInt), req("interview_type", kString), req("scheduled_at", kDateTime), f("duration_minutes", kInt), f("location", kString), f("interviewer_employee_ids", kJSON), imm("status", kString), f("score", kDecimal), f("feedback", kString)},
		Search: []string{"interview_type", "location", "status"}, Filters: []string{"candidate_id", "status"}, NoDelete: true, OrderBy: "scheduled_at DESC",
	},
	"offers": {
		Path: "offers", Table: "hr_offers", Envelope: "offers", Singular: "offer",
		Fields:  []fieldDef{req("candidate_id", kInt), f("position_id", kInt), req("offered_salary", kDecimal), f("currency", kString), req("start_date", kDate), f("expires_at", kDate), imm("status", kString), f("document_url", kString), f("terms", kString), imm("sent_at", kDateTime), imm("responded_at", kDateTime)},
		Filters: []string{"candidate_id", "position_id", "status"}, NoDelete: true, OrderBy: "created_at DESC",
	},
	"expenses": {
		Path: "expenses", Table: "hr_expenses", Envelope: "expenses", Singular: "expense",
		Fields: []fieldDef{req("employee_id", kInt), req("expense_date", kDate), req("category", kString), req("description", kString), req("amount", kDecimal), f("currency", kString), f("receipt_url", kString), imm("status", kString), imm("submitted_at", kDateTime), imm("approved_at", kDateTime), imm("approved_by", kInt), imm("reimbursed_at", kDateTime), imm("reimbursed_by", kInt), imm("payment_reference", kString), imm("rejection_reason", kString)},
		Search: []string{"description", "category", "payment_reference"}, Filters: []string{"employee_id", "category", "status"}, NoDelete: true, OrderBy: "expense_date DESC",
	},
	"compensation": {
		Path: "compensation", Table: "hr_compensation", Envelope: "compensation", Singular: "compensation_record",
		Fields:  []fieldDef{req("employee_id", kInt), req("effective_date", kDate), req("base_salary", kDecimal), f("currency", kString), f("pay_frequency", kString), f("bonus_target", kDecimal), f("allowances", kJSON), f("reason", kString), imm("is_current", kBool)},
		Filters: []string{"employee_id", "is_current"}, NoUpdate: true, NoDelete: true, OrderBy: "effective_date DESC",
	},
	"benefits": {
		Path: "benefits", Table: "hr_benefits", Envelope: "benefits", Singular: "benefit",
		Fields: []fieldDef{req("name", kString), req("code", kString), req("benefit_type", kString), f("description", kString), f("translations", kJSON), f("employer_contribution", kDecimal), f("employee_contribution", kDecimal), f("currency", kString), f("is_active", kBool)},
		Search: []string{"name", "code", "benefit_type"}, Filters: []string{"benefit_type", "is_active"}, NoDelete: true, OrderBy: "name",
	},
	"benefit-enrollments": {
		Path: "benefit-enrollments", Table: "hr_benefit_enrollments", Envelope: "benefit_enrollments", Singular: "benefit_enrollment",
		Fields:  []fieldDef{req("benefit_id", kInt), req("employee_id", kInt), req("start_date", kDate), f("end_date", kDate), enum("status", false, "active", "suspended", "ended"), f("details", kJSON)},
		Filters: []string{"benefit_id", "employee_id", "status"}, NoDelete: true, OrderBy: "start_date DESC",
	},
}

func fieldMap(def resourceDef) map[string]fieldDef {
	out := make(map[string]fieldDef, len(def.Fields))
	for _, field := range def.Fields {
		out[field.Name] = field
	}
	return out
}
