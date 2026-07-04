package mobility

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidCategory = errors.New("referenced car category does not exist")
	ErrPlateTaken      = errors.New("registration plate already exists")
	ErrPhoneTaken      = errors.New("driver phone already exists")
	ErrLicenseTaken    = errors.New("driver license already exists")
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

// --- car categories ---

func (s *Service) ListCarCategories(ctx context.Context, activeOnly bool) ([]CarCategory, error) {
	return s.repo.ListCarCategories(ctx, activeOnly)
}

func (s *Service) CreateCarCategory(ctx context.Context, req CarCategoryRequest) (*CarCategory, error) {
	slug, err := s.uniqueSlug(ctx, req.Name, 0, s.repo.CarCategorySlugExists)
	if err != nil {
		return nil, err
	}
	c := &CarCategory{
		Name:             strings.TrimSpace(req.Name),
		Slug:             slug,
		Description:      req.Description,
		DefaultDailyRate: defaultRate(req.DefaultDailyRate),
		IsCargoTransport: derefBool(req.IsCargoTransport, false),
		CargoPerKmRate:   defaultRate(req.CargoPerKmRate),
		CargoMinimumRate: defaultRate(req.CargoMinimumRate),
		SortOrder:        req.SortOrder,
		IsActive:         derefBool(req.IsActive, true),
	}
	if !c.IsCargoTransport {
		c.CargoPerKmRate = "0"
		c.CargoMinimumRate = "0"
	}
	id, err := s.repo.CreateCarCategory(ctx, c)
	if err != nil {
		return nil, err
	}
	return s.repo.GetCarCategoryByID(ctx, id)
}

func (s *Service) UpdateCarCategory(ctx context.Context, id int64, req CarCategoryRequest) (*CarCategory, error) {
	c, err := s.repo.GetCarCategoryByID(ctx, id)
	if err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(ctx, req.Name, id, s.repo.CarCategorySlugExists)
	if err != nil {
		return nil, err
	}
	c.Name = strings.TrimSpace(req.Name)
	c.Slug = slug
	c.Description = req.Description
	c.DefaultDailyRate = defaultRate(req.DefaultDailyRate)
	c.IsCargoTransport = derefBool(req.IsCargoTransport, false)
	c.CargoPerKmRate = defaultRate(req.CargoPerKmRate)
	c.CargoMinimumRate = defaultRate(req.CargoMinimumRate)
	if !c.IsCargoTransport {
		c.CargoPerKmRate = "0"
		c.CargoMinimumRate = "0"
	}
	c.SortOrder = req.SortOrder
	if req.IsActive != nil {
		c.IsActive = *req.IsActive
	}
	if err := s.repo.UpdateCarCategory(ctx, c); err != nil {
		return nil, err
	}
	return s.repo.GetCarCategoryByID(ctx, id)
}

func (s *Service) DeleteCarCategory(ctx context.Context, id int64) error {
	return s.repo.DeleteCarCategory(ctx, id)
}

// --- cars ---

func (s *Service) ListCars(ctx context.Context, f CarFilter) ([]Car, int, error) {
	return s.repo.ListCars(ctx, f)
}

func (s *Service) GetCarBySlug(ctx context.Context, slug string, publicOnly bool) (*CarDetail, error) {
	c, err := s.repo.GetCarBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if publicOnly && c.Status == CarStatusInactive {
		return nil, ErrCarNotFound
	}
	return s.assembleDetail(ctx, c)
}

func (s *Service) GetCarByID(ctx context.Context, id int64) (*CarDetail, error) {
	c, err := s.repo.GetCarByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.assembleDetail(ctx, c)
}

func (s *Service) GetCarOverview(ctx context.Context, id int64) (*CarOverview, error) {
	car, err := s.GetCarByID(ctx, id)
	if err != nil {
		return nil, err
	}
	stats, err := s.repo.CarUsageStats(ctx, id)
	if err != nil {
		return nil, err
	}
	bookings, err := s.repo.ListBookingsByCar(ctx, id, 500)
	if err != nil {
		return nil, err
	}
	return &CarOverview{Car: car, Stats: *stats, Bookings: bookings}, nil
}

