package hr

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

// chi panics at registration time on a conflicting route, which would take the
// server down at boot rather than at request time. Registering the whole HR
// route tree here catches that without needing a database.
func TestRegisterRoutesDoesNotConflict(t *testing.T) {
	passthrough := func(next http.Handler) http.Handler { return next }
	g := Guards{}
	// Every guard field is the same passthrough middleware.
	guards := []*func(http.Handler) http.Handler{
		&g.Any, &g.Lookups, &g.Dashboard, &g.Employees, &g.Organization, &g.Lifecycle,
		&g.Leave, &g.Attendance, &g.Performance, &g.Recruitment, &g.Expenses,
		&g.Compensation, &g.Documents, &g.Audit, &g.ReportWorkforce, &g.ReportLeave,
		&g.ReportAttendance, &g.ReportPerformance, &g.ReportRecruitment,
		&g.ReportExpenses, &g.ReportCompensation,
	}
	for _, guard := range guards {
		*guard = passthrough
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("route registration panicked: %v", r)
		}
	}()

	router := chi.NewRouter()
	RegisterRoutes(router, &Handler{}, g)

	found := map[string]bool{}
	_ = chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		found[method+" "+route] = true
		return nil
	})
	for _, want := range []string{
		"GET /admin/hr/contract-documents",
		"POST /admin/hr/contract-documents/preview",
		"POST /admin/hr/contract-documents/{id}/issue",
		"POST /admin/hr/contract-documents/{id}/void",
		"GET /admin/hr/contract-templates/tokens",
	} {
		if !found[want] {
			t.Errorf("route not registered: %s", want)
		}
	}
	t.Logf("registered %d HR routes", len(found))
}
