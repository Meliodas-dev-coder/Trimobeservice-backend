package hr

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/auth"
	"github.com/trimo/backend/internal/httpx"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) list(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		def := resources[resource]
		limit, page := parsePage(r)
		filters := map[string]any{}
		for _, field := range def.Filters {
			if value := r.URL.Query().Get(field); value != "" {
				filters[field] = normalizedFilter(def, field, value)
			}
		}
		if resource == "attendance" {
			if r.URL.Query().Get("late") == "true" {
				filters["__late"] = true
			}
			if r.URL.Query().Get("overtime") == "true" {
				filters["__overtime"] = true
			}
		}
		opts, err := h.svc.ScopeList(r.Context(), actor, resource, ListOptions{Search: r.URL.Query().Get("q"), Filters: filters, Limit: limit, Offset: (page - 1) * limit})
		if err != nil {
			writeError(w, err)
			return
		}
		items, total, err := h.svc.List(r.Context(), resource, opts)
		if err != nil {
			writeError(w, err)
			return
		}
		// Record the sensitive read before serving it, and as a single aggregate
		// event. Recording after the response was flushed made the audit depend on
		// the write completing (a client disconnect would cancel the context and
		// silently drop it), and one row per listed record is needless write
		// amplification. The record ids seen are kept in metadata.
		if isSensitiveReadResource(resource) {
			recordIDs := make([]int64, 0, len(items))
			for _, item := range items {
				if id, ok := mapInt64(item["id"]); ok {
					recordIDs = append(recordIDs, id)
				}
			}
			_ = h.svc.access.RecordAccessEvent(r.Context(), actor, nil, resource, nil, "list", "effective_policy", map[string]any{"count": len(recordIDs), "record_ids": recordIDs})
		}
		httpx.JSON(w, http.StatusOK, httpx.Envelope{def.Envelope: items, "meta": httpx.Envelope{"total": total, "page": page, "limit": limit}})
	}
}
func (h *Handler) get(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		_, source, item, err := h.svc.AuthorizeID(r.Context(), actor, resource, "view", id)
		if err != nil {
			writeError(w, err)
			return
		}
		if isSensitiveReadResource(resource) {
			target, _ := targetEmployeeID(resource, item)
			_ = h.svc.access.RecordAccessEvent(r.Context(), actor, target, resource, &id, "view", source, nil)
		}
		httpx.JSON(w, http.StatusOK, httpx.Envelope{resources[resource].Singular: item})
	}
}
func (h *Handler) create(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		input, ok := decodeMap(w, r)
		if !ok {
			return
		}
		if _, _, err := h.svc.AuthorizeInput(r.Context(), actor, resource, "create", input); err != nil {
			writeError(w, err)
			return
		}
		item, problems, err := h.svc.Create(r.Context(), resource, input)
		if len(problems) > 0 {
			httpx.ValidationError(w, problems)
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, httpx.Envelope{resources[resource].Singular: item})
	}
}
func (h *Handler) update(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		input, ok := decodeMap(w, r)
		if !ok {
			return
		}
		access, _, current, err := h.svc.AuthorizeID(r.Context(), actor, resource, "update", id)
		if err != nil {
			writeError(w, err)
			return
		}
		if err := h.svc.ValidateSafeUpdate(access, resource, current, input); err != nil {
			writeError(w, err)
			return
		}
		if resource == "attendance" {
			if err := h.svc.PrepareAttendanceInput(access, current, "update", input); err != nil {
				writeError(w, err)
				return
			}
		}
		var item map[string]any
		var problems map[string]string
		err = nil
		if resource == "attendance" {
			item, problems, err = h.svc.RecordAttendance(r.Context(), id, input)
		} else {
			item, problems, err = h.svc.Update(r.Context(), resource, id, input)
		}
		if len(problems) > 0 {
			httpx.ValidationError(w, problems)
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, httpx.Envelope{resources[resource].Singular: item})
	}
}
func (h *Handler) delete(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		if _, _, _, err := h.svc.AuthorizeID(r.Context(), actor, resource, "delete", id); err != nil {
			writeError(w, err)
			return
		}
		if err := h.svc.Delete(r.Context(), resource, id); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) createLifecycle(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	input, ok := decodeMap(w, r)
	if !ok {
		return
	}
	if _, _, err := h.svc.AuthorizeInput(r.Context(), actor, "lifecycle-events", "create", input); err != nil {
		writeError(w, err)
		return
	}
	item, p, err := h.svc.CreateLifecycle(r.Context(), actor, input)
	if len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"lifecycle_event": item})
}
func (h *Handler) completeLifecycle(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	if _, _, _, err := h.svc.AuthorizeID(r.Context(), actor, "lifecycle-events", "complete", id); err != nil {
		writeError(w, err)
		return
	}
	item, err := h.svc.CompleteLifecycle(r.Context(), id, actor)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"lifecycle_event": item})
}
func (h *Handler) createLeave(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	input, ok := decodeMap(w, r)
	if !ok {
		return
	}
	if _, _, err := h.svc.AuthorizeInput(r.Context(), actor, "leave-requests", "create", input); err != nil {
		writeError(w, err)
		return
	}
	item, p, err := h.svc.CreateLeaveRequest(r.Context(), actor, input)
	if len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"leave_request": item})
}
func (h *Handler) leaveAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		body := decodeActionBody(w, r)
		if body == nil {
			return
		}
		required := "approve"
		if action == "cancel" {
			required = "cancel"
		}
		if _, _, _, err := h.svc.AuthorizeID(r.Context(), actor, "leave-requests", required, id); err != nil {
			writeError(w, err)
			return
		}
		item, err := h.svc.DecideLeave(r.Context(), id, actor, action, stringFrom(body, "note"))
		if err != nil {
			writeError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, httpx.Envelope{"leave_request": item})
	}
}
func (h *Handler) createAttendance(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	input, ok := decodeMap(w, r)
	if !ok {
		return
	}
	access, _, err := h.svc.AuthorizeInput(r.Context(), actor, "attendance", "create", input)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.PrepareAttendanceInput(access, input, "create", input); err != nil {
		writeError(w, err)
		return
	}
	item, p, err := h.svc.RecordAttendance(r.Context(), 0, input)
	if len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"attendance_record": item})
}

