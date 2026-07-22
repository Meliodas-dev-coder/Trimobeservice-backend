package hr

import (
	"net/http"

	"github.com/trimo/backend/internal/httpx"
)

// Contract-document endpoints. Reads go through the generic engine (registerCRUD
// list/get, which applies scoping and the sensitive-read audit); everything that
// changes a document lives here so the frozen body is never client-supplied.
//
// Every handler authorizes against the `contract-documents` resource, which maps
// to the `contracts` feature and is treated as ownerless — issuing is an HR
// administrator activity and therefore requires all-scope access.

const contractDocumentsResource = "contract-documents"

func (h *Handler) authorizeContractDocument(w http.ResponseWriter, r *http.Request, action string) (int64, bool) {
	actor, ok := actorID(w, r)
	if !ok {
		return 0, false
	}
	if _, _, err := h.svc.AuthorizeInput(r.Context(), actor, contractDocumentsResource, action, map[string]any{}); err != nil {
		writeError(w, err)
		return 0, false
	}
	return actor, true
}

func (h *Handler) previewContractDocument(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorizeContractDocument(w, r, "view"); !ok {
		return
	}
	var in ContractDocumentInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return
	}
	render, err := h.svc.PreviewContractDocument(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"preview": render})
}

func (h *Handler) createContractDocument(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorizeContractDocument(w, r, "create"); !ok {
		return
	}
	var in ContractDocumentInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return
	}
	doc, err := h.svc.CreateContractDocument(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"contract_document": doc})
}

func (h *Handler) updateContractDocument(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorizeContractDocument(w, r, "update"); !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in ContractDocumentInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return
	}
	doc, err := h.svc.UpdateContractDocument(r.Context(), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"contract_document": doc})
}

func (h *Handler) issueContractDocument(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorizeContractDocument(w, r, "update"); !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	doc, err := h.svc.IssueContractDocument(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"contract_document": doc})
}

func (h *Handler) signContractDocument(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorizeContractDocument(w, r, "update"); !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	doc, err := h.svc.SignContractDocument(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"contract_document": doc})
}

func (h *Handler) voidContractDocument(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorizeContractDocument(w, r, "update"); !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return
	}
	doc, err := h.svc.VoidContractDocument(r.Context(), id, req.Reason)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"contract_document": doc})
}

func (h *Handler) deleteContractDocument(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authorizeContractDocument(w, r, "delete"); !ok {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteContractDocument(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"deleted": true})
}
