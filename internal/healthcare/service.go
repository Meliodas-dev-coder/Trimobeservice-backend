package healthcare

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
	ErrInvalidCategory      = errors.New("referenced healthcare service category does not exist")
	ErrCustomerMissing      = errors.New("customer not found")
	ErrServiceUnavailable   = errors.New("the selected service is unavailable")
	ErrServiceTypeMismatch  = errors.New("the selected service does not match the request type")
	ErrMissingWindow        = errors.New("a start date is required for a package request")
	ErrInvalidTransition    = errors.New("invalid status transition")
	ErrNotCancellable       = errors.New("healthcare request can no longer be cancelled")
	ErrNotQuotable          = errors.New("a quote can no longer be set for this request")
	ErrPractitionerInactive = errors.New("practitioner is inactive")
	ErrRequestClosed        = errors.New("request is completed or cancelled")
	ErrPhoneTaken           = errors.New("phone already belongs to another practitioner")
	ErrLicenseTaken         = errors.New("license number already belongs to another practitioner")
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

// ===================== roster: practitioners =====================

func (s *Service) ListPractitioners(ctx context.Context, f PractitionerFilter) ([]Practitioner, int, error) {
	return s.repo.ListPractitioners(ctx, f)
}

func (s *Service) GetPractitioner(ctx context.Context, id int64) (*Practitioner, error) {
	return s.repo.GetPractitionerByID(ctx, id)
}

func (s *Service) CreatePractitioner(ctx context.Context, req PractitionerRequest) (*Practitioner, error) {
	if err := s.ensurePractitionerUnique(ctx, req.Phone, req.LicenseNumber, 0); err != nil {
		return nil, err
	}
	p := &Practitioner{Status: PractitionerActive}
	applyPractitioner(p, req, true)
	id, err := s.repo.CreatePractitioner(ctx, p)
	if err != nil {
		return nil, err
	}
	return s.repo.GetPractitionerByID(ctx, id)
}

func (s *Service) UpdatePractitioner(ctx context.Context, id int64, req PractitionerRequest) (*Practitioner, error) {
	p, err := s.repo.GetPractitionerByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.ensurePractitionerUnique(ctx, req.Phone, req.LicenseNumber, id); err != nil {
		return nil, err
	}
	applyPractitioner(p, req, false)
	if err := s.repo.UpdatePractitioner(ctx, p); err != nil {
		return nil, err
	}
	return s.repo.GetPractitionerByID(ctx, id)
}

func (s *Service) DeletePractitioner(ctx context.Context, id int64) error {
	p, err := s.repo.GetPractitionerByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeletePractitioner(ctx, id); err != nil {
		return err
	}
	if s.images != nil && p.PhotoURL != nil && *p.PhotoURL != "" {
		_ = s.images.DeleteByURL(ctx, *p.PhotoURL)
	}
	return nil
}

func applyPractitioner(p *Practitioner, req PractitionerRequest, creating bool) {
	p.Type = strings.TrimSpace(req.Type)
	p.FullName = strings.TrimSpace(req.FullName)
	p.Specialty = req.Specialty
	p.Phone = strings.TrimSpace(req.Phone)
	p.Email = req.Email
	p.LicenseNumber = blankToNil(req.LicenseNumber)
	p.Bio = req.Bio
	p.Translations = req.Translations
	p.PhotoURL = req.PhotoURL
	if creating {
		p.Status = statusOr(req.Status, PractitionerActive)
	} else if req.Status != nil {
		p.Status = *req.Status
	}
}

