package crm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStandaloneOwnershipAndColleagueVisit(t *testing.T) {
	s, repo, u, c := fixture(t)
	before := s.Snapshot()
	manager := before.Users[2]
	if e := s.ChangeOwnership(context.Background(), u, c.ID, OwnershipInput{Ownership: "owned", OwnerID: "andrei"}); !errors.Is(e, ErrForbidden) {
		t.Fatal("employee assigned to colleague", e)
	}
	if e := s.ChangeOwnership(context.Background(), manager, c.ID, OwnershipInput{Ownership: "owned", OwnerID: "missing"}); !errors.Is(e, ErrInvalid) {
		t.Fatal("missing employee accepted", e)
	}
	repo.fail = true
	if e := s.ChangeOwnership(context.Background(), manager, c.ID, OwnershipInput{Ownership: "owned", OwnerID: "andrei"}); e == nil {
		t.Fatal("persistence failure ignored")
	}
	repo.fail = false
	if got, _ := s.Customer(u, c.ID); got.OwnerID != u.ID {
		t.Fatal("failed save changed owner")
	}
	if e := s.ChangeOwnership(context.Background(), manager, c.ID, OwnershipInput{Ownership: "owned", OwnerID: "andrei"}); e != nil {
		t.Fatal(e)
	}
	after := s.Snapshot()
	if len(after.Visits) != len(before.Visits) || !after.Customers[0].UpdatedAt.Equal(c.UpdatedAt) {
		t.Fatal("assignment invented an interaction")
	}
	if after.FollowUps[0].EmployeeID != before.FollowUps[0].EmployeeID {
		t.Fatal("assignment silently transferred tasks")
	}
	if e := s.RecordVisit(context.Background(), u, c.ID, VisitInput{Steps: []int{5}, Ownership: "keep", Notes: "Colleague visit"}); e != nil {
		t.Fatal(e)
	}
	got, _ := s.Customer(u, c.ID)
	if got.OwnerID != "andrei" {
		t.Fatal("visit stole colleague's customer")
	}
	if e := s.ChangeOwnership(context.Background(), u, c.ID, OwnershipInput{Ownership: "pool"}); !errors.Is(e, ErrForbidden) {
		t.Fatal("employee released colleague's customer")
	}
	if e := s.ChangeOwnership(context.Background(), manager, c.ID, OwnershipInput{Ownership: "pool"}); e != nil {
		t.Fatal(e)
	}
	if e := s.ChangeOwnership(context.Background(), u, c.ID, OwnershipInput{Ownership: "owned"}); e != nil {
		t.Fatal(e)
	}
	if len(s.Snapshot().Visits) != len(before.Visits)+1 {
		t.Fatal("ownership created extra visits")
	}
}
func TestFollowUpLifecycleAndLink(t *testing.T) {
	s, _, u, c := fixture(t)
	o := s.Snapshot().Opportunities[0]
	f, e := s.ScheduleFollowUp(context.Background(), u, FollowUpInput{CustomerID: c.ID, OpportunityID: o.ID, Type: "Prepare offer", Due: "2026-10-04"})
	if e != nil {
		t.Fatal(e)
	}
	if f.OpportunityID != o.ID || f.EmployeeID != u.ID {
		t.Fatal("missing relationship")
	}
	for _, status := range []string{"unreachable", "waiting", "open"} {
		if e = s.UpdateFollowUp(context.Background(), u, f.ID, FollowUpUpdate{Status: status, Due: "2026-10-05"}); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.UpdateFollowUp(context.Background(), u, f.ID, FollowUpUpdate{Status: "waiting", Due: "bad"}); !errors.Is(e, ErrInvalid) {
		t.Fatal("invalid reschedule accepted")
	}
	if e = s.UpdateFollowUp(context.Background(), u, f.ID, FollowUpUpdate{Status: "done"}); e != nil {
		t.Fatal(e)
	}
	after := s.Snapshot()
	last := after.FollowUps[len(after.FollowUps)-1]
	if last.CompletedAt == nil || last.Status != "done" || last.Due != "2026-10-05" {
		t.Fatal("completion lost data")
	}
	audits := len(after.Audit)
	_ = s.Complete(context.Background(), u, f.ID)
	if len(s.Snapshot().Audit) != audits {
		t.Fatal("duplicate completion event")
	}
	if e = s.UpdateFollowUp(context.Background(), u, f.ID, FollowUpUpdate{Status: "open", Due: "2026-10-06"}); !errors.Is(e, ErrConflict) {
		t.Fatal("completed task silently reopened")
	}
	other := s.Snapshot().Users[1]
	if _, e = s.ScheduleFollowUp(context.Background(), other, FollowUpInput{CustomerID: c.ID, OpportunityID: o.ID, Type: "Call", Due: "2026-10-05"}); !errors.Is(e, ErrForbidden) {
		t.Fatal("colleague modified opportunity next step")
	}
	if _, e = s.ScheduleFollowUp(context.Background(), u, FollowUpInput{CustomerID: s.Snapshot().Customers[1].ID, OpportunityID: o.ID, Type: "Call", Due: "2026-10-05"}); !errors.Is(e, ErrInvalid) {
		t.Fatal("mismatched opportunity/customer")
	}
	if _, e = s.ScheduleFollowUp(context.Background(), u, FollowUpInput{CustomerID: c.ID, EmployeeID: other.ID, Type: "Call", Due: "2026-10-05"}); !errors.Is(e, ErrForbidden) {
		t.Fatal("employee delegated task")
	}
}
func TestNewWorkflowStoreIsolation(t *testing.T) {
	s, _, u, c := fixture(t)
	u.StoreID = "other"
	u.Role = "manager"
	if e := s.ChangeOwnership(context.Background(), u, c.ID, OwnershipInput{Ownership: "pool"}); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if _, e := s.ScheduleFollowUp(context.Background(), u, FollowUpInput{CustomerID: c.ID, Type: "Call", Due: "2026-10-01"}); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if e := s.UpdateFollowUp(context.Background(), u, s.Snapshot().FollowUps[0].ID, FollowUpUpdate{Status: "waiting", Due: "2026-10-01"}); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	r, e := s.Report(u, "", "")
	if e != nil || len(r.Visits) != 0 || len(r.Events) != 0 || len(r.NewCustomers) != 0 {
		t.Fatal("report leaked store data")
	}
}
func TestReportDateBoundsAndHistoricalEvents(t *testing.T) {
	s, _, u, c := fixture(t)
	manager := s.Snapshot().Users[2]
	parse := func(v string) time.Time {
		x, e := time.Parse(time.RFC3339, v)
		if e != nil {
			t.Fatal(e)
		}
		return x
	}
	e := s.update(context.Background(), func(st *State) error {
		st.Visits = []Visit{{ID: "before", CustomerID: c.ID, EmployeeID: u.ID, At: parse("2026-09-27T20:59:59Z")}, {ID: "start", CustomerID: c.ID, EmployeeID: u.ID, At: parse("2026-09-27T21:00:00Z")}, {ID: "end", CustomerID: c.ID, EmployeeID: u.ID, At: parse("2026-09-28T20:59:59Z")}, {ID: "after", CustomerID: c.ID, EmployeeID: u.ID, At: parse("2026-09-28T21:00:00Z")}}
		st.Audit = []Audit{{ID: "offer", CustomerID: c.ID, ActorID: u.ID, Action: "opportunity.stage_changed", At: parse("2026-09-28T10:00:00Z"), Detail: "presentation → offer"}, {ID: "won", CustomerID: c.ID, ActorID: u.ID, Action: "opportunity.stage_changed", At: parse("2026-09-28T11:00:00Z"), Detail: "offer → won"}}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.Report(manager, "2026-09-28", "2026-09-28")
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Visits) != 2 || r.Visits[0].ID != "start" || r.Visits[1].ID != "end" {
		t.Fatalf("wrong local-day boundary: %+v", r.Visits)
	}
	if len(r.Events) != 2 || r.Events[0].Stage != "offer" || r.Events[1].Stage != "won" {
		t.Fatal("historical transitions missing")
	}
	if _, e = s.Report(u, "", ""); !errors.Is(e, ErrForbidden) {
		t.Fatal("employee report access")
	}
	for _, bounds := range [][2]string{{"2026-09-28", ""}, {"bad", "bad"}, {"2026-09-29", "2026-09-28"}} {
		if _, e = s.Report(manager, bounds[0], bounds[1]); !errors.Is(e, ErrInvalid) {
			t.Fatal("invalid range", bounds)
		}
	}
}
func TestVisitFollowUpLinkedAndRepeatedStageNoEvent(t *testing.T) {
	s, _, u, c := fixture(t)
	if e := s.RecordVisit(context.Background(), u, c.ID, VisitInput{Steps: []int{0}, Ownership: "keep", Product: "Internet", NextAction: "Call", Due: "2026-10-01"}); e != nil {
		t.Fatal(e)
	}
	st := s.Snapshot()
	o := st.Opportunities[len(st.Opportunities)-1]
	f := st.FollowUps[len(st.FollowUps)-1]
	if f.OpportunityID != o.ID {
		t.Fatal("visit did not link followup to new opportunity")
	}
	n := len(st.Audit)
	if e := s.MoveOpportunity(context.Background(), u, o.ID, "identified"); e != nil {
		t.Fatal(e)
	}
	if len(s.Snapshot().Audit) != n {
		t.Fatal("same-stage update fabricated event")
	}
}
