package crm

import (
	"errors"
	"testing"

	"vodafone/store/internal/apperr"
)

func TestNormalizePhone(t *testing.T) {
	for _, in := range []string{"0722 345 678", "+40 722 345 678", "0040722345678", "(0722) 345-678", "40722345678", "0722.345.678"} {
		if got := NormalizePhone(in); got != "+40722345678" {
			t.Errorf("NormalizePhone(%q) = %q", in, got)
		}
	}
	if ValidPhone(NormalizePhone("12345")) {
		t.Error("short number accepted")
	}
}

func TestPhoneSearchFragment(t *testing.T) {
	cases := map[string]string{
		"0722 345 678": "40722345678",
		"+40722345678": "40722345678",
		"345 678":      "345678",
		"0722":         "40722",
		"Ioana":        "",
		"07":           "",
		"Ana 0722":     "",
	}
	for in, want := range cases {
		if got := PhoneSearchFragment(in); got != want {
			t.Errorf("PhoneSearchFragment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFoldName(t *testing.T) {
	if got := FoldName("  Ștefan   ȚUȚUIANU Mâță "); got != "stefan tutuianu mata" {
		t.Errorf("got %q", got)
	}
}

func TestIDs(t *testing.T) {
	id := NewID()
	if !ValidID(id) || ValidID("not-an-id") || ValidID(id+"0") {
		t.Fatal("id validation")
	}
	if id[14] != '4' {
		t.Fatal("not a v4 uuid")
	}
}

func TestValidateVisit(t *testing.T) {
	catalog := []CatalogItem{{Kind: "visit_reason", Code: "support"}, {Kind: "product_category", Code: "mobile"}}
	cases := []struct {
		name  string
		in    VisitInput
		field string
	}{
		{"no steps", VisitInput{}, "steps"},
		{"step out of range", VisitInput{Steps: []int{8}}, "steps"},
		{"duplicate step", VisitInput{Steps: []int{1, 1}}, "steps"},
		{"unknown reason", VisitInput{Steps: []int{0}, ReasonCode: "x"}, "reasonCode"},
		{"next action without date", VisitInput{Steps: []int{0}, NextAction: "Call", Due: "2026-02-30"}, "due"},
		{"bad ownership", VisitInput{Steps: []int{0}, Ownership: "mine"}, "ownership"},
		{"empty product", VisitInput{Steps: []int{0}, Opportunities: []OpportunityDraft{{Product: " "}}}, "opportunities.0.product"},
		{"unknown category", VisitInput{Steps: []int{0}, Opportunities: []OpportunityDraft{{Product: "X", Category: "tv"}}}, "opportunities.0.category"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateVisit(&c.in, catalog)
			var e *apperr.Error
			if !errors.As(err, &e) || e.Fields[c.field] == "" {
				t.Fatalf("want field error on %s, got %v", c.field, err)
			}
		})
	}
	ok := VisitInput{Steps: []int{6, 4}, ReasonCode: "support", Ownership: "keep", Opportunities: []OpportunityDraft{{Product: "Red", Category: "mobile"}}}
	if err := validateVisit(&ok, catalog); err != nil {
		t.Fatal(err)
	}
}

func TestRolePermissions(t *testing.T) {
	emp, mgr := User{ID: "e", Role: RoleEmployee}, User{ID: "m", Role: RoleManager}
	if emp.Can(PermViewReports) || emp.Can(PermAssignAnyone) || !mgr.Can(PermViewReports) {
		t.Fatal("role capabilities")
	}
	if !emp.canWork("e") || emp.canWork("x") || !mgr.canWork("x") {
		t.Fatal("work ownership")
	}
	if (User{Role: "unknown"}).Can(PermViewAudit) || ValidRole("admin") {
		t.Fatal("unknown role must have no permissions")
	}
}
