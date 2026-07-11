package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	ErrInvalidCategory    = errors.New("referenced event service category does not exist")
	ErrCustomerMissing    = errors.New("customer not found")
	ErrServiceUnavailable = errors.New("one or more selected services are unavailable")
	ErrArtistUnavailable  = errors.New("one or more selected artists are unavailable")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrNotCancellable     = errors.New("event request can no longer be cancelled")
	ErrNotQuotable        = errors.New("a quote can no longer be set for this request")
)

// ImageDeleter removes an image's backing file from object storage. Optional
// (nil when uploads are disabled).
type ImageDeleter interface {
	DeleteByURL(ctx context.Context, url string) error
}

type Service struct {
	repo   *Repository
	images ImageDeleter
}

func NewService(repo *Repository, images ImageDeleter) *Service {
	return &Service{repo: repo, images: images}
}

// ===================== catalog: categories =====================

func (s *Service) ListCategories(ctx context.Context, activeOnly bool) ([]ServiceCategory, error) {
	return s.repo.ListCategories(ctx, activeOnly)
}

func (s *Service) CreateCategory(ctx context.Context, req ServiceCategoryRequest) (*ServiceCategory, error) {
	slug, err := s.uniqueSlug(ctx, req.Name, 0, s.repo.CategorySlugExists)
	if err != nil {
		return nil, err
	}
	c := &ServiceCategory{
		Name:         strings.TrimSpace(req.Name),
		Slug:         slug,
		Description:  req.Description,
		Translations: req.Translations,
		Icon:         req.Icon,
		ImageURL:     req.ImageURL,
		SortOrder:    req.SortOrder,
		IsActive:     derefBool(req.IsActive, true),
	}
	id, err := s.repo.CreateCategory(ctx, c)
	if err != nil {
		return nil, err
	}
	return s.repo.GetCategoryByID(ctx, id)
}

func (s *Service) UpdateCategory(ctx context.Context, id int64, req ServiceCategoryRequest) (*ServiceCategory, error) {
	c, err := s.repo.GetCategoryByID(ctx, id)
	if err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(ctx, req.Name, id, s.repo.CategorySlugExists)
	if err != nil {
		return nil, err
	}
	c.Name = strings.TrimSpace(req.Name)
	c.Slug = slug
	c.Description = req.Description
	c.Translations = req.Translations
	c.Icon = req.Icon
	c.ImageURL = req.ImageURL
	c.SortOrder = req.SortOrder
	if req.IsActive != nil {
		c.IsActive = *req.IsActive
	}
	if err := s.repo.UpdateCategory(ctx, c); err != nil {
		return nil, err
	}
	return s.repo.GetCategoryByID(ctx, id)
}

func (s *Service) DeleteCategory(ctx context.Context, id int64) error {
	return s.repo.DeleteCategory(ctx, id)
}

// ===================== catalog: services =====================

func (s *Service) ListServices(ctx context.Context, f ServiceFilter) ([]EventService, int, error) {
	return s.repo.ListServices(ctx, f)
}

func (s *Service) GetServiceBySlug(ctx context.Context, slug string, publicOnly bool) (*EventService, error) {
	svc, err := s.repo.GetServiceBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if publicOnly && !svc.IsActive {
		return nil, ErrServiceNotFound
	}
	return svc, nil
}

func (s *Service) GetServiceByID(ctx context.Context, id int64) (*EventService, error) {
	return s.repo.GetServiceByID(ctx, id)
}

func (s *Service) CreateService(ctx context.Context, req ServiceRequest) (*EventService, error) {
	if err := s.mustCategoryExist(ctx, req.CategoryID); err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(ctx, req.Name, 0, s.repo.ServiceSlugExists)
	if err != nil {
		return nil, err
	}
	svc := &EventService{
		CategoryID:   req.CategoryID,
		Name:         strings.TrimSpace(req.Name),
		Slug:         slug,
		Description:  req.Description,
		Translations: req.Translations,
		FromPrice:    normalizeMoney(req.FromPrice),
		PriceUnit:    req.PriceUnit,
		ImageURL:     req.ImageURL,
		Attributes:   req.Attributes,
		SortOrder:    req.SortOrder,
		IsActive:     derefBool(req.IsActive, true),
	}
	id, err := s.repo.CreateService(ctx, svc)
	if err != nil {
		return nil, err
	}
	return s.repo.GetServiceByID(ctx, id)
}