func (s *Service) CreateCar(ctx context.Context, req CarRequest) (*CarDetail, error) {
	// Validating the category also gives us its default rate to seed the car.
	cat, err := s.repo.GetCarCategoryByID(ctx, req.CategoryID)
	if err != nil {
		if errors.Is(err, ErrCarCategoryNotFound) {
			return nil, ErrInvalidCategory
		}
		return nil, err
	}

	if err := s.ensurePlateFree(ctx, req.RegistrationPlate, 0); err != nil {
		return nil, err
	}

	rate := strings.TrimSpace(req.DailyRate)
	if rate == "" {
		rate = cat.DefaultDailyRate // pricing cascade: category default seeds the car
	}
	slug, err := s.uniqueSlug(ctx, req.Name, 0, s.repo.CarSlugExists)
	if err != nil {
		return nil, err
	}

	c := &Car{
		CategoryID:        req.CategoryID,
		Name:              strings.TrimSpace(req.Name),
		Slug:              slug,
		Make:              req.Make,
		Model:             req.Model,
		Year:              req.Year,
		RegistrationPlate: normalizePlate(req.RegistrationPlate),
		Color:             req.Color,
		Seats:             req.Seats,
		Transmission:      req.Transmission,
		FuelType:          req.FuelType,
		DailyRate:         rate,
		Attributes:        req.Attributes,
		Description:       req.Description,
		Status:            storedCarStatus(statusOr(req.Status, CarStatusAvailable)),
	}
	id, err := s.repo.CreateCar(ctx, c)
	if err != nil {
		return nil, err
	}
	return s.GetCarByID(ctx, id)
}

func (s *Service) UpdateCar(ctx context.Context, id int64, req CarRequest) (*CarDetail, error) {
	c, err := s.repo.GetCarByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.mustCategoryExist(ctx, req.CategoryID); err != nil {
		return nil, err
	}
	if err := s.ensurePlateFree(ctx, req.RegistrationPlate, id); err != nil {
		return nil, err
	}

	rate := strings.TrimSpace(req.DailyRate)
	if rate == "" {
		rate = c.DailyRate // keep the existing rate when none supplied
	}
	slug, err := s.uniqueSlug(ctx, req.Name, id, s.repo.CarSlugExists)
	if err != nil {
		return nil, err
	}

	c.CategoryID = req.CategoryID
	c.Name = strings.TrimSpace(req.Name)
	c.Slug = slug
	c.Make = req.Make
	c.Model = req.Model
	c.Year = req.Year
	c.RegistrationPlate = normalizePlate(req.RegistrationPlate)
	c.Color = req.Color
	c.Seats = req.Seats
	c.Transmission = req.Transmission
	c.FuelType = req.FuelType
	c.DailyRate = rate
	c.Attributes = req.Attributes
	c.Description = req.Description
	if req.Status != nil {
		c.Status = storedCarStatus(*req.Status)
	} else if c.BaseStatus != "" {
		c.Status = c.BaseStatus
	}
	if err := s.repo.UpdateCar(ctx, c); err != nil {
		return nil, err
	}
	return s.GetCarByID(ctx, id)
}

func (s *Service) DeleteCar(ctx context.Context, id int64) error {
	return s.repo.DeleteCar(ctx, id)
}

// --- car images ---

func (s *Service) CreateCarImage(ctx context.Context, carID int64, req CarImageRequest) (*CarImage, error) {
	if _, err := s.repo.GetCarByID(ctx, carID); err != nil {
		return nil, err
	}
	im := &CarImage{
		CarID:     carID,
		URL:       strings.TrimSpace(req.URL),
		AltText:   req.AltText,
		IsPrimary: req.IsPrimary,
		SortOrder: req.SortOrder,
	}
	id, err := s.repo.CreateCarImage(ctx, im)
	if err != nil {
		return nil, err
	}
	im.ID = id
	return im, nil
}

func (s *Service) DeleteCarImage(ctx context.Context, id int64) error {
	img, err := s.repo.GetCarImageByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteCarImage(ctx, id); err != nil {
		return err
	}
	// Best-effort cloud cleanup.
	if s.images != nil && img.URL != "" {
		_ = s.images.DeleteByURL(ctx, img.URL)
	}
	return nil
}

// --- drivers ---

func (s *Service) ListDrivers(ctx context.Context, status string) ([]Driver, error) {
	return s.repo.ListDrivers(ctx, status)
}

func (s *Service) GetDriver(ctx context.Context, id int64) (*Driver, error) {
	return s.repo.GetDriverByID(ctx, id)
}

