package stock

import (
	"reflect"
	"testing"
)

func TestScopeNarrowsToTheRequestedDepartment(t *testing.T) {
	allowed := []string{"tech", "coffee"}
	tests := []struct {
		name      string
		requested string
		want      []string
	}{
		{"no filter keeps the whole authorized scope", "", []string{"tech", "coffee"}},
		{"an authorized filter narrows to it", "coffee", []string{"coffee"}},
		// Matching nothing is the safe answer: silently widening to every
		// authorized department would leak the shelf the admin asked about.
		{"an unauthorized filter matches nothing", "fashion", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scope(allowed, tt.requested); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("scope(%v, %q) = %v, want %v", allowed, tt.requested, got, tt.want)
			}
		})
	}
}

func TestTemplateKeysForAnEmptyScopeIsEmpty(t *testing.T) {
	if keys := templateKeys(nil); len(keys) != 0 {
		t.Errorf("an empty department scope must yield no template keys, got %v", keys)
	}
}

func TestScopeClauseMatchesNothingWithoutTemplates(t *testing.T) {
	clause, args := scopeClause(Filter{}, nil)
	if clause != " WHERE 1 = 0" || len(args) != 0 {
		t.Errorf("clause = %q args = %v, want a match-nothing clause", clause, args)
	}
}

func TestAdjustReasonsExcludeOrderMovements(t *testing.T) {
	// Reservation and release are written by the orders module inside the
	// checkout transaction; letting an admin forge one by hand would put the
	// ledger out of step with the order it claims to explain.
	for _, reason := range []string{ReasonOrderReserve, ReasonOrderRelease, ReasonInitial} {
		if adjustReasons[reason] {
			t.Errorf("%q must not be an admin-writable reason", reason)
		}
	}
	for _, reason := range []string{ReasonRestock, ReasonManualAdjust, ReasonCorrection} {
		if !adjustReasons[reason] {
			t.Errorf("%q must be an admin-writable reason", reason)
		}
	}
}

func TestOrderMovementDirectionPicksTheReason(t *testing.T) {
	reserve := OrderMovement(7, "coffee", -3, 5, 42)
	if reserve.Reason != ReasonOrderReserve || reserve.Delta != -3 || reserve.QuantityAfter != 5 {
		t.Errorf("negative delta must record a reservation, got %+v", reserve)
	}
	if reserve.ReferenceType == nil || *reserve.ReferenceType != ReferenceOrder ||
		reserve.ReferenceID == nil || *reserve.ReferenceID != 42 {
		t.Errorf("movement must reference its order, got %+v", reserve)
	}
	if release := OrderMovement(7, "coffee", 3, 8, 42); release.Reason != ReasonOrderRelease {
		t.Errorf("positive delta must record a release, got %q", release.Reason)
	}
}
