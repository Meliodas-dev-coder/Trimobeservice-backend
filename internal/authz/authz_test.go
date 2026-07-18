package authz

import "testing"

func TestIsValidKey(t *testing.T) {
	if !IsValidKey(PermMobility) {
		t.Fatalf("expected %q to be valid", PermMobility)
	}
	if IsValidKey("nonexistent-section") {
		t.Fatal("expected an unknown key to be invalid")
	}
}

func TestNewPermissionSetDropsUnknownKeys(t *testing.T) {
	set := NewPermissionSet([]string{PermOrders, "bogus", PermEvents})
	if len(set) != 2 {
		t.Fatalf("expected 2 valid keys, got %d", len(set))
	}
	if !set.Has(PermOrders) || !set.Has(PermEvents) {
		t.Fatal("expected the two valid keys to be present")
	}
	if set.Has("bogus") {
		t.Fatal("unknown key must not be retained")
	}
}

func TestHasAny(t *testing.T) {
	set := NewPermissionSet([]string{PermMobility})

	if !set.HasAny(PermMobility) {
		t.Fatal("expected HasAny to match a held permission")
	}
	if !set.HasAny(PermOrders, PermMobility) {
		t.Fatal("expected HasAny to match when at least one key is held")
	}
	if set.HasAny(PermOrders, PermPayments) {
		t.Fatal("expected HasAny to be false when no key is held")
	}
	// No required keys means "no specific permission" → allowed.
	if !set.HasAny() {
		t.Fatal("expected HasAny() with no args to be true")
	}
}

func TestCatalogKeysAreUnique(t *testing.T) {
	seen := map[string]struct{}{}
	for _, p := range Catalog {
		if p.Key == "" {
			t.Fatal("catalog entry has an empty key")
		}
		if _, dup := seen[p.Key]; dup {
			t.Fatalf("duplicate catalog key %q", p.Key)
		}
		seen[p.Key] = struct{}{}
	}
}
