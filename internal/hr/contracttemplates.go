package hr

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/trimo/backend/internal/httpx"
)

// Contract/agreement template authoring.
//
// A template body is French prose carrying {{namespace.key}} placeholders. This
// file owns the placeholder vocabulary and the renderer. Both the preview here
// and the (later) issuing step go through renderTemplate, so what an author sees
// while writing is produced by the same code that will freeze the real document.
//
// Namespaces:
//   employer.* — the organisation, from org_settings (already the invoice seller)
//   party.*    — the other side, resolved from an employee or a free-text party
//   employee.* — extras that only exist when the party is an hr_employees row
//   contract.* — terms, only when the document is linked to an hr_contracts row
//   doc.*      — facts about the document itself
//   custom.*   — the fill-in-the-blank fields; any key is valid and the set of
//                blanks for a template is discovered by scanning its body

// tokenScope says when a token can be resolved, which is what makes one memo
// template usable for both an employee and an outside contractor.
type tokenScope string

const (
	scopeAlways   tokenScope = "always"   // resolvable for every document
	scopeEmployee tokenScope = "employee" // needs an employee-backed party
	scopeContract tokenScope = "contract" // needs a linked hr_contracts row
)

type tokenDef struct {
	Token  string     `json:"token"`
	Label  string     `json:"label"`
	Sample string     `json:"sample"`
	Scope  tokenScope `json:"scope"`
}

// contractTokens is the catalog offered by the editor's insert-token picker and
// used to validate a body. Samples are what preview substitutes, so a preview
// reads like a finished document rather than a form full of holes.
var contractTokens = []tokenDef{
	{"employer.legal_name", "Raison sociale de l’employeur", "Trimobe SARL", scopeAlways},
	{"employer.brand_name", "Nom commercial", "Trimobe", scopeAlways},
	{"employer.address", "Adresse de l’employeur", "Lot II M 45 Bis Antanimena", scopeAlways},
	{"employer.city", "Ville de l’employeur", "Antananarivo", scopeAlways},
	{"employer.phone", "Téléphone de l’employeur", "+261 34 00 000 00", scopeAlways},
	{"employer.email", "E-mail de l’employeur", "contact@trimobe.mg", scopeAlways},
	{"employer.nif", "NIF de l’employeur", "1234567890", scopeAlways},
	{"employer.stat", "Numéro statistique", "12345 11 2026 0 12345", scopeAlways},
	{"employer.rcs", "Registre du commerce", "2026 B 01234", scopeAlways},
	{"employer.currency", "Devise", "MGA", scopeAlways},

	{"party.full_name", "Nom de la partie", "Rakoto Andrianina", scopeAlways},
	{"party.address", "Adresse de la partie", "Lot IVB 12 Ambohipo, Antananarivo", scopeAlways},
	{"party.id_number", "Numéro de pièce d’identité", "101 234 567 890", scopeAlways},
	{"party.phone", "Téléphone de la partie", "+261 32 00 000 00", scopeAlways},
	{"party.email", "E-mail de la partie", "rakoto@example.mg", scopeAlways},

	{"employee.employee_number", "Matricule", "TRM-0042", scopeEmployee},
	{"employee.position_title", "Intitulé du poste", "Chef d’équipe", scopeEmployee},
	{"employee.department_name", "Département", "Opérations", scopeEmployee},
	{"employee.manager_name", "Responsable hiérarchique", "Ranaivo Hery", scopeEmployee},
	{"employee.hire_date", "Date d’embauche", "1 mars 2026", scopeEmployee},

	{"contract.type", "Type de contrat", "permanent", scopeContract},
	{"contract.start_date", "Date de début", "1 mars 2026", scopeContract},
	{"contract.end_date", "Date de fin", "28 février 2027", scopeContract},
	{"contract.probation_end_date", "Fin de période d’essai", "31 mai 2026", scopeContract},
	{"contract.salary", "Salaire", "1 500 000", scopeContract},
	{"contract.currency", "Devise du salaire", "MGA", scopeContract},
	{"contract.pay_frequency", "Périodicité de paie", "mensuelle", scopeContract},

	{"doc.reference", "Référence du document", "MEMO-2026-0001", scopeAlways},
	{"doc.issue_date", "Date d’établissement", "21 juillet 2026", scopeAlways},
	{"doc.place", "Lieu d’établissement", "Antananarivo", scopeAlways},
}

const customNamespace = "custom"