func (s *Service) ensurePractitionerUnique(ctx context.Context, phone string, license *string, excludeID int64) error {
	phoneTaken, err := s.repo.PractitionerPhoneExists(ctx, strings.TrimSpace(phone), excludeID)
	if err != nil {
		return err
	}
	if phoneTaken {
		return ErrPhoneTaken
	}
	if lic := blankToNil(license); lic != nil {
		licTaken, err := s.repo.PractitionerLicenseExists(ctx, *lic, excludeID)
		if err != nil {
			return err
		}
		if licTaken {
			return ErrLicenseTaken
		}
	}
	return nil
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

func (s *Service) ListServices(ctx context.Context, f ServiceFilter) ([]HealthcareService, int, error) {
	return s.repo.ListServices(ctx, f)
}

func (s *Service) GetServiceBySlug(ctx context.Context, slug string, publicOnly bool) (*ServiceDetail, error) {
	svc, err := s.repo.GetServiceBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if publicOnly && !svc.IsActive {
		return nil, ErrServiceNotFound
	}
	return s.serviceDetail(ctx, svc)
}

func (s *Service) GetServiceByID(ctx context.Context, id int64) (*ServiceDetail, error) {
	svc, err := s.repo.GetServiceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.serviceDetail(ctx, svc)
}

func (s *Service) CreateService(ctx context.Context, req ServiceRequest) (*ServiceDetail, error) {
	if err := s.mustCategoryExist(ctx, req.CategoryID); err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(ctx, req.Name, 0, s.repo.ServiceSlugExists)
	if err != nil {
		return nil, err
	}
	svc := serviceFromRequest(&HealthcareService{Slug: slug}, req, true)
	id, err := s.repo.CreateService(ctx, svc, staffFromRequest(req))
	if err != nil {
		return nil, err
	}
	return s.GetServiceByID(ctx, id)
}

func (s *Service) UpdateService(ctx context.Context, id int64, req ServiceRequest) (*ServiceDetail, error) {
	existing, err := s.repo.GetServiceByID(ctx, id)
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
	existing.Slug = slug
	svc := serviceFromRequest(existing, req, false)
	if err := s.repo.UpdateService(ctx, svc, staffFromRequest(req)); err != nil {
		return nil, err
	}
	return s.GetServiceByID(ctx, id)
}

func (s *Service) DeleteService(ctx context.Context, id int64) error {
	svc, err := s.repo.GetServiceByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteService(ctx, id); err != nil {
		return err
	}
	if s.images != nil && svc.ImageURL != nil && *svc.ImageURL != "" {
		_ = s.images.DeleteByURL(ctx, *svc.ImageURL)
	}
	return nil
}

// serviceFromRequest maps a request onto a service. Package-only fields (price,
// duration) are kept as-is; a consultation simply leaves them nil.
func serviceFromRequest(svc *HealthcareService, req ServiceRequest, creating bool) *HealthcareService {
	svc.CategoryID = req.CategoryID
	svc.Name = strings.TrimSpace(req.Name)
	svc.Description = req.Description
	svc.Translations = req.Translations
	svc.ServiceType = normalizeServiceType(req.ServiceType)
	svc.FromPrice = normalizeMoney(req.FromPrice)
	svc.Price = normalizeMoney(req.Price)
	svc.PriceUnit = req.PriceUnit
	svc.DurationDays = req.DurationDays
	svc.ImageURL = req.ImageURL
	svc.Attributes = req.Attributes
	svc.SortOrder = req.SortOrder
	if creating {
		svc.IsActive = derefBool(req.IsActive, true)
	} else if req.IsActive != nil {
		svc.IsActive = *req.IsActive
	}
	// Consultations carry no package staff or fixed price.
	if svc.ServiceType == ServiceConsultation {
		svc.Price = nil
		svc.DurationDays = nil
	}
	return svc
}

func staffFromRequest(req ServiceRequest) []PackageStaff {
	if normalizeServiceType(req.ServiceType) != ServicePackage {
		return nil
	}
	out := make([]PackageStaff, 0, len(req.Staff))
	for _, l := range req.Staff {
		t := strings.TrimSpace(l.PractitionerType)
		if t != TypeDoctor && t != TypeNurse {
			continue
		}
		qty := l.Quantity
		if qty < 1 {
			qty = 1
		}
		out = append(out, PackageStaff{PractitionerType: t, Quantity: qty})
	}
	return out
}

func (s *Service) serviceDetail(ctx context.Context, svc *HealthcareService) (*ServiceDetail, error) {
	staff, err := s.repo.ListPackageStaff(ctx, svc.ID)
	if err != nil {
		return nil, err
	}
	return &ServiceDetail{HealthcareService: *svc, Staff: staff}, nil
}

// ===================== transactions: requests =====================

func (s *Service) Create(ctx context.Context, userID int64, req CreateRequest) (*RequestDetail, error) {
	uid := userID
	return s.create(ctx, &uid, "", req)
}

func (s *Service) CreateAdmin(ctx context.Context, req AdminCreateRequest) (*RequestDetail, error) {
	return s.create(ctx, req.UserID, req.CustomerName, req.CreateRequest)
}

func (s *Service) create(ctx context.Context, userID *int64, customerName string, req CreateRequest) (*RequestDetail, error) {
	var requestID int64
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		e := &Request{
			UserID:            userID,
			RequestNumber:     newRequestNumber(),
			RequestType:       ServiceConsultation, // default; overridden by the chosen service
			Status:            StatusRequested,
			PaymentStatus:     PaymentUnpaid,
			PatientName:       strings.TrimSpace(req.PatientName),
			PatientAge:        req.PatientAge,
			PatientGender:     blankToNil(req.PatientGender),
			PreferredAt:       req.PreferredAt,
			StartAt:           req.StartAt,
			EndAt:             req.EndAt,
			Address:           strings.TrimSpace(req.Address),
			LocationLatitude:  req.LocationLatitude,
			LocationLongitude: req.LocationLongitude,
			LocationReference: req.LocationReference,
			Symptoms:          req.Symptoms,
			ContactPhone:      strings.TrimSpace(req.ContactPhone),
			ContactEmail:      req.ContactEmail,
			Note:              req.Note,
		}

		if req.ServiceID != nil {
			row, err := s.repo.GetServiceRowTx(ctx, tx, *req.ServiceID)
			if err != nil {
				return err
			}
			if !row.IsActive {
				return ErrServiceUnavailable
			}
			e.ServiceID = &row.ID
			e.ServiceName = &row.Name
			e.RequestType = row.ServiceType
			if row.CategoryName != "" {
				cn := row.CategoryName
				e.CategoryName = &cn
			}
			if row.ServiceType == ServicePackage {
				e.PriceSnapshot = row.Price
				e.QuotedPrice = row.Price // packages are fixed price; seed the total
				if e.StartAt == nil {
					return ErrMissingWindow
				}
				if e.EndAt == nil && row.DurationDays != nil {
					end := e.StartAt.AddDate(0, 0, *row.DurationDays)
					e.EndAt = &end
				}
			} else {
				e.PriceSnapshot = row.FromPrice
			}
		}

		name := strings.TrimSpace(customerName)
		if name != "" {
			e.CustomerName = &name
		}
		if userID != nil && e.CustomerName == nil {
			accountName, err := s.repo.CustomerName(ctx, tx, *userID)
			if err != nil {
				return err
			}
			e.CustomerName = accountName
		}

		id, err := s.repo.InsertRequest(ctx, tx, e)
		if err != nil {
			return err
		}
		requestID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, requestID)
}

