package hr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// Issued contract / agreement documents.
//
// A document starts as a draft, is rendered from a template against real record
// data, and at issue time is FROZEN: body_rendered stops being recomputed and a
// gapless reference is assigned. Editing the template afterwards cannot change
// it, which is the whole point — a signed contract must stay what was signed.

// frenchMonths renders dates the way the French document expects. Documents are
// French-only (the locked invoicing decision), so this does not vary by locale.
var frenchMonths = [...]string{
	"janvier", "février", "mars", "avril", "mai", "juin",
	"juillet", "août", "septembre", "octobre", "novembre", "décembre",
}

func frenchDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	day := t.Day()
	if day == 1 {
		return fmt.Sprintf("1er %s %d", frenchMonths[t.Month()-1], t.Year())
	}
	return fmt.Sprintf("%d %s %d", day, frenchMonths[t.Month()-1], t.Year())
}

func frenchDateString(value sql.NullString) string {
	if !value.Valid || value.String == "" {
		return ""
	}
	raw := value.String
	if len(raw) > 10 {
		raw = raw[:10]
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return value.String
	}
	return frenchDate(parsed)
}

// frenchAmount groups thousands with a non-breaking space, as French money is
// written ("1 500 000"). The value stays a decimal string end to end.
func frenchAmount(value sql.NullString) string {
	if !value.Valid || value.String == "" {
		return ""
	}
	whole, frac, _ := strings.Cut(value.String, ".")
	if frac == "00" {
		frac = ""
	}
	var out []string
	for len(whole) > 3 {
		out = append([]string{whole[len(whole)-3:]}, out...)
		whole = whole[:len(whole)-3]
	}
	out = append([]string{whole}, out...)
	grouped := strings.Join(out, " ")
	if frac != "" {
		grouped += "," + frac
	}
	return grouped
}

type employerIdentity struct {
	LegalName string         `db:"legal_name"`
	BrandName sql.NullString `db:"brand_name"`
	Address   sql.NullString `db:"address"`
	City      sql.NullString `db:"city"`
	Phone     sql.NullString `db:"phone"`
	Email     sql.NullString `db:"email"`
	NIF       sql.NullString `db:"nif"`
	STAT      sql.NullString `db:"stat"`
	RCS       sql.NullString `db:"rcs"`
	Currency  string         `db:"currency"`
}

type employeeFacts struct {
	EmployeeNumber string         `db:"employee_number"`
	FullName       string         `db:"full_name"`
	Address        sql.NullString `db:"address"`
	Phone          sql.NullString `db:"phone"`
	Email          sql.NullString `db:"email"`
	HireDate       sql.NullString `db:"hire_date"`
	PositionTitle  sql.NullString `db:"position_title"`
	DepartmentName sql.NullString `db:"department_name"`
	ManagerName    sql.NullString `db:"manager_name"`
}

type contractFacts struct {
	ContractType     string         `db:"contract_type"`
	StartDate        sql.NullString `db:"start_date"`
	EndDate          sql.NullString `db:"end_date"`
	ProbationEndDate sql.NullString `db:"probation_end_date"`
	Salary           sql.NullString `db:"salary"`
	Currency         sql.NullString `db:"currency"`
	PayFrequency     sql.NullString `db:"pay_frequency"`
}

// ContractDocumentInput is what the console sends for a draft.
type ContractDocumentInput struct {
	TemplateID    int64             `json:"template_id"`
	EmployeeID    *int64            `json:"employee_id"`
	ContractID    *int64            `json:"contract_id"`
	PartyName     string            `json:"party_name"`
	PartyAddress  string            `json:"party_address"`
	PartyIDNumber string            `json:"party_id_number"`
	PartyPhone    string            `json:"party_phone"`
	PartyEmail    string            `json:"party_email"`
	Place         string            `json:"place"`
	IssueDate     string            `json:"issue_date"`
	Values        map[string]string `json:"values"`
}

// ContractDocumentRender is the resolved result the console previews and the
// service freezes.
type ContractDocumentRender struct {
	Rendered string   `json:"rendered"`
	Blanks   []string `json:"blanks"`
	Unknown  []string `json:"unknown"`
	Unfilled []string `json:"unfilled"`
}

// validationError surfaces field problems as a 422 through the existing
// ErrValidation mapping in writeError.
func validationError(problems map[string]string) error {
	parts := make([]string, 0, len(problems))
	for field, problem := range problems {
		parts = append(parts, field+" "+problem)
	}
	sort.Strings(parts)
	return fmt.Errorf("%w: %s", ErrValidation, strings.Join(parts, "; "))
}

