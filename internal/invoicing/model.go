// Package invoicing is the cross-domain billing module. An invoice is a frozen
// document generated from one of the four transaction types (order, booking,
// event, healthcare) — polymorphic via (invoiceable_type, invoiceable_id), the
// same pattern as the payments ledger. Seller identity, buyer identity, lines,
// and totals are snapshotted so later edits never rewrite an issued document.
//
// Module layering: model -> repository -> service -> handler -> routes.
package invoicing

import (
	"time"

	"github.com/jmoiron/sqlx/types"
)

// Invoiceable target types (mirror payments.Payable*).
const (
	TypeOrder      = "order"
	TypeBooking    = "booking"
	TypeEvent      = "event"
	TypeHealthcare = "healthcare"
)

// Invoice kinds.
const (
	KindProforma   = "proforma"
	KindFinal      = "final"
	KindCreditNote = "credit_note"
)

// Invoice document lifecycle. Payment state is NOT part of this — it is derived
// live from the payments ledger (see amount_paid / balance_due on InvoiceDetail).
const (
	StatusDraft    = "draft"
	StatusIssued   = "issued"
	StatusVoid     = "void"
	StatusCredited = "credited"
)

// --- domain models ---

type OrgSettings struct {
	ID               int64     `db:"id" json:"id"`
	LegalName        string    `db:"legal_name" json:"legal_name"`
	BrandName        *string   `db:"brand_name" json:"brand_name,omitempty"`
	Address          *string   `db:"address" json:"address,omitempty"`
	City             *string   `db:"city" json:"city,omitempty"`
	Phone            *string   `db:"phone" json:"phone,omitempty"`
	Email            *string   `db:"email" json:"email,omitempty"`
	Website          *string   `db:"website" json:"website,omitempty"`
	LogoURL          *string   `db:"logo_url" json:"logo_url,omitempty"`
	NIF              *string   `db:"nif" json:"nif,omitempty"`
	STAT             *string   `db:"stat" json:"stat,omitempty"`
	RCS              *string   `db:"rcs" json:"rcs,omitempty"`
	Currency         string    `db:"currency" json:"currency"`
	TaxLabel         string    `db:"tax_label" json:"tax_label"`
	DefaultTaxRate   string    `db:"default_tax_rate" json:"default_tax_rate"` // DECIMAL(5,2) as string
	PaymentTerms     *string   `db:"payment_terms" json:"payment_terms,omitempty"`
	BankDetails      *string   `db:"bank_details" json:"bank_details,omitempty"`
	MobileMoney      *string   `db:"mobile_money" json:"mobile_money,omitempty"`
	InvoicePrefix    string    `db:"invoice_prefix" json:"invoice_prefix"`
	ProformaPrefix   string    `db:"proforma_prefix" json:"proforma_prefix"`
	CreditNotePrefix string    `db:"credit_note_prefix" json:"credit_note_prefix"`
	FooterText       *string   `db:"footer_text" json:"footer_text,omitempty"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

type Invoice struct {
	ID              int64          `db:"id" json:"id"`
	InvoiceableType string         `db:"invoiceable_type" json:"invoiceable_type"`
	InvoiceableID   int64          `db:"invoiceable_id" json:"invoiceable_id"`
	SourceNumber    *string        `db:"source_number" json:"source_number,omitempty"`
	Kind            string         `db:"kind" json:"kind"`
	SourceInvoiceID *int64         `db:"source_invoice_id" json:"source_invoice_id,omitempty"`
	InvoiceNumber   *string        `db:"invoice_number" json:"invoice_number,omitempty"`
	Status          string         `db:"status" json:"status"`
	SellerSnapshot  types.JSONText `db:"seller_snapshot" json:"seller_snapshot,omitempty"`
	BuyerName       *string        `db:"buyer_name" json:"buyer_name,omitempty"`
	BuyerPhone      *string        `db:"buyer_phone" json:"buyer_phone,omitempty"`
	BuyerEmail      *string        `db:"buyer_email" json:"buyer_email,omitempty"`
	BuyerAddress    *string        `db:"buyer_address" json:"buyer_address,omitempty"`
	Currency        string         `db:"currency" json:"currency"`
	IssueDate       *time.Time     `db:"issue_date" json:"issue_date,omitempty"`
	DueDate         *time.Time     `db:"due_date" json:"due_date,omitempty"`
	Subtotal        string         `db:"subtotal" json:"subtotal"`
	Discount        string         `db:"discount" json:"discount"`
	TaxRate         string         `db:"tax_rate" json:"tax_rate"`
	TaxAmount       string         `db:"tax_amount" json:"tax_amount"`
	Total           string         `db:"total" json:"total"`
	Notes           *string        `db:"notes" json:"notes,omitempty"`
	Terms           *string        `db:"terms" json:"terms,omitempty"`
	IssuedBy        *int64         `db:"issued_by" json:"issued_by,omitempty"`
	CreatedAt       time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at" json:"updated_at"`
}

