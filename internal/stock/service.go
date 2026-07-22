package stock

import (
	"context"
	"errors"
	"strings"

	"github.com/trimo/backend/internal/catalog"
)

var (
	// ErrForbiddenDepartment is returned when the caller may not touch the SKU's department.
	ErrForbiddenDepartment = errors.New("you do not have access to this catalog department")
	// ErrInvalidAdjustment covers a malformed adjust request.
	ErrInvalidAdjustment = errors.New("invalid stock adjustment")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// adjustReasons are the reasons an admin may write. Reservation reasons belong
// to the orders module and are deliberately not accepted here.
var adjustReasons = map[string]bool{
	ReasonRestock:      true,
	ReasonManualAdjust: true,
	ReasonCorrection:   true,
}

// templateKeys turns a department scope into the category templates that belong
// to it — the shape the SQL actually filters on.
func templateKeys(departments []string) []string {
	keys := []string{}
	for _, department := range departments {
		keys = append(keys, catalog.TemplateKeysForDepartment(department)...)
	}
	return keys
}

// scope narrows the caller's authorized departments by the requested one.
// Returning an empty slice is meaningful: it matches nothing.
func scope(allowed []string, requested string) []string {
	if requested == "" {
		return allowed
	}
	for _, department := range allowed {
		if department == requested {
			return []string{requested}
		}
	}
	return []string{}
}

func (s *Service) List(ctx context.Context, allowed []string, f Filter) ([]Item, int, error) {
	departments := scope(allowed, f.Department)
	items, total, err := s.repo.ListItems(ctx, f, templateKeys(departments))
	if err != nil {
		return nil, 0, err
	}
	return withDepartments(items), total, nil
}

func (s *Service) Summary(ctx context.Context, allowed []string, department string) (*Summary, error) {
	return s.repo.Summary(ctx, templateKeys(scope(allowed, department)))
}

func (s *Service) LowStock(ctx context.Context, allowed []string, department string, limit int) ([]Item, error) {
	items, err := s.repo.LowStockItems(ctx, templateKeys(scope(allowed, department)), limit)
	if err != nil {
		return nil, err
	}
	return withDepartments(items), nil
}

func (s *Service) RecentMovements(ctx context.Context, allowed []string, department string, limit int) ([]MovementView, error) {
	return s.repo.RecentMovements(ctx, scope(allowed, department), limit)
}

// Get reads one SKU and checks the caller may see its department.
func (s *Service) Get(ctx context.Context, variantID int64, manage bool) (*Item, error) {
	item, err := s.repo.GetItem(ctx, variantID)
	if err != nil {
		return nil, err
	}
	item.Department = catalog.DepartmentForTemplateKey(item.TemplateKey)
	if !catalog.CanDepartment(ctx, item.Department, catalog.ResourceStock, manage) {
		return nil, ErrForbiddenDepartment
	}
	return item, nil
}

// historyDepth is how much of the ledger rides along with a single-SKU read.
const historyDepth = 25

// GetWithHistory is the detail read: the SKU plus the tail of its ledger, so
// one request backs the whole stock detail screen.
func (s *Service) GetWithHistory(ctx context.Context, variantID int64, manage bool) (*Item, error) {
	item, err := s.Get(ctx, variantID, manage)
	if err != nil {
		return nil, err
	}
	movements, _, err := s.repo.ListMovements(ctx, variantID, historyDepth, 0)
	if err != nil {
		return nil, err
	}
	item.Movements = movements
	return item, nil
}

func (s *Service) Movements(ctx context.Context, variantID int64, limit, offset int) ([]MovementView, int, error) {
	if _, err := s.Get(ctx, variantID, false); err != nil {
		return nil, 0, err
	}
	return s.repo.ListMovements(ctx, variantID, limit, offset)
}

// Adjust restocks, corrects, or recounts a SKU and appends the ledger entry.
func (s *Service) Adjust(ctx context.Context, variantID int64, req AdjustRequest, actor *int64) (*Item, error) {
	item, err := s.Get(ctx, variantID, true)
	if err != nil {
		return nil, err
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = ModeAdjust
	}
	if mode != ModeAdjust && mode != ModeSet {
		return nil, ErrInvalidAdjustment
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = ReasonManualAdjust
	}
	if !adjustReasons[reason] {
		return nil, ErrInvalidAdjustment
	}
	if mode == ModeAdjust && req.Quantity == 0 {
		return nil, ErrInvalidAdjustment
	}
	if mode == ModeSet && req.Quantity < 0 {
		return nil, ErrNegativeStock
	}
	note := trimmedNote(req.Note)
	if _, err := s.repo.Apply(ctx, variantID, mode, req.Quantity, reason, note, actor, item.Department); err != nil {
		return nil, err
	}
	return s.GetWithHistory(ctx, variantID, true)
}

func (s *Service) SetThreshold(ctx context.Context, variantID int64, req ThresholdRequest) (*Item, error) {
	if _, err := s.Get(ctx, variantID, true); err != nil {
		return nil, err
	}
	if req.ReorderThreshold < 0 {
		return nil, ErrInvalidAdjustment
	}
	if err := s.repo.SetThreshold(ctx, variantID, req.ReorderThreshold); err != nil {
		return nil, err
	}
	return s.GetWithHistory(ctx, variantID, true)
}

// withDepartments resolves each row's department from its category template, so
// the client never has to know the template registry.
func withDepartments(items []Item) []Item {
	for i := range items {
		items[i].Department = catalog.DepartmentForTemplateKey(items[i].TemplateKey)
	}
	return items
}

func trimmedNote(note *string) *string {
	if note == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*note)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