func (s *Service) UpdateService(ctx context.Context, id int64, req ServiceRequest) (*EventService, error) {
	svc, err := s.repo.GetServiceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.mustCategoryExist(ctx, req.CategoryID); err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(ctx, req.Name, id, s.repo.ServiceSlugExists)
	if err != nil {
		return nil, err
	}
	svc.CategoryID = req.CategoryID
	svc.Name = strings.TrimSpace(req.Name)
	svc.Slug = slug
	svc.Description = req.Description
	svc.Translations = req.Translations
	svc.FromPrice = normalizeMoney(req.FromPrice)
	svc.PriceUnit = req.PriceUnit
	svc.ImageURL = req.ImageURL
	svc.Attributes = req.Attributes
	svc.SortOrder = req.SortOrder
	if req.IsActive != nil {
		svc.IsActive = *req.IsActive
	}
	if err := s.repo.UpdateService(ctx, svc); err != nil {
		return nil, err
	}
	return s.repo.GetServiceByID(ctx, id)
}

func (s *Service) DeleteService(ctx context.Context, id int64) error {
	svc, err := s.repo.GetServiceByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteService(ctx, id); err != nil {
		return err
	}
	// Best-effort cloud cleanup: an orphaned file is preferable to a failed delete.
	if s.images != nil && svc.ImageURL != nil && *svc.ImageURL != "" {
		_ = s.images.DeleteByURL(ctx, *svc.ImageURL)
	}
	return nil
}

// ===================== catalog: artists =====================

func (s *Service) ListArtists(ctx context.Context, f ArtistFilter) ([]Artist, int, error) {
	return s.repo.ListArtists(ctx, f)
}

func (s *Service) GetArtistBySlug(ctx context.Context, slug string, publicOnly bool) (*Artist, error) {
	a, err := s.repo.GetArtistBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if publicOnly && !a.IsActive {
		return nil, ErrArtistNotFound
	}
	return a, nil
}

func (s *Service) GetArtistByID(ctx context.Context, id int64) (*Artist, error) {
	return s.repo.GetArtistByID(ctx, id)
}

func (s *Service) CreateArtist(ctx context.Context, req ArtistRequest) (*Artist, error) {
	slug, err := s.uniqueSlug(ctx, req.StageName, 0, s.repo.ArtistSlugExists)
	if err != nil {
		return nil, err
	}
	a := artistFromRequest(&Artist{Slug: slug}, req, true)
	id, err := s.repo.CreateArtist(ctx, a)
	if err != nil {
		return nil, err
	}
	return s.repo.GetArtistByID(ctx, id)
}

func (s *Service) UpdateArtist(ctx context.Context, id int64, req ArtistRequest) (*Artist, error) {
	a, err := s.repo.GetArtistByID(ctx, id)
	if err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(ctx, req.StageName, id, s.repo.ArtistSlugExists)
	if err != nil {
		return nil, err
	}
	a.Slug = slug
	a = artistFromRequest(a, req, false)
	if err := s.repo.UpdateArtist(ctx, a); err != nil {
		return nil, err
	}
	return s.repo.GetArtistByID(ctx, id)
}

func (s *Service) DeleteArtist(ctx context.Context, id int64) error {
	a, err := s.repo.GetArtistByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteArtist(ctx, id); err != nil {
		return err
	}
	// Best-effort cloud cleanup of the profile photo.
	if s.images != nil && a.PhotoURL != nil && *a.PhotoURL != "" {
		_ = s.images.DeleteByURL(ctx, *a.PhotoURL)
	}
	return nil
}

// artistFromRequest maps a request onto an artist. When creating, is_active/
// is_featured default to true/false; when updating they change only if provided.
func artistFromRequest(a *Artist, req ArtistRequest, creating bool) *Artist {
	a.StageName = strings.TrimSpace(req.StageName)
	a.Tagline = req.Tagline
	a.Bio = req.Bio
	a.Translations = req.Translations
	a.HomeBase = req.HomeBase
	a.PhotoURL = req.PhotoURL
	a.GroupSize = req.GroupSize
	a.Genres = req.Genres
	a.Formats = req.Formats
	a.Languages = req.Languages
	a.Occasions = req.Occasions
	a.SampleLinks = req.SampleLinks
	a.SocialLinks = req.SocialLinks
	a.FromFee = normalizeMoney(req.FromFee)
	a.SortOrder = req.SortOrder
	if creating {
		a.IsFeatured = derefBool(req.IsFeatured, false)
		a.IsActive = derefBool(req.IsActive, true)
	} else {
		if req.IsFeatured != nil {
			a.IsFeatured = *req.IsFeatured
		}
		if req.IsActive != nil {
			a.IsActive = *req.IsActive
		}
	}
	return a
}

