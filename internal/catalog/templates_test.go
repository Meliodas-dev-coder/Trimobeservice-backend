package catalog

import "testing"

// A category's department is not stored — it is resolved from its template_key
// through this registry, and both the public storefront and the admin back
// office filter /tech, /fashion and /coffee that way (handler.go passes
// TemplateKeysForDepartment into the category and product queries).
//
// The failure mode this guards is quiet: a template that is missing, or that
// names a department nobody serves, does not error. Its categories simply
// vanish from the department pages while still showing in the unfiltered shop,
// which reads as "the data is wrong" rather than "the registry is wrong".
func TestEveryTemplateBelongsToAServedDepartment(t *testing.T) {
	served := map[string][]string{}
	for _, d := range Departments() {
		served[d.Key] = TemplateKeysForDepartment(d.Key)
	}

	for _, tmpl := range Templates() {
		if !IsValidDepartment(tmpl.Department) {
			t.Errorf("template %q names department %q, which no department page serves", tmpl.Key, tmpl.Department)
			continue
		}
		found := false
		for _, key := range served[tmpl.Department] {
			if key == tmpl.Key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("template %q is not returned by TemplateKeysForDepartment(%q), so its categories would disappear from that department", tmpl.Key, tmpl.Department)
		}
	}
}

// The catalog seeds in seeds/*.sql write these template keys straight into
// product_categories.template_key. A key the registry does not know is dropped
// from every department view, so pin the ones the seeded categories rely on.
func TestSeededCategoryTemplatesAreRegistered(t *testing.T) {
	for _, key := range []string{"accessory", "audio", "phone", "screen_protector", "coffee"} {
		if !IsValidTemplateKey(key) {
			t.Errorf("template %q is used by a seeded category but is not registered", key)
		}
	}
}