type InvoiceLine struct {
	ID          int64     `db:"id" json:"id"`
	InvoiceID   int64     `db:"invoice_id" json:"invoice_id"`
	Description string    `db:"description" json:"description"`
	Detail      *string   `db:"detail" json:"detail,omitempty"`
	Quantity    string    `db:"quantity" json:"quantity"`
	UnitPrice   string    `db:"unit_price" json:"unit_price"`
	LineTotal   string    `db:"line_total" json:"line_total"`
	SortOrder   int       `db:"sort_order" json:"sort_order"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// InvoiceDetail is an invoice plus its lines and live payment reconciliation.
type InvoiceDetail struct {
	Invoice
	Lines      []InvoiceLine `json:"lines"`
	AmountPaid string        `json:"amount_paid"` // summed live from the payments ledger
	BalanceDue string        `json:"balance_due"` // total - amount_paid
}

// InvoiceFilter drives the list query.
type InvoiceFilter struct {
	InvoiceableType string
	InvoiceableID   *int64
	Kind            string
	Status          string
	Limit           int
	Offset          int
}

// --- composition inputs (built from a transaction, then frozen) ---

// lineInput is a not-yet-persisted invoice line during composition.
type lineInput struct {
	Description string
	Detail      *string
	QuantityC   int64 // quantity in cents (supports fractional qty)
	UnitPriceC  int64 // cents
	LineTotalC  int64 // cents
}

// invoiceSource is everything composed from a transaction before an invoice row
// is written: who to bill, the reference number, and the priced lines.
type invoiceSource struct {
	Number       string
	BuyerName    *string
	BuyerPhone   *string
	BuyerEmail   *string
	BuyerAddress *string
	Lines        []lineInput
}

// --- request DTOs ---

type CreateInvoiceRequest struct {
	InvoiceableType string  `json:"invoiceable_type"`
	InvoiceableID   int64   `json:"invoiceable_id"`
	Kind            *string `json:"kind"`     // defaults to proforma
	DueDate         *string `json:"due_date"` // YYYY-MM-DD, optional
	Discount        *string `json:"discount"` // optional
	TaxRate         *string `json:"tax_rate"` // optional; defaults to org default
	Notes           *string `json:"notes"`
	Terms           *string `json:"terms"`
	Issue           bool    `json:"issue"` // create then immediately issue
}

type UpdateInvoiceRequest struct {
	DueDate  *string `json:"due_date"`
	Discount *string `json:"discount"`
	TaxRate  *string `json:"tax_rate"`
	Notes    *string `json:"notes"`
	Terms    *string `json:"terms"`
}

type OrgSettingsRequest struct {
	LegalName        string  `json:"legal_name"`
	BrandName        *string `json:"brand_name"`
	Address          *string `json:"address"`
	City             *string `json:"city"`
	Phone            *string `json:"phone"`
	Email            *string `json:"email"`
	Website          *string `json:"website"`
	LogoURL          *string `json:"logo_url"`
	NIF              *string `json:"nif"`
	STAT             *string `json:"stat"`
	RCS              *string `json:"rcs"`
	Currency         *string `json:"currency"`
	TaxLabel         *string `json:"tax_label"`
	DefaultTaxRate   *string `json:"default_tax_rate"`
	PaymentTerms     *string `json:"payment_terms"`
	BankDetails      *string `json:"bank_details"`
	MobileMoney      *string `json:"mobile_money"`
	InvoicePrefix    *string `json:"invoice_prefix"`
	ProformaPrefix   *string `json:"proforma_prefix"`
	CreditNotePrefix *string `json:"credit_note_prefix"`
	FooterText       *string `json:"footer_text"`
}