// ===================== transactions: event requests =====================

func (s *Service) Create(ctx context.Context, userID int64, req CreateEventRequest) (*EventRequestDetail, error) {
	uid := userID
	return s.create(ctx, &uid, "", req)
}

func (s *Service) CreateAdmin(ctx context.Context, req AdminCreateEventRequest) (*EventRequestDetail, error) {
	return s.create(ctx, req.UserID, req.CustomerName, req.CreateEventRequest)
}

func (s *Service) create(ctx context.Context, userID *int64, customerName string, req CreateEventRequest) (*EventRequestDetail, error) {
	var requestID int64
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		ids := make([]int64, 0, len(req.Services))
		for _, sel := range req.Services {
			ids = append(ids, sel.ServiceID)
		}
		rows, err := s.repo.GetServiceRowsTx(ctx, tx, ids)
		if err != nil {
			return err
		}
		byID := make(map[int64]serviceRow, len(rows))
		for _, row := range rows {
			byID[row.ID] = row
		}
		// Every selected service must exist and be active.
		for _, sel := range req.Services {
			row, ok := byID[sel.ServiceID]
			if !ok || !row.IsActive {
				return ErrServiceUnavailable
			}
		}

		name := strings.TrimSpace(customerName)
		var customerNameSnapshot *string
		if name != "" {
			customerNameSnapshot = &name
		}
		if userID != nil && customerNameSnapshot == nil {
			accountName, err := s.repo.CustomerName(ctx, tx, *userID)
			if err != nil {
				return err
			}
			customerNameSnapshot = accountName
		}

		e := &EventRequest{
			UserID:        userID,
			CustomerName:  customerNameSnapshot,
			RequestNumber: newRequestNumber(),
			EventType:     strings.TrimSpace(req.EventType),
			Status:        StatusRequested,
			PaymentStatus: PaymentUnpaid,
			EventStart:    req.EventStart,
			EventEnd:      req.EventEnd,
			Location:      strings.TrimSpace(req.Location),
			GuestCount:    req.GuestCount,
			Budget:        normalizeMoney(req.Budget),
			ContactPhone:  strings.TrimSpace(req.ContactPhone),
			ContactEmail:  req.ContactEmail,
			Note:          req.Note,
		}
		id, err := s.repo.InsertRequest(ctx, tx, e)
		if err != nil {
			return err
		}
		requestID = id

		for _, sel := range req.Services {
			row := byID[sel.ServiceID]
			qty := sel.Quantity
			if qty < 1 {
				qty = 1
			}
			serviceID := sel.ServiceID
			var categoryName *string
			if row.CategoryName != "" {
				cn := row.CategoryName
				categoryName = &cn
			}
			line := &RequestService{
				RequestID:         id,
				ServiceID:         &serviceID,
				ServiceName:       row.Name,
				CategoryName:      categoryName,
				FromPriceSnapshot: row.FromPrice,
				Quantity:          qty,
				Note:              sel.Note,
			}
			if err := s.repo.InsertRequestService(ctx, tx, line); err != nil {
				return err
			}
		}

		if len(req.Artists) > 0 {
			artistIDs := make([]int64, 0, len(req.Artists))
			for _, sel := range req.Artists {
				artistIDs = append(artistIDs, sel.ArtistID)
			}
			artistRows, err := s.repo.GetArtistRowsTx(ctx, tx, artistIDs)
			if err != nil {
				return err
			}
			artistByID := make(map[int64]artistRow, len(artistRows))
			for _, row := range artistRows {
				artistByID[row.ID] = row
			}
			for _, sel := range req.Artists {
				row, ok := artistByID[sel.ArtistID]
				if !ok || !row.IsActive {
					return ErrArtistUnavailable
				}
				artistID := sel.ArtistID
				line := &RequestArtist{
					RequestID:   id,
					ArtistID:    &artistID,
					ArtistName:  row.StageName,
					FeeSnapshot: row.FromFee,
					Note:        sel.Note,
				}
				if err := s.repo.InsertRequestArtist(ctx, tx, line); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, requestID)
}

// --- customer actions ---

func (s *Service) ListMyRequests(ctx context.Context, userID int64, limit, offset int) ([]EventRequest, int, error) {
	return s.repo.List(ctx, EventRequestFilter{UserID: &userID, Limit: limit, Offset: offset})
}

func (s *Service) GetMyRequest(ctx context.Context, userID, id int64) (*EventRequestDetail, error) {
	e, err := s.repo.GetForUser(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return s.attachServices(ctx, e)
}

func (s *Service) CancelMyRequest(ctx context.Context, userID, id int64) (*EventRequestDetail, error) {
	e, err := s.repo.GetForUser(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if !isCancellable(e.Status) || e.PaymentStatus == PaymentPaid {
		return nil, ErrNotCancellable
	}
	if err := s.repo.SetStatus(ctx, id, StatusCancelled); err != nil {
		return nil, err
	}
	return s.detail(ctx, id)
}

// --- admin actions ---

func (s *Service) List(ctx context.Context, f EventRequestFilter) ([]EventRequest, int, error) {
	return s.repo.List(ctx, f)
}

func (s *Service) Get(ctx context.Context, id int64) (*EventRequestDetail, error) {
	return s.detail(ctx, id)
}

func (s *Service) UpdateStatus(ctx context.Context, id int64, target string) (*EventRequestDetail, error) {
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canTransition(e.Status, target) {
		return nil, ErrInvalidTransition
	}
	if err := s.repo.SetStatus(ctx, id, target); err != nil {
		return nil, err
	}
	return s.detail(ctx, id)
}

// SetQuote records (or revises) the admin quote. From an early state it also
// advances the request to 'quoted'.
func (s *Service) SetQuote(ctx context.Context, id int64, req QuoteRequest) (*EventRequestDetail, error) {
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !isQuotable(e.Status) {
		return nil, ErrNotQuotable
	}
	advance := e.Status == StatusRequested || e.Status == StatusReviewing
	if err := s.repo.SetQuote(ctx, id, strings.TrimSpace(req.QuotedPrice), req.AdminNote, advance); err != nil {
		return nil, err
	}
	return s.detail(ctx, id)
}

// --- helpers ---

func (s *Service) detail(ctx context.Context, id int64) (*EventRequestDetail, error) {
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.attachServices(ctx, e)
}

func (s *Service) attachServices(ctx context.Context, e *EventRequest) (*EventRequestDetail, error) {
	services, err := s.repo.ListRequestServices(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	artists, err := s.repo.ListRequestArtists(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	return &EventRequestDetail{EventRequest: *e, Services: services, Artists: artists}, nil
}

func (s *Service) mustCategoryExist(ctx context.Context, id int64) error {
	ok, err := s.repo.CategoryExists(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCategory
	}
	return nil
}

// uniqueSlug builds a slug from name and appends -2, -3, … until it is free.
func (s *Service) uniqueSlug(ctx context.Context, name string, excludeID int64, existsFn func(context.Context, string, int64) (bool, error)) (string, error) {
	base := slugify(name)
	slug := base
	for i := 2; ; i++ {
		exists, err := existsFn(ctx, slug, excludeID)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

func isCancellable(status string) bool {
	switch status {
	case StatusRequested, StatusReviewing, StatusQuoted, StatusConfirmed:
		return true
	}
	return false
}

func isQuotable(status string) bool {
	switch status {
	case StatusRequested, StatusReviewing, StatusQuoted:
		return true
	}
	return false
}

func canTransition(from, to string) bool {
	switch to {
	case StatusReviewing:
		return from == StatusRequested
	case StatusQuoted:
		return from == StatusRequested || from == StatusReviewing
	case StatusConfirmed:
		return from == StatusQuoted
	case StatusInProgress:
		return from == StatusConfirmed
	case StatusCompleted:
		return from == StatusInProgress
	case StatusCancelled:
		return from == StatusRequested || from == StatusReviewing || from == StatusQuoted || from == StatusConfirmed
	}
	return false
}

// normalizeMoney trims a money string and drops blanks to NULL.
func normalizeMoney(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func newRequestNumber() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("EVT-%s-%s", time.Now().Format("20060102"), hex.EncodeToString(b))
}
