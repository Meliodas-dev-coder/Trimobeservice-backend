package invoicing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/types"
)

var (
	ErrInvalidType    = errors.New("invoiceable_type must be 'order', 'booking', 'event', or 'healthcare'")
	ErrInvalidKind    = errors.New("kind must be 'proforma' or 'final'")
	ErrNotDraft       = errors.New("only a draft invoice can be modified or issued")
	ErrCannotVoid     = errors.New("only a draft or issued invoice can be voided")
	ErrNotFinalSource = errors.New("a credit note can only be created from an issued final invoice")
	ErrAlreadyCredit  = errors.New("this invoice already has a credit note")
	ErrBadAmount      = errors.New("amount must be a decimal value")
	ErrBadDate        = errors.New("due_date must be a valid date (YYYY-MM-DD)")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// --- org settings ---

func (s *Service) GetOrgSettings(ctx context.Context) (*OrgSettings, error) {
	return s.repo.GetOrgSettings(ctx)
}

func (s *Service) UpdateOrgSettings(ctx context.Context, req OrgSettingsRequest) (*OrgSettings, error) {
	cur, err := s.repo.GetOrgSettings(ctx)
	if err != nil {
		return nil, err
	}
	cur.LegalName = strings.TrimSpace(req.LegalName)
	cur.BrandName = req.BrandName
	cur.Address = req.Address
	cur.City = req.City
	cur.Phone = req.Phone
	cur.Email = req.Email
	cur.Website = req.Website
	cur.LogoURL = req.LogoURL
	cur.NIF = req.NIF
	cur.STAT = req.STAT
	cur.RCS = req.RCS
	if req.Currency != nil && strings.TrimSpace(*req.Currency) != "" {
		cur.Currency = strings.TrimSpace(*req.Currency)
	}
	if req.TaxLabel != nil && strings.TrimSpace(*req.TaxLabel) != "" {
		cur.TaxLabel = strings.TrimSpace(*req.TaxLabel)
	}
	if req.DefaultTaxRate != nil {
		cur.DefaultTaxRate = normalizeAmount(*req.DefaultTaxRate)
	}
	cur.PaymentTerms = req.PaymentTerms
	cur.BankDetails = req.BankDetails
	cur.MobileMoney = req.MobileMoney
	if req.InvoicePrefix != nil && strings.TrimSpace(*req.InvoicePrefix) != "" {
		cur.InvoicePrefix = strings.TrimSpace(*req.InvoicePrefix)
	}
	if req.ProformaPrefix != nil && strings.TrimSpace(*req.ProformaPrefix) != "" {
		cur.ProformaPrefix = strings.TrimSpace(*req.ProformaPrefix)
	}
	if req.CreditNotePrefix != nil && strings.TrimSpace(*req.CreditNotePrefix) != "" {
		cur.CreditNotePrefix = strings.TrimSpace(*req.CreditNotePrefix)
	}
	cur.FooterText = req.FooterText
	if err := s.repo.UpdateOrgSettings(ctx, cur); err != nil {
		return nil, err
	}
	return s.repo.GetOrgSettings(ctx)
}

// --- invoices ---

func (s *Service) CreateInvoice(ctx context.Context, adminID int64, req CreateInvoiceRequest) (*InvoiceDetail, error) {
	if !validType(req.InvoiceableType) {
		return nil, ErrInvalidType
	}
	kind := KindProforma
	if req.Kind != nil && strings.TrimSpace(*req.Kind) != "" {
		kind = strings.TrimSpace(*req.Kind)
	}
	if kind != KindProforma && kind != KindFinal {
		return nil, ErrInvalidKind // credit notes are created via CreateCreditNote
	}

	org, err := s.repo.GetOrgSettings(ctx)
	if err != nil {
		return nil, err
	}
	src, err := s.repo.LoadSource(ctx, req.InvoiceableType, req.InvoiceableID)
	if err != nil {
		return nil, err
	}

	taxRate := org.DefaultTaxRate
	if req.TaxRate != nil && strings.TrimSpace(*req.TaxRate) != "" {
		taxRate = normalizeAmount(*req.TaxRate)
	}
	discount := "0.00"
	if req.Discount != nil && strings.TrimSpace(*req.Discount) != "" {
		discount = normalizeAmount(*req.Discount)
	}
	discountC, err := parseCents(discount)
	if err != nil {
		return nil, ErrBadAmount
	}
	rateC, err := parseCents(taxRate)
	if err != nil {
		return nil, ErrBadAmount
	}
	subtotalC, taxC, totalC := computeTotals(src.Lines, discountC, rateC)

	dueDate, err := parseDate(req.DueDate)
	if err != nil {
		return nil, err
	}
	seller, _ := json.Marshal(org)

	inv := &Invoice{
		InvoiceableType: req.InvoiceableType,
		InvoiceableID:   req.InvoiceableID,
		SourceNumber:    strPtr(src.Number),
		Kind:            kind,
		SellerSnapshot:  types.JSONText(seller),
		BuyerName:       src.BuyerName,
		BuyerPhone:      src.BuyerPhone,
		BuyerEmail:      src.BuyerEmail,
		BuyerAddress:    src.BuyerAddress,
		Currency:        org.Currency,
		DueDate:         dueDate,
		Subtotal:        formatCents(subtotalC),
		Discount:        formatCents(discountC),
		TaxRate:         taxRate,
		TaxAmount:       formatCents(taxC),
		Total:           formatCents(totalC),
		Notes:           req.Notes,
		Terms:           req.Terms,
	}

	var newID int64
	err = s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		id, err := s.repo.InsertInvoiceTx(ctx, tx, inv)
		if err != nil {
			return err
		}
		newID = id
		return s.insertLines(ctx, tx, id, src.Lines)
	})
	if err != nil {
		return nil, err
	}

	if req.Issue {
		if _, err := s.IssueInvoice(ctx, adminID, newID); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, newID)
}

