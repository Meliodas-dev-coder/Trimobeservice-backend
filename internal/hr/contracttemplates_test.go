package hr

import (
	"database/sql"
	"os"
	"strings"
	"testing"
)

// Every non-custom token used by the seeded starter bodies must exist in the
// catalog, otherwise it would render as a literal {{placeholder}} on a contract.
func TestSeededBodiesOnlyUseKnownTokens(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/000033_hr_contract_templates.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Drop SQL comment lines; the header prose mentions {{namespace.key}}.
	var stripped strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			stripped.WriteString(line + "\n")
		}
	}

	index := tokenIndex()
	seen := map[string]bool{}
	for _, m := range tokenPattern.FindAllStringSubmatch(stripped.String(), -1) {
		ns, key := m[1], m[2]
		if ns == customNamespace {
			continue
		}
		token := ns + "." + key
		if seen[token] {
			continue
		}
		seen[token] = true
		if _, ok := index[token]; !ok {
			t.Errorf("seeded body uses unknown token %s", token)
		}
	}
	t.Logf("distinct catalog tokens used by seeds: %d", len(seen))
}

// An employee must see contracts addressed to them, without that opening any
// door to writing one — or to reading a colleague's or an external party's.
func TestContractDocumentScoping(t *testing.T) {
	if got := resourceEmployeeColumn(contractDocumentsResource); got != "employee_id" {
		t.Errorf("documents must be employee-scoped for self-service, got %q", got)
	}
	if resourceRequiresAllScopeAccess(contractDocumentsResource) {
		t.Error("requiring all scope to read would hide an employee's own contract")
	}
	if !resourceMutationRequiresAllScope(contractDocumentsResource) {
		t.Error("drafting/issuing/voiding must stay with all-scope HR")
	}
	// Templates stay ownerless configuration, unaffected by the above.
	if !resourceRequiresAllScopeAccess("contract-templates") {
		t.Error("templates are configuration and must require all scope")
	}
	if resourceFeature(contractDocumentsResource) != "contracts" {
		t.Error("documents should ride the contracts feature employees already hold")
	}
	if !isSensitiveReadResource(contractDocumentsResource) {
		t.Error("contract reads must be audited")
	}
}

func TestFrenchDocumentFormatting(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"2026-07-21", "21 juillet 2026"},
		{"2026-03-01", "1er mars 2026"}, // French uses the ordinal for the 1st
		{"", ""},
	} {
		if got := frenchDateString(sql.NullString{String: tc.in, Valid: tc.in != ""}); got != tc.want {
			t.Errorf("frenchDateString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Thousands are separated by a non-breaking space, as French typography
	// requires, so the number never wraps across a line in the printed contract.
	for _, tc := range []struct{ in, want string }{
		{"1500000.00", "1\u00a0500\u00a0000"},
		{"250.50", "250,50"},
		{"999.00", "999"},
		{"", ""},
	} {
		if got := frenchAmount(sql.NullString{String: tc.in, Valid: tc.in != ""}); got != tc.want {
			t.Errorf("frenchAmount(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if contractReferencePrefix("memo_deal") != "MEMO" || contractReferencePrefix("employment") != "CTR" {
		t.Error("unexpected reference prefix")
	}
}

// An empty optional fact (no RCS, no end date on a permanent contract) must
// still render, while an unfilled blank must be reported so issuing can refuse.
func TestRenderTemplateSeparatesEmptyFactsFromUnfilledBlanks(t *testing.T) {
	resolved := map[string]string{"employer.rcs": "", "party.full_name": "Rakoto"}
	result := renderTemplate(
		"RCS {{employer.rcs}} pour {{party.full_name}}, préavis {{custom.notice_period}}.",
		func(token string) (string, bool) { value, ok := resolved[token]; return value, ok },
		nil,
		nil,
	)
	if len(result.Unknown) != 0 {
		t.Errorf("empty-but-known token must not be unknown: %v", result.Unknown)
	}
	if len(result.Unfilled) != 1 || result.Unfilled[0] != "notice_period" {
		t.Errorf("unfilled = %v, want [notice_period]", result.Unfilled)
	}
	if !strings.Contains(result.Rendered, "RCS  pour Rakoto") {
		t.Errorf("empty fact should render blank: %q", result.Rendered)
	}
}

func TestAnalyzeTemplate(t *testing.T) {
	body := "Poste {{employee.position_title}} à {{custom.work_place}} pour {{party.full_name}}. {{bogus.key}}"

	got := AnalyzeTemplate("employment", body, nil)
	if len(got.Blanks) != 1 || got.Blanks[0] != "work_place" {
		t.Errorf("blanks = %v, want [work_place]", got.Blanks)
	}
	if len(got.Unknown) != 1 || got.Unknown[0] != "bogus.key" {
		t.Errorf("unknown = %v, want [bogus.key]", got.Unknown)
	}
	if len(got.Conditional) != 0 {
		t.Errorf("employment should have no conditional tokens, got %v", got.Conditional)
	}
	if strings.Contains(got.Rendered, "{{employee.position_title}}") {
		t.Error("known token was not substituted")
	}
	if !strings.Contains(got.Rendered, "{{bogus.key}}") {
		t.Error("unknown token should be left visible in the preview")
	}

	// A memo may be signed with a non-employee, so employee.* is flagged.
	memo := AnalyzeTemplate("memo_deal", body, nil)
	if len(memo.Conditional) != 1 || memo.Conditional[0] != "employee.position_title" {
		t.Errorf("conditional = %v, want [employee.position_title]", memo.Conditional)
	}

	// Filled blanks replace the placeholder marker.
	filled := AnalyzeTemplate("employment", body, map[string]string{"work_place": "Antananarivo"})
	if !strings.Contains(filled.Rendered, "Antananarivo") {
		t.Errorf("filled blank not applied: %s", filled.Rendered)
	}
}
