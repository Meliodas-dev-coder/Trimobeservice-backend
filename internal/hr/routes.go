package hr

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/hraccess"
)

// Guards are composed in cmd/api from RequireAdmin + the indicated HR domain
// capability + audit recording. Existing `hr` permission is included in every
// guard there and therefore remains the full HR administrator umbrella.
type Guards struct {
	Any, Lookups, Dashboard, Employees, Organization, Lifecycle, Leave, Attendance,
	Performance, Recruitment, Expenses, Compensation, Documents, Audit,
	ReportWorkforce, ReportLeave, ReportAttendance, ReportPerformance, ReportRecruitment, ReportExpenses, ReportCompensation func(http.Handler) http.Handler
}

func RegisterRoutes(r chi.Router, h *Handler, g Guards) {
	r.Route("/admin/hr", func(r chi.Router) {
		// Memoize the actor's resolved access for the life of each HR request, so
		// the several Resolve calls one request makes (guard, handler, workflow)
		// run their identity/capability/policy/tree queries only once.
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(hraccess.WithRequestCache(req.Context())))
			})
		})
		guarded(r, g.Lookups, func(r chi.Router) {
			r.Get("/lookups/employees", h.employeeLookup)
			r.Get("/lookups/departments", h.organizationLookup("departments"))
			r.Get("/lookups/positions", h.organizationLookup("positions"))
		})
		guarded(r, g.Any, func(r chi.Router) {
			r.Get("/notifications", h.notifications)
			r.Patch("/notifications/{id}/read", h.readNotification)
		})
		guarded(r, g.Dashboard, func(r chi.Router) { r.Get("/dashboard", h.dashboard) })
		guarded(r, g.Organization, func(r chi.Router) {
			r.Get("/access/catalog", h.accessCatalog)
			registerCRUD(r, h, "departments")
			registerCRUD(r, h, "positions")
			registerCRUD(r, h, "contract-templates")
			r.Get("/contract-templates/tokens", h.contractTemplateTokens)
			r.Post("/contract-templates/preview", h.previewContractTemplate)
			r.Get("/departments/{id}/position-hierarchy", h.positionHierarchy)
			r.Post("/departments/{id}/position-hierarchy/apply", h.applyPositionTemplate)
			r.Get("/departments/{id}/access", h.departmentAccess)
			r.Put("/departments/{id}/access", h.updateDepartmentAccess)
			r.Get("/positions/{id}/access", h.positionAccess)
			r.Put("/positions/{id}/access", h.updatePositionAccess)
		})
		guarded(r, g.Employees, func(r chi.Router) {
			r.Get("/lookups/admin-users", h.adminUserLookup)
			registerCRUD(r, h, "employees")
			r.Get("/employees/{id}/access", h.employeeAccess)
			r.Post("/employees/{id}/account", h.provisionEmployeeAccount)
			r.Patch("/employees/{id}/account", h.provisionEmployeeAccount)
			registerCRUD(r, h, "emergency-contacts")
		})
		guarded(r, g.Compensation, func(r chi.Router) {
			// Reads via the engine (scoping + sensitive-read audit); every state
			// change goes through the workflow endpoints below.
			registerCRUD(r, h, "contract-documents")
			r.Post("/contract-documents/preview", h.previewContractDocument)
			r.Post("/contract-documents", h.createContractDocument)
			r.Put("/contract-documents/{id}", h.updateContractDocument)
			r.Post("/contract-documents/{id}/issue", h.issueContractDocument)
			r.Post("/contract-documents/{id}/sign", h.signContractDocument)
			r.Post("/contract-documents/{id}/void", h.voidContractDocument)
			r.Delete("/contract-documents/{id}", h.deleteContractDocument)
			registerCRUD(r, h, "contracts")
			registerCRUD(r, h, "compensation")
			registerCRUD(r, h, "benefits")
			registerCRUD(r, h, "benefit-enrollments")
		})
		guarded(r, g.Documents, func(r chi.Router) {
			r.Get("/documents", h.list("documents"))
			r.Post("/documents/upload", h.uploadDocument)
			r.Get("/documents/{id}", h.get("documents"))
			r.Put("/documents/{id}", h.update("documents"))
			r.Patch("/documents/{id}", h.update("documents"))
			r.Delete("/documents/{id}", h.delete("documents"))
			r.Get("/documents/{id}/download", h.downloadDocument)
		})
		guarded(r, g.Lifecycle, func(r chi.Router) {
			r.Get("/lifecycle-events", h.list("lifecycle-events"))
			r.Post("/lifecycle-events", h.createLifecycle)
			r.Get("/lifecycle-events/{id}", h.get("lifecycle-events"))
			r.Post("/lifecycle-events/{id}/complete", h.completeLifecycle)
		})
		guarded(r, g.Leave, func(r chi.Router) {
			registerCRUD(r, h, "leave-policies")
			registerCRUD(r, h, "leave-balances")
			r.Get("/leave-requests", h.list("leave-requests"))
			r.Post("/leave-requests", h.createLeave)
			r.Get("/leave-requests/{id}", h.get("leave-requests"))
			r.Post("/leave-requests/{id}/approve", h.leaveAction("approve"))
			r.Post("/leave-requests/{id}/reject", h.leaveAction("reject"))
			r.Post("/leave-requests/{id}/cancel", h.leaveAction("cancel"))
			r.Get("/leave-calendar", h.leaveCalendar)
		})
		guarded(r, g.Attendance, func(r chi.Router) {
			registerCRUD(r, h, "shifts")
			registerCRUD(r, h, "shift-assignments")
			r.Get("/attendance", h.list("attendance"))
			r.Post("/attendance", h.createAttendance)
			// Self-service clock (everyone with an employee record): server-stamped
			// times, so worked/late/overtime cannot be faked. Static paths take
			// precedence over /attendance/{id}.
			r.Get("/attendance/me/today", h.myAttendanceToday)
			// Employee × day grid backing the attendance calendar screen.
			r.Get("/attendance/calendar", h.attendanceCalendar)
			r.Post("/attendance/clock-in", h.attendanceClock("in"))
			r.Post("/attendance/clock-out", h.attendanceClock("out"))
			r.Get("/attendance/{id}", h.get("attendance"))
			r.Put("/attendance/{id}", h.update("attendance"))
			r.Patch("/attendance/{id}", h.update("attendance"))
			registerCRUD(r, h, "timesheets")
			r.Post("/timesheets/{id}/submit", h.timesheetAction("submit"))
			r.Post("/timesheets/{id}/approve", h.timesheetAction("approve"))
			r.Post("/timesheets/{id}/reject", h.timesheetAction("reject"))
		})
		guarded(r, g.Performance, func(r chi.Router) {
			registerCRUD(r, h, "performance-reviews")
			registerCRUD(r, h, "goals")
			registerCRUD(r, h, "feedback")
			registerCRUD(r, h, "one-to-ones")
			r.Post("/performance-reviews/{id}/complete", h.workflowAction("performance-reviews", "complete"))
			r.Post("/one-to-ones/{id}/complete", h.workflowAction("one-to-ones", "complete"))
			r.Post("/one-to-ones/{id}/cancel", h.workflowAction("one-to-ones", "cancel"))
		})
		guarded(r, g.Recruitment, func(r chi.Router) {
			registerCRUD(r, h, "vacancies")
			registerCRUD(r, h, "candidates")
			registerCRUD(r, h, "interviews")
			registerCRUD(r, h, "offers")
			for _, a := range []string{"submit", "pause", "complete", "cancel"} {
				r.Post("/vacancies/{id}/"+a, h.workflowAction("vacancies", a))
			}
			for _, a := range []string{"approve", "reject", "cancel"} {
				r.Post("/candidates/{id}/"+a, h.workflowAction("candidates", a))
			}
			r.Post("/candidates/{id}/hire", h.hireCandidate)
			for _, a := range []string{"complete", "cancel"} {
				r.Post("/interviews/{id}/"+a, h.workflowAction("interviews", a))
			}
			for _, a := range []string{"submit", "approve", "reject", "cancel"} {
				r.Post("/offers/{id}/"+a, h.workflowAction("offers", a))
			}
		})
		guarded(r, g.Expenses, func(r chi.Router) {
			registerCRUD(r, h, "expenses")
			r.Post("/expenses/{id}/approve", h.expenseAction("approve"))
			r.Post("/expenses/{id}/reject", h.expenseAction("reject"))
			r.Post("/expenses/{id}/reimburse", h.expenseAction("reimburse"))
		})
		guarded(r, g.Audit, func(r chi.Router) { r.Get("/audit-history", h.auditHistory) })
		guarded(r, g.ReportWorkforce, func(r chi.Router) {
			r.Get("/reports/workforce", h.reportNamed("workforce"))
			r.Get("/exports/workforce", h.exportNamed("workforce"))
		})
		guarded(r, g.ReportLeave, func(r chi.Router) {
			r.Get("/reports/leave", h.reportNamed("leave"))
			r.Get("/exports/leave", h.exportNamed("leave"))
		})
		guarded(r, g.ReportAttendance, func(r chi.Router) {
			r.Get("/reports/attendance", h.reportNamed("attendance"))
			r.Get("/exports/attendance", h.exportNamed("attendance"))
		})
		guarded(r, g.ReportPerformance, func(r chi.Router) {
			r.Get("/reports/performance", h.reportNamed("performance"))
			r.Get("/exports/performance", h.exportNamed("performance"))
		})
		guarded(r, g.ReportRecruitment, func(r chi.Router) {
			r.Get("/reports/recruitment", h.reportNamed("recruitment"))
			r.Get("/exports/recruitment", h.exportNamed("recruitment"))
		})
		guarded(r, g.ReportExpenses, func(r chi.Router) {
			r.Get("/reports/expenses", h.reportNamed("expenses"))
			r.Get("/exports/expenses", h.exportNamed("expenses"))
		})
		guarded(r, g.ReportCompensation, func(r chi.Router) {
			r.Get("/reports/compensation", h.reportNamed("compensation"))
			r.Get("/exports/compensation", h.exportNamed("compensation"))
		})
	})
}

func guarded(r chi.Router, middleware func(http.Handler) http.Handler, routes func(chi.Router)) {
	r.Group(func(r chi.Router) { r.Use(middleware); routes(r) })
}

func registerCRUD(r chi.Router, h *Handler, resource string) {
	def := resources[resource]
	base := "/" + def.Path
	r.Get(base, h.list(resource))
	if !def.NoCreate {
		r.Post(base, h.create(resource))
	}
	r.Get(base+"/{id}", h.get(resource))
	if !def.NoUpdate {
		r.Put(base+"/{id}", h.update(resource))
		r.Patch(base+"/{id}", h.update(resource))
	}
	if !def.NoDelete {
		r.Delete(base+"/{id}", h.delete(resource))
	}
}
