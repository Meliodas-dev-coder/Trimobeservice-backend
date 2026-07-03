// Package uploads exposes an admin-only image upload endpoint. The file is
// validated (type + size) and pushed to object storage; the response carries
// the public URL that image records then reference.
package uploads

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/httpx"
	"github.com/trimo/backend/internal/storage"
)

// allowed maps a sniffed content type to a file extension.
var allowed = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

type Handler struct {
	store    storage.Storage
	maxBytes int64
}

func NewHandler(store storage.Storage, maxBytes int64) *Handler {
	if maxBytes <= 0 {
		maxBytes = 5 * 1024 * 1024
	}
	return &Handler{store: store, maxBytes: maxBytes}
}

// RegisterRoutes mounts the upload endpoint under /admin (admin only).
func RegisterRoutes(r chi.Router, h *Handler, adminOnly func(http.Handler) http.Handler) {
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)
		r.Post("/admin/uploads", h.Upload)
	})
}

// Upload accepts a multipart form with a single "file" field.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes+1024) // small headroom for multipart overhead

	file, _, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "a 'file' field is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "file is too large")
		return
	}
	if int64(len(data)) > h.maxBytes {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "file exceeds the size limit")
		return
	}

	contentType := http.DetectContentType(data)
	ext, ok := allowed[contentType]
	if !ok {
		httpx.Error(w, http.StatusUnsupportedMediaType, "only JPEG, PNG, WebP, or GIF images are allowed")
		return
	}

	key := "uploads/" + randomID() + ext
	url, err := h.store.Upload(r.Context(), key, contentType, bytes.NewReader(data))
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "could not store the image")
		return
	}

	httpx.JSON(w, http.StatusCreated, httpx.Envelope{"url": url, "key": key})
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
