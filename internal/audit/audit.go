// Package audit records who changed what in the admin back office and exposes a
// read-only view of that trail. A middleware (see middleware.go) that wraps the
// admin auth records every successful admin write; the handler lists them.
//
// It follows the platform's model -> repository -> service -> handler layering,
// kept in one file like the (also read-only) customers module.
package audit

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
)

// Entry is a single admin write, as the middleware records it.
type Entry struct {
	ActorUserID *int64
	Method      string
	Path        string
	TargetType  *string
	TargetID    *int64
	StatusCode  int
	Payload     []byte // validated JSON, or nil
}

// Log is the read model returned to admins: an entry joined with the actor's
// name/email (nullable if the account was later removed).
type Log struct {
	ID          int64           `db:"id" json:"id"`
	ActorUserID *int64          `db:"actor_user_id" json:"actor_user_id,omitempty"`
	ActorName   *string         `db:"actor_name" json:"actor_name,omitempty"`
	ActorEmail  *string         `db:"actor_email" json:"actor_email,omitempty"`
	Method      string          `db:"method" json:"method"`
	Path        string          `db:"path" json:"path"`
	TargetType  *string         `db:"target_type" json:"target_type,omitempty"`
	TargetID    *int64          `db:"target_id" json:"target_id,omitempty"`
	StatusCode  int            `db:"status_code" json:"status_code"`
	Payload     types.JSONText `db:"payload" json:"payload,omitempty"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
}

type Filter struct {
	ActorUserID *int64
	TargetType  string
	Method      string
	Limit       int
	Offset      int
}

// --- repository ---

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Insert(ctx context.Context, e Entry) error {
	var payload any // NULL unless we have valid JSON to store
	if len(e.Payload) > 0 {
		payload = string(e.Payload)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, method, path, target_type, target_id, status_code, payload)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ActorUserID, e.Method, e.Path, e.TargetType, e.TargetID, e.StatusCode, payload)
	return err
}

const logCols = `a.id, a.actor_user_id, u.full_name AS actor_name, u.email AS actor_email,
	a.method, a.path, a.target_type, a.target_id, a.status_code, a.payload, a.created_at`

func (r *Repository) List(ctx context.Context, f Filter) ([]Log, int, error) {
	var where []string
	var args []any
	if f.ActorUserID != nil {
		where = append(where, "a.actor_user_id = ?")
		args = append(args, *f.ActorUserID)
	}
	if f.TargetType != "" {
		where = append(where, "a.target_type = ?")
		args = append(args, f.TargetType)
	}
	if f.Method != "" {
		where = append(where, "a.method = ?")
		args = append(args, f.Method)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM audit_logs a`+clause, args...); err != nil {
		return nil, 0, err
	}

	listArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	out := []Log{}
	err := r.db.SelectContext(ctx, &out,
		`SELECT `+logCols+` FROM audit_logs a
		 LEFT JOIN users u ON u.id = a.actor_user_id`+clause+`
		 ORDER BY a.created_at DESC, a.id DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// --- service ---

type Service struct {
	repo *Repository
	log  *slog.Logger
}

func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

func (s *Service) List(ctx context.Context, f Filter) ([]Log, int, error) {
	return s.repo.List(ctx, f)
}

// Record persists an audit entry. It is best-effort: a logging failure must
// never break the admin action that was just performed, so errors are logged,
// not returned.
func (s *Service) Record(ctx context.Context, e Entry) {
	if err := s.repo.Insert(ctx, e); err != nil {
		s.log.Error("audit insert failed", "method", e.Method, "path", e.Path, "error", err)
	}
}