func (s *Service) CreateDriver(ctx context.Context, req DriverRequest) (*Driver, error) {
	if err := s.ensureDriverUnique(ctx, req.Phone, req.LicenseNumber, 0); err != nil {
		return nil, err
	}
	d := &Driver{
		FullName:      strings.TrimSpace(req.FullName),
		Phone:         strings.TrimSpace(req.Phone),
		LicenseNumber: strings.TrimSpace(req.LicenseNumber),
		Status:        statusOr(req.Status, DriverStatusAvailable),
		Notes:         req.Notes,
	}
	id, err := s.repo.CreateDriver(ctx, d)
	if err != nil {
		return nil, err
	}
	return s.repo.GetDriverByID(ctx, id)
}

func (s *Service) UpdateDriver(ctx context.Context, id int64, req DriverRequest) (*Driver, error) {
	d, err := s.repo.GetDriverByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureDriverUnique(ctx, req.Phone, req.LicenseNumber, id); err != nil {
		return nil, err
	}
	d.FullName = strings.TrimSpace(req.FullName)
	d.Phone = strings.TrimSpace(req.Phone)
	d.LicenseNumber = strings.TrimSpace(req.LicenseNumber)
	if req.Status != nil {
		d.Status = storedDriverStatus(d, *req.Status)
	} else if d.BaseStatus != "" {
		d.Status = d.BaseStatus
	}
	d.Notes = req.Notes
	if err := s.repo.UpdateDriver(ctx, d); err != nil {
		return nil, err
	}
	return s.repo.GetDriverByID(ctx, id)
}

func (s *Service) DeleteDriver(ctx context.Context, id int64) error {
	return s.repo.DeleteDriver(ctx, id)
}

// --- internal helpers ---

func (s *Service) assembleDetail(ctx context.Context, c *Car) (*CarDetail, error) {
	images, err := s.repo.ListImagesByCar(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	detail := &CarDetail{Car: *c, Images: images}
	if cat, err := s.repo.GetCarCategoryByID(ctx, c.CategoryID); err == nil {
		detail.Category = cat
	}
	return detail, nil
}

func (s *Service) mustCategoryExist(ctx context.Context, id int64) error {
	if _, err := s.repo.GetCarCategoryByID(ctx, id); err != nil {
		if errors.Is(err, ErrCarCategoryNotFound) {
			return ErrInvalidCategory
		}
		return err
	}
	return nil
}

func (s *Service) ensurePlateFree(ctx context.Context, plate *string, excludeID int64) error {
	p := normalizePlate(plate)
	if p == nil {
		return nil
	}
	taken, err := s.repo.PlateExists(ctx, *p, excludeID)
	if err != nil {
		return err
	}
	if taken {
		return ErrPlateTaken
	}
	return nil
}

func (s *Service) ensureDriverUnique(ctx context.Context, phone, license string, excludeID int64) error {
	phoneTaken, err := s.repo.DriverPhoneExists(ctx, strings.TrimSpace(phone), excludeID)
	if err != nil {
		return err
	}
	if phoneTaken {
		return ErrPhoneTaken
	}
	licTaken, err := s.repo.DriverLicenseExists(ctx, strings.TrimSpace(license), excludeID)
	if err != nil {
		return err
	}
	if licTaken {
		return ErrLicenseTaken
	}
	return nil
}

func (s *Service) uniqueSlug(ctx context.Context, name string, excludeID int64, existsFn func(context.Context, string, int64) (bool, error)) (string, error) {
	base := Slugify(name)
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

func defaultRate(s string) string {
	if s = strings.TrimSpace(s); s != "" {
		return s
	}
	return "0"
}

func statusOr(s *string, def string) string {
	if s != nil && *s != "" {
		return *s
	}
	return def
}

func storedCarStatus(status string) string {
	if status == CarStatusNotAvailable {
		return CarStatusAvailable
	}
	return status
}

func storedDriverStatus(d *Driver, requested string) string {
	if d.BaseStatus == DriverStatusAvailable && d.Status == DriverStatusAssigned && requested == DriverStatusAssigned {
		return DriverStatusAvailable
	}
	return requested
}

// normalizePlate trims and drops empty plates to NULL so the unique index does
// not collide on empty strings.
func normalizePlate(plate *string) *string {
	if plate == nil {
		return nil
	}
	p := strings.TrimSpace(*plate)
	if p == "" {
		return nil
	}
	return &p
}
