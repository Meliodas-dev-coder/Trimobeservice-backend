package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	gcs "cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// GCS stores objects in a Google Cloud Storage / Firebase Storage bucket.
// The bucket is expected to be publicly readable (see README), so uploaded
// objects are served directly from publicBaseURL/bucket/key.
type GCS struct {
	client        *gcs.Client
	bucket        string
	publicBaseURL string
}

// NewGCS builds a GCS-backed Storage. credentialsFile is optional; when empty,
// Application Default Credentials are used (e.g. GOOGLE_APPLICATION_CREDENTIALS).
func NewGCS(ctx context.Context, bucket, credentialsFile, publicBaseURL string) (*GCS, error) {
	if bucket == "" {
		return nil, errors.New("storage: bucket is required")
	}
	var opts []option.ClientOption
	if credentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsFile))
	}
	client, err := gcs.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("storage: new client: %w", err)
	}
	if publicBaseURL == "" {
		publicBaseURL = "https://storage.googleapis.com"
	}
	return &GCS{
		client:        client,
		bucket:        bucket,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}, nil
}

func (s *GCS) Upload(ctx context.Context, key, contentType string, r io.Reader) (string, error) {
	obj := s.client.Bucket(s.bucket).Object(key)
	w := obj.NewWriter(ctx)
	w.ContentType = contentType
	w.CacheControl = "public, max-age=86400"

	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return "", fmt.Errorf("storage: write object: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("storage: finalize object: %w", err)
	}

	// Best-effort per-object public read for fine-grained buckets. On buckets
	// with uniform access this errors and is ignored — those rely on a public
	// IAM binding at the bucket level instead.
	_ = obj.ACL().Set(ctx, gcs.AllUsers, gcs.RoleReader)

	return fmt.Sprintf("%s/%s/%s", s.publicBaseURL, s.bucket, key), nil
}

func (s *GCS) DeleteByURL(ctx context.Context, url string) error {
	key := s.keyFromURL(url)
	if key == "" {
		return nil // not one of ours
	}
	err := s.client.Bucket(s.bucket).Object(key).Delete(ctx)
	if errors.Is(err, gcs.ErrObjectNotExist) {
		return nil
	}
	return err
}

func (s *GCS) Close() error {
	return s.client.Close()
}

func (s *GCS) keyFromURL(url string) string {
	prefix := fmt.Sprintf("%s/%s/", s.publicBaseURL, s.bucket)
	if !strings.HasPrefix(url, prefix) {
		return ""
	}
	return strings.TrimPrefix(url, prefix)
}
