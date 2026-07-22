package orders

import "testing"

func ptr(s string) *string { return &s }

func TestScopeCovers(t *testing.T) {
	coffee := Scope{Departments: []string{"coffee"}}
	tests := []struct {
		name       string
		scope      Scope
		department *string
		want       bool
	}{
		{"global sees everything", Scope{All: true}, ptr("tech"), true},
		{"global sees unattributed lines", Scope{All: true}, nil, true},
		{"department sees its own", coffee, ptr("coffee"), true},
		{"department never sees another", coffee, ptr("tech"), false},
		{"department never sees unattributed", coffee, nil, false},
		{"department never sees empty", coffee, ptr(""), false},
		{"no scope sees nothing", Scope{}, ptr("coffee"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.scope.Covers(tt.department); got != tt.want {
				t.Errorf("Covers(%v) = %v, want %v", tt.department, got, tt.want)
			}
		})
	}
}

func TestScopeCoversAll(t *testing.T) {
	coffee := Scope{Departments: []string{"coffee"}}
	coffeeOnly := []OrderItem{{Department: ptr("coffee")}, {Department: ptr("coffee")}}
	mixed := []OrderItem{{Department: ptr("coffee")}, {Department: ptr("tech")}}

	if !coffee.CoversAll(coffeeOnly) {
		t.Error("a coffee-only order must be movable by a coffee admin")
	}
	// Status is shared by every line, so a mixed order is off limits: advancing
	// it would move someone else's lines too.
	if coffee.CoversAll(mixed) {
		t.Error("a mixed order must not be movable by a department admin")
	}
	if !(Scope{All: true}).CoversAll(mixed) {
		t.Error("global access must cover a mixed order")
	}
	if coffee.CoversAll(nil) {
		t.Error("an order with no readable lines must not be movable")
	}
}

func TestApplyScopeTrimsToDepartmentLines(t *testing.T) {
	detail := &OrderDetail{
		Order: Order{ID: 1, Total: "150.00"},
		Items: []OrderItem{
			{Department: ptr("coffee"), LineTotal: "40.00", Quantity: 2},
			{Department: ptr("tech"), LineTotal: "110.00", Quantity: 1},
		},
	}
	scoped := applyScope(detail, Scope{Departments: []string{"coffee"}})
	if scoped == nil {
		t.Fatal("expected the coffee slice to be visible")
	}
	if len(scoped.Items) != 1 || *scoped.Items[0].Department != "coffee" {
		t.Fatalf("expected only the coffee line, got %+v", scoped.Items)
	}
	if scoped.DepartmentSubtotal == nil || *scoped.DepartmentSubtotal != "40.00" {
		t.Errorf("department subtotal = %v, want 40.00", scoped.DepartmentSubtotal)
	}
	if scoped.DepartmentQuantity == nil || *scoped.DepartmentQuantity != 2 {
		t.Errorf("department quantity = %v, want 2", scoped.DepartmentQuantity)
	}
	// The order total is what the customer actually pays; it is never rewritten.
	if scoped.Total != "150.00" {
		t.Errorf("order total = %s, want the untouched 150.00", scoped.Total)
	}
}

func TestApplyScopeHidesOrdersWithNothingVisible(t *testing.T) {
	detail := &OrderDetail{
		Order: Order{ID: 2, Total: "110.00"},
		Items: []OrderItem{{Department: ptr("tech"), LineTotal: "110.00", Quantity: 1}},
	}
	if applyScope(detail, Scope{Departments: []string{"coffee"}}) != nil {
		t.Error("an order with no coffee lines must not exist for a coffee admin")
	}
}

func TestApplyScopeLeavesGlobalReadsUntouched(t *testing.T) {
	detail := &OrderDetail{
		Order: Order{ID: 3},
		Items: []OrderItem{{Department: ptr("coffee")}, {Department: ptr("tech")}},
	}
	scoped := applyScope(detail, Scope{All: true})
	if len(scoped.Items) != 2 || scoped.DepartmentSubtotal != nil {
		t.Error("a global read must see the whole order with no department share")
	}
}
