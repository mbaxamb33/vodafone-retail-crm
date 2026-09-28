package crm

import (
	"context"
	"errors"
	"os"
	"testing"
)

type memoryRepo struct {
	state State
	fail  bool
}

func (r *memoryRepo) Load(context.Context) (State, error) {
	if len(r.state.Users) == 0 {
		return State{}, os.ErrNotExist
	}
	return r.state, nil
}
func (r *memoryRepo) Save(_ context.Context, s State) error {
	if r.fail {
		return errors.New("disk failure")
	}
	r.state = s
	return nil
}
func fixture(t *testing.T) (*Service, *memoryRepo, User, Customer) {
	t.Helper()
	r := &memoryRepo{}
	s, e := New(context.Background(), r, true)
	if e != nil {
		t.Fatal(e)
	}
	return s, r, s.Snapshot().Users[0], s.Snapshot().Customers[0]
}
func TestNormalizePhone(t *testing.T) {
	for _, p := range []string{"0722 345 678", "+40 722 345 678", "0040722345678", "(0722) 345-678"} {
		if got := NormalizePhone(p); got != "+40722345678" {
			t.Errorf("%s → %s", p, got)
		}
	}
}
func TestVisitAtomicAndHistorical(t *testing.T) {
	s, r, u, c := fixture(t)
	before := s.Snapshot()
	in := VisitInput{Reason: "Support", Steps: []int{5, 6}, Notes: "Useful note", Ownership: "owned", NextAction: "Call", Due: "2026-10-01", Product: "Internet"}
	r.fail = true
	if e := s.RecordVisit(context.Background(), u, c.ID, in); e == nil {
		t.Fatal("expected failure")
	}
	if len(s.Snapshot().Visits) != len(before.Visits) {
		t.Fatal("failed transaction changed state")
	}
	r.fail = false
	if e := s.RecordVisit(context.Background(), u, c.ID, in); e != nil {
		t.Fatal(e)
	}
	after := s.Snapshot()
	if len(after.Visits) != len(before.Visits)+1 || len(after.FollowUps) != len(before.FollowUps)+1 || len(after.Opportunities) != len(before.Opportunities)+1 {
		t.Fatal("related records missing")
	}
	v := after.Visits[len(after.Visits)-1]
	if len(v.Steps) != 2 || v.Steps[0] != 5 {
		t.Fatal("invented earlier steps")
	}
	if after.Visits[0].Notes != before.Visits[0].Notes {
		t.Fatal("history overwritten")
	}
	if after.Opportunities[len(after.Opportunities)-1].Stage != "identified" {
		t.Fatal("visit stages leaked into opportunity")
	}
}
func TestOwnershipAndStoreBoundaries(t *testing.T) {
	s, _, u, c := fixture(t)
	in := VisitInput{Steps: []int{0}, Ownership: "owned"}
	other := u
	other.ID = "andrei"
	if e := s.RecordVisit(context.Background(), other, c.ID, in); !errors.Is(e, ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", e)
	}
	other.StoreID = "other-store"
	if _, e := s.Customer(other, c.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross-store read")
	}
	if e := s.RecordVisit(context.Background(), other, c.ID, in); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross-store mutation")
	}
	manager := s.Snapshot().Users[2]
	if e := s.RecordVisit(context.Background(), manager, c.ID, in); e != nil {
		t.Fatal(e)
	}
	if s.Snapshot().Customers[0].OwnerID != manager.ID {
		t.Fatal("manager assignment failed")
	}
	if len(s.Snapshot().Audit) < 2 {
		t.Fatal("missing audit")
	}
}
func TestVisitValidation(t *testing.T) {
	for _, in := range []VisitInput{{Ownership: "owned"}, {Steps: []int{8}, Ownership: "owned"}, {Steps: []int{0, 0}, Ownership: "owned"}, {Steps: []int{0}, Ownership: "bad"}, {Steps: []int{0}, Ownership: "owned", NextAction: "Call", Due: "bad"}} {
		s, _, u, c := fixture(t)
		if e := s.RecordVisit(context.Background(), u, c.ID, in); !errors.Is(e, ErrInvalid) {
			t.Errorf("expected invalid: %+v", in)
		}
	}
}
func TestFollowupAndOpportunityPermissions(t *testing.T) {
	s, _, u, _ := fixture(t)
	st := s.Snapshot()
	other := st.Users[1]
	if e := s.Complete(context.Background(), other, st.FollowUps[0].ID); !errors.Is(e, ErrForbidden) {
		t.Fatal("followup authorization failed")
	}
	if e := s.Complete(context.Background(), u, st.FollowUps[0].ID); e != nil {
		t.Fatal(e)
	}
	oid := st.Opportunities[0].ID
	if e := s.MoveOpportunity(context.Background(), other, oid, "won"); !errors.Is(e, ErrForbidden) {
		t.Fatal("opportunity authorization failed")
	}
	if e := s.MoveOpportunity(context.Background(), u, oid, "invalid"); !errors.Is(e, ErrInvalid) {
		t.Fatal("stage validation failed")
	}
	if e := s.MoveOpportunity(context.Background(), u, oid, "won"); e != nil {
		t.Fatal(e)
	}
	if e := s.MoveOpportunity(context.Background(), u, oid, "offer"); !errors.Is(e, ErrConflict) {
		t.Fatal("closed opportunity reopened")
	}
}
func TestFilePersistence(t *testing.T) {
	r := FileRepository{Path: t.TempDir() + "/store.json"}
	s, e := New(context.Background(), r, true)
	if e != nil {
		t.Fatal(e)
	}
	u := s.Snapshot().Users[0]
	c, e := s.CreateCustomer(context.Background(), u, "Test Customer", "0722345678")
	if e != nil {
		t.Fatal(e)
	}
	reloaded, e := New(context.Background(), r, false)
	if e != nil {
		t.Fatal(e)
	}
	got, e := reloaded.Customer(u, c.ID)
	if e != nil || got.Phone != "+40722345678" {
		t.Fatal("persistence failed")
	}
}
