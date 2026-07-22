package audit

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCapturePayloadSkipsHRDataWithoutConsumingBody(t *testing.T) {
	body := `{"employee_id":7,"base_salary":"2500000.00","notes":"private"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/hr/compensation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if payload := capturePayload(req); payload != nil {
		t.Fatalf("expected HR payload to be excluded, got %q", payload)
	}
	remaining, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(remaining) != body {
		t.Fatalf("handler body changed: got %q", remaining)
	}
}

func TestCapturePayloadStillRecordsAndRedactsNonHRData(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/admin/users", strings.NewReader(`{"name":"A","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")

	payload := string(capturePayload(req))
	if !strings.Contains(payload, `"password":"[redacted]"`) {
		t.Fatalf("expected password redaction, got %q", payload)
	}
}

func TestParseTargetUsesConcreteHRResource(t *testing.T) {
	target, id := parseTarget("/api/v1/admin/hr/leave-requests/42/approve")
	if target != "hr_leave_requests" || id != 42 {
		t.Fatalf("got target=%q id=%d", target, id)
	}
}

func TestParseTargetKeepsExistingAdminResources(t *testing.T) {
	target, id := parseTarget("/api/v1/admin/orders/9/status")
	if target != "orders" || id != 9 {
		t.Fatalf("got target=%q id=%d", target, id)
	}
}
