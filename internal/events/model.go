// Package events is Trimobe's event-planning module: an admin-managed catalog of
// event services (sound, lighting, catering, artists, decoration…) grouped into
// categories, plus client event requests that the team quotes manually. It
// follows the platform's Category -> Item -> Transaction shape:
//   ServiceCategory -> Service -> EventRequest (+ RequestService snapshots).
// Pricing is quote-based: a request carries no price until an admin sets one;
// payment then flows through the shared manual-payments ledger (payable 'event').
package events

import (
	"time"

	"github.com/jmoiron/sqlx/types"
)

// Planning lifecycle (payment is a separate concern; see PaymentStatus).
const (
	StatusRequested  = "requested"
	StatusReviewing  = "reviewing"
	StatusQuoted     = "quoted"
	StatusConfirmed  = "confirmed"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusCancelled  = "cancelled"
)

const (
	PaymentUnpaid   = "unpaid"
	PaymentPaid     = "paid"
	PaymentRefunded = "refunded"
)

// --- catalog: what we offer ---

type ServiceCategory struct {
	ID          int64     `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Slug        string    `db:"slug" json:"slug"`
	Description *string   `db:"description" json:"description,omitempty"`
	Icon        *string   `db:"icon" json:"icon,omitempty"`
	ImageURL    *string   `db:"image_url" json:"image_url,omitempty"`
	SortOrder   int       `db:"sort_order" json:"sort_order"`
	IsActive    bool      `db:"is_active" json:"is_active"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`

	// List-only aggregate.
	ServiceCount int `db:"service_count" json:"service_count"`
}

type EventService struct {
	ID          int64          `db:"id" json:"id"`
	CategoryID  int64          `db:"category_id" json:"category_id"`
	Name        string         `db:"name" json:"name"`
	Slug        string         `db:"slug" json:"slug"`
	Description *string        `db:"description" json:"description,omitempty"`
	FromPrice   *string        `db:"from_price" json:"from_price,omitempty"` // DECIMAL(12,2) as string; indicative only
	PriceUnit   *string        `db:"price_unit" json:"price_unit,omitempty"`
	ImageURL    *string        `db:"image_url" json:"image_url,omitempty"`
	Attributes  types.JSONText `db:"attributes" json:"attributes,omitempty"`
	SortOrder   int            `db:"sort_order" json:"sort_order"`
	IsActive    bool           `db:"is_active" json:"is_active"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at" json:"updated_at"`

	// List-only join (category name for display).
	CategoryName *string `db:"category_name" json:"category_name,omitempty"`
}

// --- transaction: an event request ---

type EventRequest struct {
	ID            int64      `db:"id" json:"id"`
	UserID        *int64     `db:"user_id" json:"user_id,omitempty"`
	CustomerName  *string    `db:"customer_name" json:"customer_name,omitempty"`
	RequestNumber string     `db:"request_number" json:"request_number"`
	EventType     string     `db:"event_type" json:"event_type"`
	Status        string     `db:"status" json:"status"`
	PaymentStatus string     `db:"payment_status" json:"payment_status"`
	PaidAt        *time.Time `db:"paid_at" json:"paid_at,omitempty"`
	EventStart    time.Time  `db:"event_start" json:"event_start"`
	EventEnd      *time.Time `db:"event_end" json:"event_end,omitempty"`
	Location      string     `db:"location" json:"location"`
	GuestCount    *int       `db:"guest_count" json:"guest_count,omitempty"`
	Budget        *string    `db:"budget" json:"budget,omitempty"`
	QuotedPrice   *string    `db:"quoted_price" json:"quoted_price,omitempty"`
	ContactPhone  string     `db:"contact_phone" json:"contact_phone"`
	ContactEmail  *string    `db:"contact_email" json:"contact_email,omitempty"`
	Note          *string    `db:"note" json:"note,omitempty"`
	AdminNote     *string    `db:"admin_note" json:"admin_note,omitempty"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at" json:"updated_at"`

	// List-only aggregate.
	ServiceCount int `db:"service_count" json:"service_count"`
}

// RequestService is a service line snapshotted onto a request.
type RequestService struct {
	ID                int64     `db:"id" json:"id"`
	RequestID         int64     `db:"request_id" json:"request_id"`
	ServiceID         *int64    `db:"service_id" json:"service_id,omitempty"`
	ServiceName       string    `db:"service_name" json:"service_name"`
	CategoryName      *string   `db:"category_name" json:"category_name,omitempty"`
	FromPriceSnapshot *string   `db:"from_price_snapshot" json:"from_price_snapshot,omitempty"`
	Quantity          int       `db:"quantity" json:"quantity"`
	Note              *string   `db:"note" json:"note,omitempty"`
	CreatedAt         time.Time `db:"created_at" json:"created_at"`
}

// EventRequestDetail is a request with its selected services and artists.
type EventRequestDetail struct {
	EventRequest
	Services []RequestService `json:"services"`
	Artists  []RequestArtist  `json:"artists"`
}

// --- catalog: gospel artists ---