// ContractDocument reads one document through the generic engine so the row is
// shaped exactly like the list rows the console already renders.
func (s *Service) ContractDocument(ctx context.Context, id int64) (map[string]any, error) {
	return s.repo.Get(ctx, resources["contract-documents"], id)
}

func nullableTrimmed(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func stringOrEmpty(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// resolveTokens gathers every non-custom token value for a document. Missing
// optional facts resolve to an empty string rather than an error: an employer
// with no RCS yet, or a permanent contract with no end date, must still be able
// to issue a document.
func (s *Service) resolveTokens(ctx context.Context, q sqlx.QueryerContext, in ContractDocumentInput) (map[string]string, error) {
	values := map[string]string{}

	var employer employerIdentity
	// org_settings is owned by the invoicing module but is the organisation's one
	// legal identity; reading it here avoids a second, driftable copy.
	err := sqlx.GetContext(ctx, q, &employer,
		`SELECT legal_name, brand_name, address, city, phone, email, nif, stat, rcs, currency FROM org_settings WHERE id = 1`)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		values["employer.legal_name"] = employer.LegalName
		values["employer.brand_name"] = stringOrEmpty(employer.BrandName)
		values["employer.address"] = stringOrEmpty(employer.Address)
		values["employer.city"] = stringOrEmpty(employer.City)
		values["employer.phone"] = stringOrEmpty(employer.Phone)
		values["employer.email"] = stringOrEmpty(employer.Email)
		values["employer.nif"] = stringOrEmpty(employer.NIF)
		values["employer.stat"] = stringOrEmpty(employer.STAT)
		values["employer.rcs"] = stringOrEmpty(employer.RCS)
		values["employer.currency"] = employer.Currency
	}

	// Party: an employee's own record wins; otherwise the free-text party.
	values["party.full_name"] = strings.TrimSpace(in.PartyName)
	values["party.address"] = strings.TrimSpace(in.PartyAddress)
	values["party.id_number"] = strings.TrimSpace(in.PartyIDNumber)
	values["party.phone"] = strings.TrimSpace(in.PartyPhone)
	values["party.email"] = strings.TrimSpace(in.PartyEmail)

	if in.EmployeeID != nil {
		var employee employeeFacts
		err := sqlx.GetContext(ctx, q, &employee, `SELECT e.employee_number,
			CONCAT(e.first_name, ' ', e.last_name) AS full_name, e.address, e.phone,
			COALESCE(e.personal_email, e.work_email) AS email, e.hire_date,
			(SELECT p.title FROM hr_positions p WHERE p.id = e.position_id) AS position_title,
			(SELECT d.name FROM hr_departments d WHERE d.id = e.department_id) AS department_name,
			(SELECT CONCAT(m.first_name, ' ', m.last_name) FROM hr_employees m WHERE m.id = e.manager_id) AS manager_name
			FROM hr_employees e WHERE e.id = ?`, *in.EmployeeID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if values["party.full_name"] == "" {
			values["party.full_name"] = employee.FullName
		}
		if values["party.address"] == "" {
			values["party.address"] = stringOrEmpty(employee.Address)
		}
		if values["party.phone"] == "" {
			values["party.phone"] = stringOrEmpty(employee.Phone)
		}
		if values["party.email"] == "" {
			values["party.email"] = stringOrEmpty(employee.Email)
		}
		values["employee.employee_number"] = employee.EmployeeNumber
		values["employee.position_title"] = stringOrEmpty(employee.PositionTitle)
		values["employee.department_name"] = stringOrEmpty(employee.DepartmentName)
		values["employee.manager_name"] = stringOrEmpty(employee.ManagerName)
		values["employee.hire_date"] = frenchDateString(employee.HireDate)
	}

	if in.ContractID != nil {
		var contract contractFacts
		err := sqlx.GetContext(ctx, q, &contract,
			`SELECT contract_type, start_date, end_date, probation_end_date, salary, currency, pay_frequency
			 FROM hr_contracts WHERE id = ?`, *in.ContractID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		values["contract.type"] = contract.ContractType
		values["contract.start_date"] = frenchDateString(contract.StartDate)
		values["contract.end_date"] = frenchDateString(contract.EndDate)
		values["contract.probation_end_date"] = frenchDateString(contract.ProbationEndDate)
		values["contract.salary"] = frenchAmount(contract.Salary)
		values["contract.currency"] = stringOrEmpty(contract.Currency)
		values["contract.pay_frequency"] = stringOrEmpty(contract.PayFrequency)
	}

	issue := time.Now()
	if parsed, err := time.Parse("2006-01-02", strings.TrimSpace(in.IssueDate)); err == nil {
		issue = parsed
	}
	values["doc.issue_date"] = frenchDate(issue)
	values["doc.place"] = strings.TrimSpace(in.Place)
	// The reference only exists once issued; a draft preview shows a placeholder
	// rather than pretending to have a number it has not reserved.
	values["doc.reference"] = ""

	return values, nil
}

// renderDocument resolves tokens and renders the body. Every catalog token is
// considered known even when its value is empty, so an optional fact (no RCS, no
// end date) renders blank instead of blocking the document.
func (s *Service) renderDocument(ctx context.Context, q sqlx.QueryerContext, kind, body string, in ContractDocumentInput, reference string) (ContractDocumentRender, map[string]string, error) {
	resolved, err := s.resolveTokens(ctx, q, in)
	if err != nil {
		return ContractDocumentRender{}, nil, err
	}
	if reference != "" {
		resolved["doc.reference"] = reference
	}
	index := tokenIndex()
	result := renderTemplate(body,
		func(token string) (string, bool) {
			if _, known := index[token]; !known {
				return "", false
			}
			return resolved[token], true
		},
		in.Values,
		nil,
	)
	return ContractDocumentRender{
		Rendered: result.Rendered,
		Blanks:   result.Blanks,
		Unknown:  result.Unknown,
		Unfilled: result.Unfilled,
	}, resolved, nil
}

type templateRow struct {
	ID      int64  `db:"id"`
	Kind    string `db:"kind"`
	Code    string `db:"code"`
	Name    string `db:"name"`
	Body    string `db:"body"`
	Version int    `db:"version"`
}

func loadTemplate(ctx context.Context, q sqlx.QueryerContext, id int64) (templateRow, error) {
	var tpl templateRow
	err := sqlx.GetContext(ctx, q, &tpl,
		`SELECT id, kind, code, name, body, version FROM hr_contract_templates WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return tpl, ErrNotFound
	}
	return tpl, err
}

// PreviewContractDocument renders a would-be document without persisting it, so
// the console can show the real text while the blanks are being filled in.
func (s *Service) PreviewContractDocument(ctx context.Context, in ContractDocumentInput) (ContractDocumentRender, error) {
	tpl, err := loadTemplate(ctx, s.repo.db, in.TemplateID)
	if err != nil {
		return ContractDocumentRender{}, err
	}
	render, _, err := s.renderDocument(ctx, s.repo.db, tpl.Kind, tpl.Body, in, "")
	return render, err
}

func validateDocumentInput(in ContractDocumentInput) map[string]string {
	problems := map[string]string{}
	if in.TemplateID <= 0 {
		problems["template_id"] = "is required"
	}
	if in.EmployeeID == nil && strings.TrimSpace(in.PartyName) == "" {
		problems["party_name"] = "is required for an external party"
	}
	return problems
}

// CreateContractDocument stores a draft. The body is rendered immediately so the
// draft is previewable, but it is only frozen at issue time.
func (s *Service) CreateContractDocument(ctx context.Context, in ContractDocumentInput) (map[string]any, error) {
	if problems := validateDocumentInput(in); len(problems) > 0 {
		return nil, validationError(problems)
	}
	var id int64
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		tpl, err := loadTemplate(ctx, tx, in.TemplateID)
		if err != nil {
			return err
		}
		render, resolved, err := s.renderDocument(ctx, tx, tpl.Kind, tpl.Body, in, "")
		if err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{"resolved": resolved, "custom": in.Values})
		if err != nil {
			return err
		}
		partyName := strings.TrimSpace(in.PartyName)
		if partyName == "" {
			partyName = resolved["party.full_name"]
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO hr_contract_documents
			(kind, status, template_id, template_code, template_name, template_version,
			 employee_id, contract_id, party_name, party_address, party_id_number, party_phone, party_email,
			 place, issue_date, token_values, body_rendered)
			VALUES (?, 'draft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tpl.Kind, tpl.ID, tpl.Code, tpl.Name, tpl.Version,
			in.EmployeeID, in.ContractID, partyName,
			nullableTrimmed(in.PartyAddress), nullableTrimmed(in.PartyIDNumber),
			nullableTrimmed(in.PartyPhone), nullableTrimmed(in.PartyEmail),
			nullableTrimmed(in.Place), nullableTrimmed(in.IssueDate),
			string(payload), render.Rendered)
		if err != nil {
			return mapDBError(err)
		}
		id, err = result.LastInsertId()
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.ContractDocument(ctx, id)
}

// UpdateContractDocument re-renders a draft. Issued documents are immutable.
func (s *Service) UpdateContractDocument(ctx context.Context, id int64, in ContractDocumentInput) (map[string]any, error) {
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var current struct {
			Status     string        `db:"status"`
			TemplateID sql.NullInt64 `db:"template_id"`
		}
		if err := tx.GetContext(ctx, &current, `SELECT status, template_id FROM hr_contract_documents WHERE id = ? FOR UPDATE`, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if current.Status != "draft" {
			return ErrInvalidTransition
		}
		if in.TemplateID <= 0 && current.TemplateID.Valid {
			in.TemplateID = current.TemplateID.Int64
		}
		tpl, err := loadTemplate(ctx, tx, in.TemplateID)
		if err != nil {
			return err
		}
		render, resolved, err := s.renderDocument(ctx, tx, tpl.Kind, tpl.Body, in, "")
		if err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{"resolved": resolved, "custom": in.Values})
		if err != nil {
			return err
		}
		partyName := strings.TrimSpace(in.PartyName)
		if partyName == "" {
			partyName = resolved["party.full_name"]
		}
		_, err = tx.ExecContext(ctx, `UPDATE hr_contract_documents SET
			kind = ?, template_id = ?, template_code = ?, template_name = ?, template_version = ?,
			employee_id = ?, contract_id = ?, party_name = ?, party_address = ?, party_id_number = ?,
			party_phone = ?, party_email = ?, place = ?, issue_date = ?, token_values = ?, body_rendered = ?
			WHERE id = ?`,
			tpl.Kind, tpl.ID, tpl.Code, tpl.Name, tpl.Version,
			in.EmployeeID, in.ContractID, partyName,
			nullableTrimmed(in.PartyAddress), nullableTrimmed(in.PartyIDNumber),
			nullableTrimmed(in.PartyPhone), nullableTrimmed(in.PartyEmail),
			nullableTrimmed(in.Place), nullableTrimmed(in.IssueDate),
			string(payload), render.Rendered, id)
		return mapDBError(err)
	})
	if err != nil {
		return nil, err
	}
	return s.ContractDocument(ctx, id)
}

// nextContractNumberTx mirrors invoice_sequences: the LAST_INSERT_ID row lock is
// held to end-of-tx, so concurrent issues receive distinct, gapless numbers.
func nextContractNumberTx(ctx context.Context, tx *sqlx.Tx, kind string, year int) (int, error) {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO hr_contract_sequences (kind, year, last_number)
		 VALUES (?, ?, LAST_INSERT_ID(1))
		 ON DUPLICATE KEY UPDATE last_number = LAST_INSERT_ID(last_number + 1)`, kind, year); err != nil {
		return 0, err
	}
	var n int
	if err := tx.GetContext(ctx, &n, `SELECT LAST_INSERT_ID()`); err != nil {
		return 0, err
	}
	return n, nil
}

func contractReferencePrefix(kind string) string {
	if kind == "memo_deal" {
		return "MEMO"
	}
	return "CTR"
}

// IssueContractDocument freezes the draft: it reserves a reference, renders one
// final time with that reference in place, and stores the result. From here the
// stored text is the document — the template is never consulted again.
func (s *Service) IssueContractDocument(ctx context.Context, id int64) (map[string]any, error) {
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var row struct {
			Status     string         `db:"status"`
			Kind       string         `db:"kind"`
			TemplateID sql.NullInt64  `db:"template_id"`
			EmployeeID sql.NullInt64  `db:"employee_id"`
			ContractID sql.NullInt64  `db:"contract_id"`
			PartyName  string         `db:"party_name"`
			PartyAddr  sql.NullString `db:"party_address"`
			PartyID    sql.NullString `db:"party_id_number"`
			PartyPhone sql.NullString `db:"party_phone"`
			PartyEmail sql.NullString `db:"party_email"`
			Place      sql.NullString `db:"place"`
			IssueDate  sql.NullString `db:"issue_date"`
			Tokens     sql.NullString `db:"token_values"`
		}
		if err := tx.GetContext(ctx, &row, `SELECT status, kind, template_id, employee_id, contract_id,
			party_name, party_address, party_id_number, party_phone, party_email, place, issue_date, token_values
			FROM hr_contract_documents WHERE id = ? FOR UPDATE`, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if row.Status != "draft" {
			return ErrInvalidTransition
		}
		if !row.TemplateID.Valid {
			return validationError(map[string]string{"template_id": "the source template no longer exists"})
		}
		tpl, err := loadTemplate(ctx, tx, row.TemplateID.Int64)
		if err != nil {
			return err
		}

		in := ContractDocumentInput{
			TemplateID:    tpl.ID,
			PartyName:     row.PartyName,
			PartyAddress:  stringOrEmpty(row.PartyAddr),
			PartyIDNumber: stringOrEmpty(row.PartyID),
			PartyPhone:    stringOrEmpty(row.PartyPhone),
			PartyEmail:    stringOrEmpty(row.PartyEmail),
			Place:         stringOrEmpty(row.Place),
			IssueDate:     stringOrEmpty(row.IssueDate),
			Values:        map[string]string{},
		}
		if row.EmployeeID.Valid {
			in.EmployeeID = &row.EmployeeID.Int64
		}
		if row.ContractID.Valid {
			in.ContractID = &row.ContractID.Int64
		}
		if row.Tokens.Valid && row.Tokens.String != "" {
			var stored struct {
				Custom map[string]string `json:"custom"`
			}
			if err := json.Unmarshal([]byte(row.Tokens.String), &stored); err == nil && stored.Custom != nil {
				in.Values = stored.Custom
			}
		}

		issueDate := time.Now()
		if parsed, err := time.Parse("2006-01-02", in.IssueDate); err == nil {
			issueDate = parsed
		}
		number, err := nextContractNumberTx(ctx, tx, row.Kind, issueDate.Year())
		if err != nil {
			return err
		}
		reference := fmt.Sprintf("%s-%d-%04d", contractReferencePrefix(row.Kind), issueDate.Year(), number)

		render, resolved, err := s.renderDocument(ctx, tx, tpl.Kind, tpl.Body, in, reference)
		if err != nil {
			return err
		}
		// A document must never go out with holes in it.
		if len(render.Unfilled) > 0 {
			return validationError(map[string]string{"values": "fill every blank before issuing: " + strings.Join(render.Unfilled, ", ")})
		}
		if len(render.Unknown) > 0 {
			return validationError(map[string]string{"body": "the template has unknown placeholders: " + strings.Join(render.Unknown, ", ")})
		}
		payload, err := json.Marshal(map[string]any{"resolved": resolved, "custom": in.Values})
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE hr_contract_documents
			SET status = 'issued', reference = ?, issue_date = ?, body_rendered = ?, token_values = ?, issued_at = NOW()
			WHERE id = ?`, reference, issueDate.Format("2006-01-02"), render.Rendered, string(payload), id)
		return mapDBError(err)
	})
	if err != nil {
		return nil, err
	}
	return s.ContractDocument(ctx, id)
}

// setContractDocumentStatus handles sign and void, the only post-issue moves.
func (s *Service) setContractDocumentStatus(ctx context.Context, id int64, target, reason string) (map[string]any, error) {
	err := s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var status string
		if err := tx.GetContext(ctx, &status, `SELECT status FROM hr_contract_documents WHERE id = ? FOR UPDATE`, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		switch target {
		case "signed":
			if status != "issued" {
				return ErrInvalidTransition
			}
			_, err := tx.ExecContext(ctx, `UPDATE hr_contract_documents SET status = 'signed', signed_at = NOW() WHERE id = ?`, id)
			return err
		case "void":
			// An issued document is voided, never deleted or rewritten.
			if status != "issued" && status != "signed" {
				return ErrInvalidTransition
			}
			_, err := tx.ExecContext(ctx, `UPDATE hr_contract_documents SET status = 'void', voided_at = NOW(), void_reason = ? WHERE id = ?`,
				nullableTrimmed(reason), id)
			return err
		default:
			return ErrValidation
		}
	})
	if err != nil {
		return nil, err
	}
	return s.ContractDocument(ctx, id)
}

func (s *Service) SignContractDocument(ctx context.Context, id int64) (map[string]any, error) {
	return s.setContractDocumentStatus(ctx, id, "signed", "")
}

func (s *Service) VoidContractDocument(ctx context.Context, id int64, reason string) (map[string]any, error) {
	return s.setContractDocumentStatus(ctx, id, "void", reason)
}

// DeleteContractDocument only ever removes a draft.
func (s *Service) DeleteContractDocument(ctx context.Context, id int64) error {
	return s.repo.InTx(ctx, func(tx *sqlx.Tx) error {
		var status string
		if err := tx.GetContext(ctx, &status, `SELECT status FROM hr_contract_documents WHERE id = ? FOR UPDATE`, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status != "draft" {
			return ErrInvalidTransition
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM hr_contract_documents WHERE id = ?`, id)
		return err
	})
}