// tokenPattern deliberately tolerates inner spacing ({{ party.full_name }}) so a
// stray space while authoring does not silently ship an unreplaced placeholder.
var tokenPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][\w]*)\.([a-zA-Z_][\w]*)\s*\}\}`)

func tokenIndex() map[string]tokenDef {
	index := make(map[string]tokenDef, len(contractTokens))
	for _, def := range contractTokens {
		index[def.Token] = def
	}
	return index
}

// TemplateAnalysis is what the editor needs to tell an author whether a body is
// sound: the rendered preview, the blanks it will ask for at issue time, and the
// two failure modes worth surfacing before a document is ever produced.
type TemplateAnalysis struct {
	Rendered    string   `json:"rendered"`
	Blanks      []string `json:"blanks"`
	Unknown     []string `json:"unknown"`
	Conditional []string `json:"conditional"`
}

// renderResult is the outcome of walking a body once.
type renderResult struct {
	Rendered string
	Blanks   []string // every custom.* key the body asks for
	Unknown  []string // tokens absent from the catalog
	Unfilled []string // custom.* keys left empty — these block issuing
	Flagged  []string // catalog tokens the caller asked to be flagged
}

// renderTemplate is the single renderer. Authoring preview and real document
// rendering both go through it so what an author approves is what gets frozen;
// they differ only in the lookup they supply (catalog samples vs resolved
// record values) and in which catalog tokens they want flagged.
func renderTemplate(
	body string,
	lookup func(token string) (string, bool),
	custom map[string]string,
	flag func(token string) bool,
) renderResult {
	blanks := map[string]bool{}
	unknown := map[string]bool{}
	unfilled := map[string]bool{}
	flagged := map[string]bool{}

	rendered := tokenPattern.ReplaceAllStringFunc(body, func(match string) string {
		parts := tokenPattern.FindStringSubmatch(match)
		namespace, key := parts[1], parts[2]
		token := namespace + "." + key

		if namespace == customNamespace {
			blanks[key] = true
			if value := strings.TrimSpace(custom[key]); value != "" {
				return value
			}
			unfilled[key] = true
			return "«  " + key + "  »"
		}

		value, ok := lookup(token)
		if !ok {
			unknown[token] = true
			return match // stays visible rather than silently vanishing
		}
		if flag != nil && flag(token) {
			flagged[token] = true
		}
		return value
	})

	return renderResult{
		Rendered: rendered,
		Blanks:   sortedKeys(blanks),
		Unknown:  sortedKeys(unknown),
		Unfilled: sortedKeys(unfilled),
		Flagged:  sortedKeys(flagged),
	}
}

// AnalyzeTemplate renders body against the catalog samples and reports what the
// author needs to know. values overrides samples for the custom blanks so the
// same call powers a live preview as blanks get filled in.
func AnalyzeTemplate(kind, body string, values map[string]string) TemplateAnalysis {
	index := tokenIndex()
	result := renderTemplate(body,
		func(token string) (string, bool) {
			def, ok := index[token]
			return def.Sample, ok
		},
		values,
		// A memo may be signed with an outside party, so employee/contract data
		// is not guaranteed. Flag it rather than reject it: the author may know
		// this particular memo is always employee-backed.
		func(token string) bool {
			return kind == "memo_deal" && index[token].Scope != scopeAlways
		},
	)
	return TemplateAnalysis{
		Rendered:    result.Rendered,
		Blanks:      result.Blanks,
		Unknown:     result.Unknown,
		Conditional: result.Flagged,
	}
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// ContractTokenCatalog is served to the editor's token picker.
func ContractTokenCatalog() []tokenDef { return contractTokens }

// Both endpoints sit behind the Organization guard and additionally reuse the
// resource's own authorization, so template authoring follows exactly the same
// all-scope rule as creating a position.
func (h *Handler) contractTemplateTokens(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	if _, _, err := h.svc.AuthorizeInput(r.Context(), actor, "contract-templates", "view", map[string]any{}); err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"contract_tokens": ContractTokenCatalog()})
}

func (h *Handler) previewContractTemplate(w http.ResponseWriter, r *http.Request) {
	actor, ok := actorID(w, r)
	if !ok {
		return
	}
	var req struct {
		Kind   string            `json:"kind"`
		Body   string            `json:"body"`
		Values map[string]string `json:"values"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return
	}
	if _, _, err := h.svc.AuthorizeInput(r.Context(), actor, "contract-templates", "view", map[string]any{}); err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, httpx.Envelope{"preview": AnalyzeTemplate(req.Kind, req.Body, req.Values)})
}