// --- customer actions ---

func (s *Service) ListMyRequests(ctx context.Context, userID int64, limit, offset int) ([]Request, int, error) {
	return s.repo.List(ctx, RequestFilter{UserID: &userID, Limit: limit, Offset: offset})
}

func (s *Service) GetMyRequest(ctx context.Context, userID, id int64) (*RequestDetail, error) {
	e, err := s.repo.GetForUser(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return s.attach(ctx, e)
}

func (s *Service) CancelMyRequest(ctx context.Context, userID, id int64) (*RequestDetail, error) {
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

func (s *Service) List(ctx context.Context, f RequestFilter) ([]Request, int, error) {
	return s.repo.List(ctx, f)
}

func (s *Service) Get(ctx context.Context, id int64) (*RequestDetail, error) {
	return s.detail(ctx, id)
}

func (s *Service) UpdateStatus(ctx context.Context, id int64, target string) (*RequestDetail, error) {
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
func (s *Service) SetQuote(ctx context.Context, id int64, req QuoteRequest) (*RequestDetail, error) {
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

// Assign attaches a practitioner to a request. Overlap is NOT hard-blocked
// (v1 soft rule): it returns the practitioner's other active assignments as
// warnings so the admin can decide. Confirmed requests advance to 'assigned'.
func (s *Service) Assign(ctx context.Context, requestID int64, req AssignRequest) (*RequestDetail, []Assignment, error) {
	e, err := s.repo.GetByID(ctx, requestID)
	if err != nil {
		return nil, nil, err
	}
	if e.Status == StatusCompleted || e.Status == StatusCancelled {
		return nil, nil, ErrRequestClosed
	}
	p, err := s.repo.GetPractitionerByID(ctx, req.PractitionerID)
	if err != nil {
		return nil, nil, err
	}
	if p.Status != PractitionerActive {
		return nil, nil, ErrPractitionerInactive
	}

	warnings, err := s.repo.ActivePractitionerAssignments(ctx, p.ID, requestID)
	if err != nil {
		return nil, nil, err
	}

	a := &Assignment{
		RequestID:        requestID,
		PractitionerID:   &p.ID,
		PractitionerName: p.FullName,
		PractitionerType: p.Type,
		Note:             req.Note,
	}
	if _, err := s.repo.InsertAssignment(ctx, a); err != nil {
		return nil, nil, err
	}
	if e.Status == StatusConfirmed {
		if err := s.repo.SetStatus(ctx, requestID, StatusAssigned); err != nil {
			return nil, nil, err
		}
	}
	detail, err := s.detail(ctx, requestID)
	if err != nil {
		return nil, nil, err
	}
	return detail, warnings, nil
}

func (s *Service) Unassign(ctx context.Context, assignmentID int64) error {
	return s.repo.DeleteAssignment(ctx, assignmentID)
}

// ===================== settings =====================

func (s *Service) GetSettings(ctx context.Context) (*Settings, error) {
	return s.repo.GetSettings(ctx)
}

func (s *Service) UpdateSettings(ctx context.Context, req SettingsRequest) (*Settings, error) {
	set := &Settings{
		EmergencyPhone: blankToNil(req.EmergencyPhone),
		EmergencyHours: blankToNil(req.EmergencyHours),
		EmergencyNote:  blankToNil(req.EmergencyNote),
		Translations:   req.Translations,
	}
	if err := s.repo.UpdateSettings(ctx, set); err != nil {
		return nil, err
	}
	return s.repo.GetSettings(ctx)
}

// ===================== helpers =====================

func (s *Service) detail(ctx context.Context, id int64) (*RequestDetail, error) {
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.attach(ctx, e)
}

func (s *Service) attach(ctx context.Context, e *Request) (*RequestDetail, error) {
	assignments, err := s.repo.ListAssignments(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	var staff []PackageStaff
	if e.RequestType == ServicePackage && e.ServiceID != nil {
		staff, err = s.repo.ListPackageStaff(ctx, *e.ServiceID)
		if err != nil {
			return nil, err
		}
	}
	return &RequestDetail{Request: *e, Staff: staff, Assignments: assignments}, nil
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

// canTransition guards the admin lifecycle. Assignment auto-advances
// confirmed -> assigned; both confirmed and assigned can begin service.
func canTransition(from, to string) bool {
	switch to {
	case StatusReviewing:
		return from == StatusRequested
	case StatusQuoted:
		return from == StatusRequested || from == StatusReviewing
	case StatusConfirmed:
		return from == StatusQuoted
	case StatusAssigned:
		return from == StatusConfirmed
	case StatusInProgress:
		return from == StatusConfirmed || from == StatusAssigned
	case StatusCompleted:
		return from == StatusInProgress
	case StatusCancelled:
		switch from {
		case StatusRequested, StatusReviewing, StatusQuoted, StatusConfirmed, StatusAssigned:
			return true
		}
	}
	return false
}

func normalizeServiceType(t string) string {
	if strings.TrimSpace(t) == ServicePackage {
		return ServicePackage
	}
	return ServiceConsultation
}

func statusOr(v *string, def string) string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return def
	}
	return *v
}

// normalizeMoney trims a money string and drops blanks to NULL.
func normalizeMoney(v *string) *string {
	return blankToNil(v)
}

// blankToNil trims a pointer string and returns nil when empty.
func blankToNil(v *string) *string {
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
	return fmt.Sprintf("HC-%s-%s", time.Now().Format("20060102"), hex.EncodeToString(b))
}