// myAttendanceToday returns the calling employee's own attendance record for
// today (or null). It backs the self-service clock widget everyone sees.
func (h *Handler) myAttendanceToday(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	item, err := h.svc.MyAttendanceToday(r.Context(), actor)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"attendance_record": item})
}

// attendanceClock stamps the caller's own clock-in ("in") or clock-out ("out")
// with the server time. Self-service: employee_id and the timestamp are set on
// the server, never supplied by the client.
func (h *Handler) attendanceClock(direction string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		var item map[string]any
		var problems map[string]string
		var err error
		if direction == "in" {
			item, problems, err = h.svc.ClockIn(r.Context(), actor)
		} else {
			item, problems, err = h.svc.ClockOut(r.Context(), actor)
		}
		if len(problems) > 0 {
			httpx.ValidationError(w, problems)
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, httpx.Envelope{"attendance_record": item})
	}
}
func (h *Handler) timesheetAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		body := decodeActionBody(w, r)
		if body == nil {
			return
		}
		required := "approve"
		if action == "submit" {
			required = "submit"
		}
		if _, _, _, err := h.svc.AuthorizeID(r.Context(), actor, "timesheets", required, id); err != nil {
			writeError(w, err)
			return
		}
		item, err := h.svc.TimesheetAction(r.Context(), id, actor, action, stringFrom(body, "note"))
		if err != nil {
			writeError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, httpx.Envelope{"timesheet": item})
	}
}
func (h *Handler) expenseAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		body := decodeActionBody(w, r)
		if body == nil {
			return
		}
		required := "approve"
		if action == "reimburse" {
			required = "reimburse"
		}
		if _, _, _, err := h.svc.AuthorizeID(r.Context(), actor, "expenses", required, id); err != nil {
			writeError(w, err)
			return
		}
		item, err := h.svc.ExpenseAction(r.Context(), id, actor, action, stringFrom(body, "note"), stringFrom(body, "payment_reference"))
		if err != nil {
			writeError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, httpx.Envelope{"expense": item})
	}
}
func (h *Handler) workflowAction(resource, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idParam(w, r)
		if !ok {
			return
		}
		body := decodeActionBody(w, r)
		if body == nil {
			return
		}
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		required := action
		if action == "reject" || action == "cancel" || action == "pause" || action == "submit" {
			required = "update"
		}
		if action == "complete" && (resource == "vacancies" || resource == "interviews") {
			required = "update"
		}
		if _, _, _, err := h.svc.AuthorizeID(r.Context(), actor, resource, required, id); err != nil {
			writeError(w, err)
			return
		}
		item, err := h.svc.WorkflowAction(r.Context(), resource, id, actor, action, stringFrom(body, "note"))
		if err != nil {
			writeError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, httpx.Envelope{resources[resource].Singular: item})
	}
}
func (h *Handler) hireCandidate(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	if _, _, _, err := h.svc.AuthorizeID(r.Context(), actor, "candidates", "hire", id); err != nil {
		writeError(w, err)
		return
	}
	input, ok := decodeMap(w, r)
	if !ok {
		return
	}
	candidate, employee, p, err := h.svc.HireCandidate(r.Context(), id, actor, input)
	if len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"candidate": candidate, "employee": employee})
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	value, err := h.svc.Dashboard(r.Context(), actor)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"dashboard": value})
}
func (h *Handler) employeeLookup(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.EmployeeLookup(r.Context(), actor, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"employees": items})
}
func (h *Handler) organizationLookup(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		items, err := h.svc.OrganizationLookup(r.Context(), actor, kind, r.URL.Query().Get("q"))
		if err != nil {
			writeError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, httpx.Envelope{kind: items})
	}
}
func (h *Handler) adminUserLookup(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.AdminUserLookup(r.Context(), actor, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"admin_users": items})
}
func (h *Handler) reportNamed(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := actorID(w, r)
		if !ok {
			return
		}
		if err := h.svc.AuthorizeReport(r.Context(), actor, name, "view"); err != nil {
			writeError(w, err)
			return
		}
		items, err := h.svc.Report(r.Context(), name, r.URL.Query().Get("start"), r.URL.Query().Get("end"))
		if err != nil {
			writeError(w, err)
			return
		}
		httpx.JSON(w, http.StatusOK, httpx.Envelope{"report": name, "rows": items, "meta": httpx.Envelope{"total": len(items)}})
	}
}
func (h *Handler) exportNamed(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { h.exportReport(name, w, r) }
}
func (h *Handler) exportReport(name string, w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	if err := h.svc.AuthorizeReport(r.Context(), actor, name, "export"); err != nil {
		writeError(w, err)
		return
	}
	items, err := h.svc.Report(r.Context(), name, r.URL.Query().Get("start"), r.URL.Query().Get("end"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="hr-%s.csv"`, safeFilename(name)))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	if len(items) == 0 {
		writer.Flush()
		return
	}
	headers := reportColumns[name]
	if len(headers) == 0 {
		headers = make([]string, 0, len(items[0]))
		for key := range items[0] {
			headers = append(headers, key)
		}
		sort.Strings(headers)
	}
	_ = writer.Write(headers)
	for _, row := range items {
		record := make([]string, len(headers))
		for i, key := range headers {
			record[i] = sanitizeCSVCell(row[key])
		}
		_ = writer.Write(record)
	}
	writer.Flush()
}
func (h *Handler) auditHistory(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	limit, page := parsePage(r)
	items, total, err := h.svc.AuditHistory(r.Context(), actor, limit, (page-1)*limit)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"audit_history": items, "meta": httpx.Envelope{"total": total, "page": page, "limit": limit}})
}
func (h *Handler) leaveCalendar(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	opts, err := h.svc.ScopeList(r.Context(), actor, "leave-requests", ListOptions{Filters: map[string]any{}, Limit: 500, Offset: 0})
	if err != nil {
		writeError(w, err)
		return
	}
	items, _, err := h.svc.List(r.Context(), "leave-requests", opts)
	if err != nil {
		writeError(w, err)
		return
	}
	start, end := r.URL.Query().Get("start"), r.URL.Query().Get("end")
	if start != "" || end != "" {
		filtered := make([]map[string]any, 0, len(items))
		for _, item := range items {
			itemStart, itemEnd := accessStringValue(item["start_date"]), accessStringValue(item["end_date"])
			if start != "" && itemEnd < start {
				continue
			}
			if end != "" && itemStart > end {
				continue
			}
			filtered = append(filtered, item)
		}
		items = filtered
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"leave_calendar": items})
}

func (h *Handler) notifications(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	limit, page := parsePage(r)
	items, total, err := h.svc.ListNotifications(r.Context(), actor, r.URL.Query().Get("unread") == "true", limit, (page-1)*limit)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"notifications": items, "meta": httpx.Envelope{"total": total, "page": page, "limit": limit}})
}
func (h *Handler) readNotification(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	item, err := h.svc.MarkNotificationRead(r.Context(), id, actor)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"notification": item})
}

func (h *Handler) accessCatalog(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.requireSuper(r.Context(), actor); err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"access_catalog": h.svc.AccessCatalog()})
}

func (h *Handler) positionHierarchy(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if _, _, err := h.svc.AuthorizeInput(r.Context(), actor, "positions", "view", map[string]any{}); err != nil {
		writeError(w, err)
		return
	}
	items, err := h.svc.PositionHierarchy(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"positions": items})
}

func (h *Handler) applyPositionTemplate(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if _, _, err := h.svc.AuthorizeInput(r.Context(), actor, "positions", "create", map[string]any{}); err != nil {
		writeError(w, err)
		return
	}
	items, err := h.svc.ApplyBaseTemplate(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"positions": items})
}

func (h *Handler) departmentAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	value, err := h.svc.GetDepartmentAccess(r.Context(), actor, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"access": value})
}

func (h *Handler) updateDepartmentAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Modules []string `json:"modules"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	value, problems, err := h.svc.SetDepartmentAccess(r.Context(), actor, id, req.Modules)
	if len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"access": value})
}