// IssueInvoice reserves the next per-(kind, year) number and marks the invoice
// issued. Atomic: the sequence row is locked to end-of-tx.
func (s *Service) IssueInvoice(ctx context.Context, adminID, id int64) (*InvoiceDetail, error) {
	org, err := s.repo.GetOrgSettings(ctx)
	if err != nil {
		return nil, err
	}
	err = s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		inv, err := s.repo.GetInvoiceForUpdateTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if inv.Status != StatusDraft {
			return ErrNotDraft
		}
		var year int
		if err := tx.GetContext(ctx, &year, `SELECT YEAR(CURRENT_DATE())`); err != nil {
			return err
		}
		seq, err := s.repo.nextSequenceTx(ctx, tx, inv.Kind, year)
		if err != nil {
			return err
		}
		number := formatNumber(prefixForKind(org, inv.Kind), year, seq)
		return s.repo.AssignNumberTx(ctx, tx, id, number, adminID)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// UpdateInvoice edits a draft's due date / discount / tax rate / notes and
// recomputes tax + total (lines are unchanged).
func (s *Service) UpdateInvoice(ctx context.Context, id int64, req UpdateInvoiceRequest) (*InvoiceDetail, error) {
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		inv, err := s.repo.GetInvoiceForUpdateTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if inv.Status != StatusDraft {
			return ErrNotDraft
		}
		if req.Discount != nil {
			inv.Discount = normalizeAmount(*req.Discount)
		}
		if req.TaxRate != nil {
			inv.TaxRate = normalizeAmount(*req.TaxRate)
		}
		if req.Notes != nil {
			inv.Notes = req.Notes
		}
		if req.Terms != nil {
			inv.Terms = req.Terms
		}
		if req.DueDate != nil {
			d, err := parseDate(req.DueDate)
			if err != nil {
				return err
			}
			inv.DueDate = d
		}
		subtotalC, _ := parseCents(inv.Subtotal)
		discountC, err := parseCents(inv.Discount)
		if err != nil {
			return ErrBadAmount
		}
		rateC, err := parseCents(inv.TaxRate)
		if err != nil {
			return ErrBadAmount
		}
		base := subtotalC - discountC
		taxC := taxCents(base, rateC)
		inv.TaxAmount = formatCents(taxC)
		inv.Total = formatCents(base + taxC)
		return s.repo.UpdateFieldsTx(ctx, tx, inv)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *Service) VoidInvoice(ctx context.Context, id int64) (*InvoiceDetail, error) {
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		inv, err := s.repo.GetInvoiceForUpdateTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if inv.Status != StatusDraft && inv.Status != StatusIssued {
			return ErrCannotVoid
		}
		return s.repo.SetStatusTx(ctx, tx, id, StatusVoid)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// CreateCreditNote issues an "avoir" reversing an issued final invoice, and
// marks the source invoice credited — all atomically.
func (s *Service) CreateCreditNote(ctx context.Context, adminID, sourceID int64) (*InvoiceDetail, error) {
	org, err := s.repo.GetOrgSettings(ctx)
	if err != nil {
		return nil, err
	}
	src, err := s.repo.GetInvoice(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if src.Kind != KindFinal || src.Status != StatusIssued {
		return nil, ErrNotFinalSource
	}
	lines, err := s.repo.ListLines(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	subtotalC, _ := parseCents(src.Subtotal)
	discountC, _ := parseCents(src.Discount)
	taxC, _ := parseCents(src.TaxAmount)
	totalC, _ := parseCents(src.Total)

	note := fmt.Sprintf("Avoir sur facture %s", derefStr(src.InvoiceNumber))
	cn := &Invoice{
		InvoiceableType: src.InvoiceableType,
		InvoiceableID:   src.InvoiceableID,
		SourceNumber:    src.SourceNumber,
		Kind:            KindCreditNote,
		SourceInvoiceID: &sourceID,
		SellerSnapshot:  src.SellerSnapshot,
		BuyerName:       src.BuyerName,
		BuyerPhone:      src.BuyerPhone,
		BuyerEmail:      src.BuyerEmail,
		BuyerAddress:    src.BuyerAddress,
		Currency:        src.Currency,
		Subtotal:        formatCents(-subtotalC),
		Discount:        formatCents(-discountC),
		TaxRate:         src.TaxRate,
		TaxAmount:       formatCents(-taxC),
		Total:           formatCents(-totalC),
		Notes:           &note,
	}

	var newID int64
	err = s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		// re-check the source under lock so two credit notes can't both win
		locked, err := s.repo.GetInvoiceForUpdateTx(ctx, tx, sourceID)
		if err != nil {
			return err
		}
		if locked.Status != StatusIssued {
			return ErrAlreadyCredit
		}
		id, err := s.repo.InsertInvoiceTx(ctx, tx, cn)
		if err != nil {
			return err
		}
		newID = id
		for i, l := range lines {
			ltC, _ := parseCents(l.LineTotal)
			upC, _ := parseCents(l.UnitPrice)
			neg := &InvoiceLine{
				Description: l.Description,
				Detail:      l.Detail,
				Quantity:    l.Quantity,
				UnitPrice:   formatCents(-upC),
				LineTotal:   formatCents(-ltC),
				SortOrder:   i,
			}
			if err := s.repo.InsertLineTx(ctx, tx, id, neg); err != nil {
				return err
			}
		}
		var year int
		if err := tx.GetContext(ctx, &year, `SELECT YEAR(CURRENT_DATE())`); err != nil {
			return err
		}
		seq, err := s.repo.nextSequenceTx(ctx, tx, KindCreditNote, year)
		if err != nil {
			return err
		}
		number := formatNumber(org.CreditNotePrefix, year, seq)
		if err := s.repo.AssignNumberTx(ctx, tx, id, number, adminID); err != nil {
			return err
		}
		return s.repo.SetStatusTx(ctx, tx, sourceID, StatusCredited)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, newID)
}

func (s *Service) DeleteInvoice(ctx context.Context, id int64) error {
	inv, err := s.repo.GetInvoice(ctx, id)
	if err != nil {
		return err
	}
	if inv.Status != StatusDraft {
		return ErrNotDraft // issued documents are voided, never deleted
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) List(ctx context.Context, f InvoiceFilter) ([]Invoice, int, error) {
	return s.repo.List(ctx, f)
}

func (s *Service) Get(ctx context.Context, id int64) (*InvoiceDetail, error) {
	inv, err := s.repo.GetInvoice(ctx, id)
	if err != nil {
		return nil, err
	}
	lines, err := s.repo.ListLines(ctx, id)
	if err != nil {
		return nil, err
	}
	paid, err := s.repo.AmountPaid(ctx, inv.InvoiceableType, inv.InvoiceableID)
	if err != nil {
		return nil, err
	}
	paidC, _ := parseCents(paid)
	totalC, _ := parseCents(inv.Total)
	return &InvoiceDetail{
		Invoice:    *inv,
		Lines:      lines,
		AmountPaid: formatCents(paidC),
		BalanceDue: formatCents(totalC - paidC),
	}, nil
}

// --- helpers ---

func (s *Service) insertLines(ctx context.Context, tx *sqlx.Tx, invoiceID int64, lines []lineInput) error {
	for i, l := range lines {
		row := &InvoiceLine{
			Description: l.Description,
			Detail:      l.Detail,
			Quantity:    formatCents(l.QuantityC),
			UnitPrice:   formatCents(l.UnitPriceC),
			LineTotal:   formatCents(l.LineTotalC),
			SortOrder:   i,
		}
		if err := s.repo.InsertLineTx(ctx, tx, invoiceID, row); err != nil {
			return err
		}
	}
	return nil
}

func computeTotals(lines []lineInput, discountC, rateC int64) (subtotalC, taxC, totalC int64) {
	for _, l := range lines {
		subtotalC += l.LineTotalC
	}
	base := subtotalC - discountC
	taxC = taxCents(base, rateC)
	return subtotalC, taxC, base + taxC
}

func validType(t string) bool {
	switch t {
	case TypeOrder, TypeBooking, TypeEvent, TypeHealthcare:
		return true
	}
	return false
}

func prefixForKind(org *OrgSettings, kind string) string {
	switch kind {
	case KindFinal:
		return org.InvoicePrefix
	case KindCreditNote:
		return org.CreditNotePrefix
	default:
		return org.ProformaPrefix
	}
}

func formatNumber(prefix string, year, seq int) string {
	return fmt.Sprintf("%s-%d-%04d", prefix, year, seq)
}

// normalizeAmount trims and, if blank, returns "0.00" — but keeps the raw string
// otherwise (validation happens separately).
func normalizeAmount(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "0.00"
	}
	return s
}

func parseDate(s *string) (*time.Time, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	d, err := time.Parse("2006-01-02", strings.TrimSpace(*s))
	if err != nil {
		return nil, ErrBadDate
	}
	return &d, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
