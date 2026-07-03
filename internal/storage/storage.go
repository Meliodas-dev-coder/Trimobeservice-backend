// Package storage abstracts object storage for uploaded media so the provider
// is swappable. The current implementation targets a Firebase Storage bucket
// (which is a Google Cloud Storage bucket) via the GCS Go client.
package storage

import (
	"context"
	"io"
)

// Storage uploads and deletes public objects.
type Storage interface {
	// Upload stores the object at key and returns its public URL.
	Upload(ctx context.Context, key, contentType string, r io.Reader) (url string, err error)
	// DeleteByURL removes the object identified by a URL previously returned by
	// Upload. Unknown URLs are a no-op.
	DeleteByURL(ctx context.Context, url string) error
}