// Artist is a public, browsable performer profile. Multi-value fields (genres,
// formats, languages, occasions, links) are comma/newline-separated strings the
// client splits into tags; availability is coordinated off-system.
type Artist struct {
	ID          int64     `db:"id" json:"id"`
	StageName   string    `db:"stage_name" json:"stage_name"`
	Slug        string    `db:"slug" json:"slug"`
	Tagline     *string   `db:"tagline" json:"tagline,omitempty"`
	Bio         *string   `db:"bio" json:"bio,omitempty"`
	HomeBase    *string   `db:"home_base" json:"home_base,omitempty"`
	PhotoURL    *string   `db:"photo_url" json:"photo_url,omitempty"`
	GroupSize   *string   `db:"group_size" json:"group_size,omitempty"`
	Genres      *string   `db:"genres" json:"genres,omitempty"`
	Formats     *string   `db:"formats" json:"formats,omitempty"`
	Languages   *string   `db:"languages" json:"languages,omitempty"`
	Occasions   *string   `db:"occasions" json:"occasions,omitempty"`
	SampleLinks *string   `db:"sample_links" json:"sample_links,omitempty"`
	SocialLinks *string   `db:"social_links" json:"social_links,omitempty"`
	FromFee     *string   `db:"from_fee" json:"from_fee,omitempty"` // DECIMAL(12,2) as string; indicative
	IsFeatured  bool      `db:"is_featured" json:"is_featured"`
	SortOrder   int       `db:"sort_order" json:"sort_order"`
	IsActive    bool      `db:"is_active" json:"is_active"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// RequestArtist is an artist named on a request, snapshotted.
type RequestArtist struct {
	ID          int64     `db:"id" json:"id"`
	RequestID   int64     `db:"request_id" json:"request_id"`
	ArtistID    *int64    `db:"artist_id" json:"artist_id,omitempty"`
	ArtistName  string    `db:"artist_name" json:"artist_name"`
	FeeSnapshot *string   `db:"fee_snapshot" json:"fee_snapshot,omitempty"`
	Note        *string   `db:"note" json:"note,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// artistRow is the read subset of an artist used to snapshot it onto a request.
type artistRow struct {
	ID        int64   `db:"id"`
	StageName string  `db:"stage_name"`
	FromFee   *string `db:"from_fee"`
	IsActive  bool    `db:"is_active"`
}

// ArtistFilter drives the artist list query.
type ArtistFilter struct {
	Search       string
	ActiveOnly   bool
	FeaturedOnly bool
	Limit        int // 0 = no limit (public grouped roster)
	Offset       int
}

// serviceRow is the read subset of a service used to snapshot it onto a request.
type serviceRow struct {
	ID           int64   `db:"id"`
	Name         string  `db:"name"`
	FromPrice    *string `db:"from_price"`
	IsActive     bool    `db:"is_active"`
	CategoryName string  `db:"category_name"`
}

// ServiceFilter drives the service list query.
type ServiceFilter struct {
	CategoryID *int64
	Search     string
	ActiveOnly bool
	Limit      int // 0 = no limit (public grouped catalog)
	Offset     int
}

// EventRequestFilter drives list queries (admin and per-user).
type EventRequestFilter struct {
	UserID        *int64
	Status        string
	PaymentStatus string
	EventType     string
	Limit         int
	Offset        int
}

// --- request DTOs ---

type ServiceCategoryRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	ImageURL    *string `json:"image_url"`
	SortOrder   int     `json:"sort_order"`
	IsActive    *bool   `json:"is_active"`
}

type ServiceRequest struct {
	CategoryID  int64          `json:"category_id"`
	Name        string         `json:"name"`
	Description *string        `json:"description"`
	FromPrice   *string        `json:"from_price"`
	PriceUnit   *string        `json:"price_unit"`
	ImageURL    *string        `json:"image_url"`
	Attributes  types.JSONText `json:"attributes"`
	SortOrder   int            `json:"sort_order"`
	IsActive    *bool          `json:"is_active"`
}

// SelectedService is one service the client picks on a request.
type SelectedService struct {
	ServiceID int64   `json:"service_id"`
	Quantity  int     `json:"quantity"`
	Note      *string `json:"note"`
}

// SelectedArtist is one artist the client names on a request.
type SelectedArtist struct {
	ArtistID int64   `json:"artist_id"`
	Note     *string `json:"note"`
}

type CreateEventRequest struct {
	EventType    string            `json:"event_type"`
	EventStart   time.Time         `json:"event_start"` // RFC3339
	EventEnd     *time.Time        `json:"event_end"`   // RFC3339, optional
	Location     string            `json:"location"`
	GuestCount   *int              `json:"guest_count"`
	Budget       *string           `json:"budget"`
	ContactPhone string            `json:"contact_phone"`
	ContactEmail *string           `json:"contact_email"`
	Note         *string           `json:"note"`
	Services     []SelectedService `json:"services"`
	Artists      []SelectedArtist  `json:"artists"`
}

// ArtistRequest is the admin create/update payload for an artist.
type ArtistRequest struct {
	StageName   string  `json:"stage_name"`
	Tagline     *string `json:"tagline"`
	Bio         *string `json:"bio"`
	HomeBase    *string `json:"home_base"`
	PhotoURL    *string `json:"photo_url"`
	GroupSize   *string `json:"group_size"`
	Genres      *string `json:"genres"`
	Formats     *string `json:"formats"`
	Languages   *string `json:"languages"`
	Occasions   *string `json:"occasions"`
	SampleLinks *string `json:"sample_links"`
	SocialLinks *string `json:"social_links"`
	FromFee     *string `json:"from_fee"`
	IsFeatured  *bool   `json:"is_featured"`
	SortOrder   int     `json:"sort_order"`
	IsActive    *bool   `json:"is_active"`
}

// AdminCreateEventRequest lets an admin log a phone/walk-in request.
type AdminCreateEventRequest struct {
	UserID       *int64 `json:"user_id"`
	CustomerName string `json:"customer_name"`
	CreateEventRequest
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// QuoteRequest sets (or revises) the admin quote for a request.
type QuoteRequest struct {
	QuotedPrice string  `json:"quoted_price"`
	AdminNote   *string `json:"admin_note"`
}

func derefBool(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}
