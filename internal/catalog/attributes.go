package catalog

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx/types"
)

const facetValueMax = 128

// ProblemError carries field-level validation problems from the service up to the
// handler, which renders them as a 422 with per-field messages. Used for attribute
// validation, which depends on the category's template and so can't live in the
// stateless request validators.
type ProblemError struct {
	Problems map[string]string
}

func (e *ProblemError) Error() string { return "validation failed" }

// Facet is one denormalised, filterable value for the product_facets index.
type Facet struct {
	Key   string
	Value string
}

// normalizeAttributes validates a raw attributes JSON against template fields:
// required checks, type coercion, and select-option membership. It keeps only
// known keys (dropping anything not in the template) and returns the cleaned JSON
// plus a map of "attributes.<key>" -> problem message.
func normalizeAttributes(fields []TemplateField, raw types.JSONText) (types.JSONText, map[string]string) {
	problems := map[string]string{}

	in := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &in); err != nil {
			problems["attributes"] = "must be a JSON object"
			return types.JSONText("{}"), problems
		}
	}

	out := map[string]any{}
	for _, f := range fields {
		val, present := in[f.Key]
		if !present || val == nil {
			if f.Required {
				problems["attributes."+f.Key] = "is required"
			}
			continue
		}
		clean, problem := coerceField(f, val)
		if problem != "" {
			problems["attributes."+f.Key] = problem
			continue
		}
		if clean == nil { // e.g. blank text -> treat as absent
			if f.Required {
				problems["attributes."+f.Key] = "is required"
			}
			continue
		}
		out[f.Key] = clean
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		return types.JSONText("{}"), problems
	}
	return types.JSONText(encoded), problems
}

// coerceField returns the cleaned value (nil = treat as absent) and a problem
// message ("" = ok) for one field value.
func coerceField(f TemplateField, val any) (any, string) {
	switch f.Type {
	case FieldNumber:
		n, ok := toNumber(val)
		if !ok {
			return nil, "must be a number"
		}
		return n, ""
	case FieldBool:
		b, ok := val.(bool)
		if !ok {
			return nil, "must be true or false"
		}
		return b, ""
	case FieldSelect:
		s, ok := val.(string)
		if !ok || !contains(f.Options, s) {
			return nil, "must be one of: " + strings.Join(f.Options, ", ")
		}
		return s, ""
	default: // text
		s, ok := val.(string)
		if !ok {
			return nil, "must be text"
		}
		if s = strings.TrimSpace(s); s == "" {
			return nil, ""
		}
		return s, ""
	}
}

// templateFacets returns the filterable facet rows from a cleaned attributes JSON
// for the given template fields (product fields or variant axes).
func templateFacets(fields []TemplateField, attrs types.JSONText) []Facet {
	if len(attrs) == 0 || string(attrs) == "null" {
		return nil
	}
	m := map[string]any{}
	if err := json.Unmarshal(attrs, &m); err != nil {
		return nil
	}
	var out []Facet
	for _, f := range fields {
		if !f.Filterable {
			continue
		}
		val, ok := m[f.Key]
		if !ok || val == nil {
			continue
		}
		s := facetValue(val)
		if s != "" {
			out = append(out, Facet{Key: f.Key, Value: s})
		}
	}
	return out
}

func facetValue(val any) string {
	var s string
	switch v := val.(type) {
	case bool:
		if v {
			s = "true"
		} else {
			s = "false"
		}
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		s = strings.TrimSpace(v)
	default:
		return ""
	}
	if len(s) > facetValueMax {
		s = s[:facetValueMax]
	}
	return s
}

func toNumber(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	}
	return 0, false
}

func contains(opts []string, s string) bool {
	for _, o := range opts {
		if o == s {
			return true
		}
	}
	return false
}