func (h *Handler) positionAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	value, err := h.svc.GetPositionAccess(r.Context(), actor, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"access": value})
}

func (h *Handler) updatePositionAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req PositionAccessRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	value, problems, err := h.svc.SetPositionAccess(r.Context(), actor, id, req)
	if len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"access": value})
}

func (h *Handler) employeeAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	value, err := h.svc.EmployeeAccess(r.Context(), actor, id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"access": value})
}

func (h *Handler) provisionEmployeeAccount(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req AccountRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	value, problems, err := h.svc.ProvisionEmployeeAccount(r.Context(), actor, id, req)
	if len(problems) > 0 {
		httpx.ValidationError(w, problems)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"account": value})
}

func (h *Handler) uploadDocument(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, (5<<20)+(512<<10))
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid multipart form or file too large")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.ValidationError(w, map[string]string{"file": "is required"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (5<<20)+1))
	if err != nil || len(data) > 5<<20 {
		httpx.ValidationError(w, map[string]string{"file": "must not exceed 5 MiB"})
		return
	}
	name, mime, ok := validatedDocument(header, data)
	if !ok {
		httpx.ValidationError(w, map[string]string{"file": "must be PDF, DOCX, JPEG, PNG, or plain text"})
		return
	}
	employeeID, err := strconv.ParseInt(r.FormValue("employee_id"), 10, 64)
	if err != nil || employeeID <= 0 {
		httpx.ValidationError(w, map[string]string{"employee_id": "must be a positive id"})
		return
	}
	confidential := r.FormValue("is_confidential") != "false"
	meta := map[string]any{"employee_id": float64(employeeID), "document_type": r.FormValue("document_type"), "name": r.FormValue("name"), "is_confidential": confidential}
	if meta["name"] == "" {
		meta["name"] = name
	}
	if v := r.FormValue("expires_at"); v != "" {
		meta["expires_at"] = v
	}
	if v := r.FormValue("notes"); v != "" {
		meta["notes"] = v
	}
	if _, _, err := h.svc.AuthorizeInput(r.Context(), actor, "documents", "upload", meta); err != nil {
		writeError(w, err)
		return
	}
	item, p, err := h.svc.UploadDocument(r.Context(), actor, meta, name, mime, data)
	if len(p) > 0 {
		httpx.ValidationError(w, p)
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"document": item})
}
func (h *Handler) downloadDocument(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	_, source, item, err := h.svc.AuthorizeID(r.Context(), actor, "documents", "download", id)
	if err != nil {
		writeError(w, err)
		return
	}
	file, err := h.svc.DownloadDocument(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	// Record the confidential-document access before streaming it, so a
	// mid-transfer client disconnect cannot cancel the context and drop the audit.
	target, _ := targetEmployeeID("documents", item)
	_ = h.svc.access.RecordAccessEvent(r.Context(), actor, target, "documents", &id, "download", source, map[string]any{"file_name": file.Name})
	w.Header().Set("Content-Type", file.MIME)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeFilename(file.Name)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Data)
}

func validatedDocument(header *multipart.FileHeader, data []byte) (string, string, bool) {
	name := safeFilename(header.Filename)
	detected := http.DetectContentType(data)
	ext := strings.ToLower(filepath.Ext(name))
	switch detected {
	case "application/pdf", "image/jpeg", "image/png":
		return name, detected, true
	case "text/plain; charset=utf-8":
		if ext == ".txt" || ext == ".csv" {
			return name, detected, true
		}
	case "application/zip":
		if ext == ".docx" {
			return name, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true
		}
	}
	return "", "", false
}
func safeFilename(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if r == '\r' || r == '\n' || r == '"' || r == '/' || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return "document"
	}
	if len(out) > 180 {
		return out[:180]
	}
	return out
}

func decodeMap(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	var input map[string]any
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return input, true
}
func decodeActionBody(w http.ResponseWriter, r *http.Request) map[string]any {
	if r.ContentLength == 0 {
		return map[string]any{}
	}
	input, ok := decodeMap(w, r)
	if !ok {
		return nil
	}
	return input
}
func stringFrom(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}
func actorID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "authentication required")
		return 0, false
	}
	return id, true
}
func parsePage(r *http.Request) (int, int) {
	limit, page := 20, 1
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		if n > 500 {
			n = 500
		}
		limit = n
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 0 {
		page = n
	}
	return limit, page
}

func normalizedFilter(def resourceDef, name, value string) any {
	for _, field := range def.Fields {
		if field.Name != name {
			continue
		}
		switch field.Kind {
		case kBool:
			return value == "true" || value == "1"
		case kInt:
			if n, err := strconv.ParseInt(value, 10, 64); err == nil {
				return n
			}
		}
	}
	return value
}
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrForbidden):
		httpx.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrValidation):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrConflict), errors.Is(err, ErrReferenced), errors.Is(err, ErrInvalidTransition), errors.Is(err, ErrInsufficientLeave), errors.Is(err, ErrLeaveOverlap), errors.Is(err, ErrApprovalOrder):
		httpx.Error(w, http.StatusConflict, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
