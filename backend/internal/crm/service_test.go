package crm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"vodafone/store/internal/apperr"
	"vodafone/store/internal/crm"
	"vodafone/store/internal/postgres"
	"vodafone/store/internal/postgres/pgtest"
)

type env struct {
	db    *postgres.DB
	svc   *crm.Service
	st    pgtest.Store
	now   time.Time
	ctx   context.Context
	other pgtest.Store
}

// setup creates two stores; the second verifies isolation. The clock starts at a fixed instant.
func setup(t *testing.T) *env {
	t.Helper()
	db := pgtest.New(t)
	a := pgtest.FastAuth(db)
	e := &env{db: db, ctx: context.Background(), now: time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)}
	e.svc = crm.NewService(db, crm.WithClock(func() time.Time { return e.now }))
	e.st = pgtest.NewStore(t, db, a, "Store A")
	e.other = pgtest.NewStore(t, db, a, "Store B")
	return e
}

func (e *env) customer(t *testing.T, by crm.User, name, phone string) crm.Customer {
	t.Helper()
	c, err := e.svc.CreateCustomer(e.ctx, by, crm.CustomerInput{Name: name, Phone: phone})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func is(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestVisitIsAtomicAndHistorical(t *testing.T) {
	e := setup(t)
	c := e.customer(t, e.st.Ioana, "Maria Pop", "0722 111 222")

	// A failure part-way through must leave nothing behind.
	failing := crm.NewService(failAfterVisit{e.db}, crm.WithClock(func() time.Time { return e.now }))
	_, err := failing.RecordVisit(e.ctx, e.st.Ioana, c.ID, crm.VisitInput{Steps: []int{0}, Ownership: "owned", NextAction: "Call", Due: "2026-10-01"})
	if err == nil {
		t.Fatal("expected failure")
	}
	p, err := e.svc.CustomerProfile(e.ctx, e.st.Mgr, c.ID, 0)
	must(t, err)
	if p.Total != 0 || len(p.FollowUps) != 0 || p.Customer.Ownership != crm.OwnershipPool || p.Customer.LastInteractionAt != nil {
		t.Fatalf("partial visit persisted: %+v", p)
	}

	res, err := e.svc.RecordVisit(e.ctx, e.st.Ioana, c.ID, crm.VisitInput{ReasonCode: "renewal", Steps: []int{6, 5}, Notes: "Notă", Ownership: "owned", NextAction: "Call", Due: "2026-10-01",
		Opportunities: []crm.OpportunityDraft{{Product: "Red Unlimited", Category: "mobile"}}})
	must(t, err)
	if got := res.Visit.Steps; len(got) != 2 || got[0] != 5 || got[1] != 6 || res.Visit.FurthestStep != 6 {
		t.Fatalf("steps must be stored as selected, sorted, never inferred: %v", got)
	}
	if res.Visit.Reason != "Reînnoire abonament" {
		t.Fatalf("reason label: %q", res.Visit.Reason)
	}
	if res.Opportunities[0].Stage != crm.StageIdentified {
		t.Fatal("visit progress leaked into opportunity stage")
	}
	if res.FollowUp == nil || res.FollowUp.OpportunityID != res.Opportunities[0].ID {
		t.Fatal("single opportunity must be linked to the next action")
	}
	p, err = e.svc.CustomerProfile(e.ctx, e.st.Mgr, c.ID, 0)
	must(t, err)
	if p.Customer.OwnerID != e.st.Ioana.ID || p.Customer.LastInteractionAt == nil || p.LastVisit == nil || len(p.OwnershipHistory) != 1 {
		t.Fatalf("profile after visit: %+v", p)
	}

	// Two opportunities: the next action is not attached to either.
	res, err = e.svc.RecordVisit(e.ctx, e.st.Ioana, c.ID, crm.VisitInput{Steps: []int{0}, NextAction: "Call", Due: "2026-10-02",
		Opportunities: []crm.OpportunityDraft{{Product: "A"}, {Product: "B"}}})
	must(t, err)
	if res.FollowUp.OpportunityID != "" || len(res.Opportunities) != 2 {
		t.Fatal("ambiguous opportunity link")
	}
}

// failAfterVisit fails the follow-up insert to exercise rollback.
type failAfterVisit struct{ *postgres.DB }

func (f failAfterVisit) InTx(ctx context.Context, fn func(tx crm.Tx) error) error {
	return f.DB.InTx(ctx, func(tx crm.Tx) error { return fn(failingTx{tx}) })
}

type failingTx struct{ crm.Tx }

func (failingTx) InsertFollowUp(context.Context, crm.FollowUp) error { return errors.New("disk full") }

func TestOwnershipRules(t *testing.T) {
	e := setup(t)
	s := e.st
	cases := []struct {
		name  string
		actor crm.User
		start crm.OwnershipInput // applied by the manager first
		in    crm.OwnershipInput
		want  error
		owner string
	}{
		{"employee claims pool customer", s.Ioana, crm.OwnershipInput{Ownership: "pool"}, crm.OwnershipInput{Ownership: "owned"}, nil, s.Ioana.ID},
		{"employee cannot assign colleague", s.Ioana, crm.OwnershipInput{Ownership: "pool"}, crm.OwnershipInput{Ownership: "owned", OwnerID: s.Andrei.ID}, apperr.ErrForbidden, ""},
		{"employee cannot take colleague's customer", s.Ioana, crm.OwnershipInput{Ownership: "owned", OwnerID: s.Andrei.ID}, crm.OwnershipInput{Ownership: "owned"}, apperr.ErrForbidden, s.Andrei.ID},
		{"employee cannot release colleague's customer", s.Ioana, crm.OwnershipInput{Ownership: "owned", OwnerID: s.Andrei.ID}, crm.OwnershipInput{Ownership: "pool"}, apperr.ErrForbidden, s.Andrei.ID},
		{"employee releases own customer", s.Ioana, crm.OwnershipInput{Ownership: "owned", OwnerID: s.Ioana.ID}, crm.OwnershipInput{Ownership: "unassigned"}, nil, ""},
		{"manager reassigns", s.Mgr, crm.OwnershipInput{Ownership: "owned", OwnerID: s.Ioana.ID}, crm.OwnershipInput{Ownership: "owned", OwnerID: s.Andrei.ID}, nil, s.Andrei.ID},
		{"unknown employee", s.Mgr, crm.OwnershipInput{Ownership: "pool"}, crm.OwnershipInput{Ownership: "owned", OwnerID: crm.NewID()}, apperr.ErrEmployeeNotFound, ""},
		{"employee of another store", s.Mgr, crm.OwnershipInput{Ownership: "pool"}, crm.OwnershipInput{Ownership: "owned", OwnerID: e.other.Ioana.ID}, apperr.ErrEmployeeNotFound, ""},
		{"pool with owner", s.Mgr, crm.OwnershipInput{Ownership: "pool"}, crm.OwnershipInput{Ownership: "pool", OwnerID: s.Ioana.ID}, apperr.ErrValidation, ""},
		{"manager of another store", e.other.Mgr, crm.OwnershipInput{Ownership: "pool"}, crm.OwnershipInput{Ownership: "pool"}, apperr.ErrCustomerNotFound, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cust := e.customer(t, s.Mgr, "Client "+c.name[:4], "0722 000 999")
			_, err := e.svc.ChangeOwnership(e.ctx, s.Mgr, cust.ID, c.start)
			must(t, err)
			_, err = e.svc.ChangeOwnership(e.ctx, c.actor, cust.ID, c.in)
			if c.want == nil {
				must(t, err)
			} else {
				is(t, err, c.want)
			}
			got, err := e.svc.CustomerProfile(e.ctx, s.Mgr, cust.ID, 0)
			must(t, err)
			if got.Customer.OwnerID != c.owner {
				t.Fatalf("owner = %q, want %q", got.Customer.OwnerID, c.owner)
			}
		})
	}
}

func TestOwnershipChangeIsNotAnInteraction(t *testing.T) {
	e := setup(t)
	s := e.st
	c := e.customer(t, s.Ioana, "Ion Ionescu", "0722 000 100")
	_, err := e.svc.RecordVisit(e.ctx, s.Ioana, c.ID, crm.VisitInput{Steps: []int{0}, Ownership: "owned", NextAction: "Call", Due: "2026-10-01"})
	must(t, err)
	before, _ := e.svc.CustomerProfile(e.ctx, s.Mgr, c.ID, 0)
	e.now = e.now.Add(time.Hour)
	_, err = e.svc.ChangeOwnership(e.ctx, s.Mgr, c.ID, crm.OwnershipInput{Ownership: "owned", OwnerID: s.Andrei.ID})
	must(t, err)
	after, _ := e.svc.CustomerProfile(e.ctx, s.Mgr, c.ID, 0)
	if after.Total != before.Total || !after.Customer.LastInteractionAt.Equal(*before.Customer.LastInteractionAt) {
		t.Fatal("ownership change invented an interaction")
	}
	if after.FollowUps[0].EmployeeID != s.Ioana.ID {
		t.Fatal("ownership change silently moved tasks")
	}
	if len(after.OwnershipHistory) != 2 || len(after.Audit) < 4 {
		t.Fatalf("ownership history not preserved: %d", len(after.OwnershipHistory))
	}
	// Both employees are told about the reassignment; the manager (actor) is not.
	for _, u := range []crm.User{s.Andrei, s.Ioana} {
		n, err := e.svc.Notifications(e.ctx, u, true)
		must(t, err)
		if n.Unread != 1 || n.Items[0].CustomerName != "Ion Ionescu" {
			t.Fatalf("notifications for %s: %+v", u.Name, n)
		}
	}
	// A colleague's visit keeps ownership and notifies the owner.
	e.now = e.now.Add(time.Minute)
	_, err = e.svc.RecordVisit(e.ctx, s.Ioana, c.ID, crm.VisitInput{Steps: []int{1}})
	must(t, err)
	got, _ := e.svc.CustomerProfile(e.ctx, s.Ioana, c.ID, 0)
	if got.Customer.OwnerID != s.Andrei.ID || len(got.Audit) != 0 {
		t.Fatal("colleague visit changed ownership, or employee sees audit")
	}
	n, _ := e.svc.Notifications(e.ctx, s.Andrei, true)
	if n.Unread != 2 || n.Items[0].Kind != "customer_returned" {
		t.Fatalf("owner not told the customer returned: %+v", n.Items)
	}
	must(t, e.svc.MarkNotificationsRead(e.ctx, s.Andrei, nil))
	n, _ = e.svc.Notifications(e.ctx, s.Andrei, true)
	if n.Unread != 0 {
		t.Fatal("mark read")
	}
}

func TestFollowUpLifecycle(t *testing.T) {
	e := setup(t)
	s := e.st
	c := e.customer(t, s.Ioana, "Ana Stan", "0722 000 200")
	o, err := e.svc.CreateOpportunity(e.ctx, s.Ioana, crm.OpportunityInput{CustomerID: c.ID, OpportunityDraft: crm.OpportunityDraft{Product: "Internet"}})
	must(t, err)
	f, err := e.svc.ScheduleFollowUp(e.ctx, s.Ioana, crm.FollowUpInput{CustomerID: c.ID, OpportunityID: o.ID, Type: "Prepare offer", Due: "2026-10-04"})
	must(t, err)
	for _, status := range []string{"unreachable", "waiting", "open"} {
		_, err = e.svc.UpdateFollowUp(e.ctx, s.Ioana, f.ID, crm.FollowUpUpdate{Status: status, Due: "2026-10-05"})
		must(t, err)
	}
	_, err = e.svc.UpdateFollowUp(e.ctx, s.Ioana, f.ID, crm.FollowUpUpdate{Status: "waiting"})
	is(t, err, apperr.ErrValidation)
	_, err = e.svc.UpdateFollowUp(e.ctx, s.Andrei, f.ID, crm.FollowUpUpdate{Status: "done"})
	is(t, err, apperr.ErrForbidden)
	_, err = e.svc.UpdateFollowUp(e.ctx, e.other.Mgr, f.ID, crm.FollowUpUpdate{Status: "done"})
	is(t, err, apperr.ErrFollowUpNotFound)

	done, err := e.svc.UpdateFollowUp(e.ctx, s.Ioana, f.ID, crm.FollowUpUpdate{Status: "done"})
	must(t, err)
	if done.CompletedAt == nil || done.Due != "2026-10-05" {
		t.Fatal("completion lost the due date or timestamp")
	}
	audits := func() int {
		p, _ := e.svc.CustomerProfile(e.ctx, s.Mgr, c.ID, 0)
		return len(p.Audit)
	}
	n := audits()
	_, err = e.svc.UpdateFollowUp(e.ctx, s.Ioana, f.ID, crm.FollowUpUpdate{Status: "done"})
	must(t, err)
	if audits() != n {
		t.Fatal("repeated completion recorded a second event")
	}
	_, err = e.svc.UpdateFollowUp(e.ctx, s.Ioana, f.ID, crm.FollowUpUpdate{Status: "open", Due: "2026-10-06"})
	is(t, err, apperr.ErrConflict)

	// Assignment rules.
	_, err = e.svc.ScheduleFollowUp(e.ctx, s.Ioana, crm.FollowUpInput{CustomerID: c.ID, EmployeeID: s.Andrei.ID, Type: "Call", Due: "2026-10-05"})
	is(t, err, apperr.ErrForbidden)
	mf, err := e.svc.ScheduleFollowUp(e.ctx, s.Mgr, crm.FollowUpInput{CustomerID: c.ID, EmployeeID: s.Andrei.ID, Type: "Call", Due: "2026-10-05"})
	must(t, err)
	if nl, _ := e.svc.Notifications(e.ctx, s.Andrei, true); nl.Unread != 1 || nl.Items[0].Kind != "followup_assigned" {
		t.Fatal("assignee not notified")
	}
	_, err = e.svc.UpdateFollowUp(e.ctx, s.Mgr, mf.ID, crm.FollowUpUpdate{Status: "waiting", Due: "2026-10-07"})
	must(t, err)
	_, err = e.svc.ScheduleFollowUp(e.ctx, s.Andrei, crm.FollowUpInput{CustomerID: c.ID, OpportunityID: o.ID, Type: "Call", Due: "2026-10-05"})
	is(t, err, apperr.ErrForbidden)
	c2 := e.customer(t, s.Ioana, "Alt Client", "0722 000 201")
	_, err = e.svc.ScheduleFollowUp(e.ctx, s.Ioana, crm.FollowUpInput{CustomerID: c2.ID, OpportunityID: o.ID, Type: "Call", Due: "2026-10-05"})
	is(t, err, apperr.ErrValidation)

	// Listing scopes and due filters (store today is 2026-09-28).
	_, err = e.svc.ScheduleFollowUp(e.ctx, s.Ioana, crm.FollowUpInput{CustomerID: c.ID, Type: "Late", Due: "2026-09-20"})
	must(t, err)
	late, err := e.svc.ListFollowUps(e.ctx, s.Ioana, crm.WorkQuery{Due: "overdue"})
	must(t, err)
	if late.Total != 1 || late.Items[0].Type != "Late" {
		t.Fatalf("overdue: %+v", late)
	}
	_, err = e.svc.ListFollowUps(e.ctx, s.Ioana, crm.WorkQuery{Employee: s.Andrei.ID})
	is(t, err, apperr.ErrForbidden)
	all, err := e.svc.ListFollowUps(e.ctx, s.Mgr, crm.WorkQuery{Employee: "all", Status: "all"})
	must(t, err)
	if all.Total != 3 {
		t.Fatalf("manager store-wide list: %d", all.Total)
	}
}

func TestOpportunityTransitions(t *testing.T) {
	e := setup(t)
	s := e.st
	c := e.customer(t, s.Ioana, "Dan Rus", "0722 000 300")
	o, err := e.svc.CreateOpportunity(e.ctx, s.Ioana, crm.OpportunityInput{CustomerID: c.ID, OpportunityDraft: crm.OpportunityDraft{Product: "Galaxy", Category: "device"}})
	must(t, err)
	stage := func(v string) *string { return &v }
	_, err = e.svc.UpdateOpportunity(e.ctx, s.Andrei, o.ID, crm.OpportunityPatch{Stage: stage("offer")})
	is(t, err, apperr.ErrForbidden)
	_, err = e.svc.UpdateOpportunity(e.ctx, s.Ioana, o.ID, crm.OpportunityPatch{Stage: stage("signed")})
	is(t, err, apperr.ErrValidation)

	e.now = e.now.Add(time.Hour)
	moved, err := e.svc.UpdateOpportunity(e.ctx, s.Ioana, o.ID, crm.OpportunityPatch{Stage: stage("offer")})
	must(t, err)
	e.now = e.now.Add(time.Hour)
	same, err := e.svc.UpdateOpportunity(e.ctx, s.Ioana, o.ID, crm.OpportunityPatch{Stage: stage("offer")})
	must(t, err)
	if !same.StageChangedAt.Equal(moved.StageChangedAt) {
		t.Fatal("repeating a stage reset its age")
	}
	won, err := e.svc.UpdateOpportunity(e.ctx, s.Mgr, o.ID, crm.OpportunityPatch{Stage: stage("won")})
	must(t, err)
	if won.ClosedAt == nil {
		t.Fatal("closedAt missing")
	}
	_, err = e.svc.UpdateOpportunity(e.ctx, s.Ioana, o.ID, crm.OpportunityPatch{Stage: stage("offer")})
	is(t, err, apperr.ErrInvalidStageTransition)
	_, err = e.svc.ScheduleFollowUp(e.ctx, s.Ioana, crm.FollowUpInput{CustomerID: c.ID, OpportunityID: o.ID, Type: "Call", Due: "2026-10-01"})
	is(t, err, apperr.ErrInvalidStageTransition)

	rep, err := e.svc.ManagerDashboard(e.ctx, s.Mgr, "", "")
	must(t, err)
	if rep.Summary.Offers != 1 || rep.Summary.Contracts != 1 || rep.Summary.OpportunitiesCreated != 1 {
		t.Fatalf("events not reflected in report: %+v", rep.Summary)
	}
	if n, _ := e.svc.Notifications(e.ctx, s.Ioana, true); n.Unread != 1 || n.Items[0].Kind != "opportunity_updated" {
		t.Fatal("owner not told a manager closed their opportunity")
	}
}

func TestStoreIsolation(t *testing.T) {
	e := setup(t)
	c := e.customer(t, e.st.Ioana, "Secret Client", "0722 000 400")
	res, err := e.svc.RecordVisit(e.ctx, e.st.Ioana, c.ID, crm.VisitInput{Steps: []int{0}, NextAction: "Call", Due: "2026-10-01", Opportunities: []crm.OpportunityDraft{{Product: "X"}}})
	must(t, err)
	outsider := e.other.Mgr
	_, err = e.svc.CustomerProfile(e.ctx, outsider, c.ID, 0)
	is(t, err, apperr.ErrCustomerNotFound)
	_, err = e.svc.RecordVisit(e.ctx, outsider, c.ID, crm.VisitInput{Steps: []int{0}})
	is(t, err, apperr.ErrCustomerNotFound)
	_, err = e.svc.UpdateOpportunity(e.ctx, outsider, res.Opportunities[0].ID, crm.OpportunityPatch{})
	is(t, err, apperr.ErrOpportunityNotFound)
	_, err = e.svc.UpdateVisitNotes(e.ctx, outsider, res.Visit.ID, "x")
	is(t, err, apperr.ErrVisitNotFound)
	list, err := e.svc.ListCustomers(e.ctx, outsider, crm.CustomerFilter{Query: "Secret"})
	must(t, err)
	ws, err := e.svc.Workspace(e.ctx, outsider)
	must(t, err)
	rep, err := e.svc.ManagerDashboard(e.ctx, outsider, "", "")
	must(t, err)
	if list.Total != 0 || len(ws.Customers) != 0 || len(ws.FollowUps) != 0 || len(ws.Opportunities) != 0 || rep.Summary.Visits != 0 || rep.Summary.NewCustomers != 0 {
		t.Fatal("data leaked across stores")
	}
}

func TestSearchFiltersAndPaging(t *testing.T) {
	e := setup(t)
	s := e.st
	for i, name := range []string{"Ștefan Tănase", "Maria Ionescu", "Mihai Pop", "Ana Pop"} {
		e.now = e.now.Add(time.Minute)
		c := e.customer(t, s.Ioana, name, "0722 345 67"+string(rune('0'+i)))
		if i < 2 {
			_, err := e.svc.ChangeOwnership(e.ctx, s.Ioana, c.ID, crm.OwnershipInput{Ownership: "owned"})
			must(t, err)
		}
	}
	search := func(f crm.CustomerFilter) crm.Page[crm.Customer] {
		t.Helper()
		p, err := e.svc.ListCustomers(e.ctx, s.Andrei, f)
		must(t, err)
		return p
	}
	if p := search(crm.CustomerFilter{Query: "stefan tanase"}); p.Total != 1 {
		t.Fatal("diacritic-insensitive name search")
	}
	for _, q := range []string{"0722345671", "0722 345 671", "+40 722 345 671", "345671"} {
		if p := search(crm.CustomerFilter{Query: q}); p.Total != 1 || p.Items[0].Name != "Maria Ionescu" {
			t.Fatalf("phone search %q: %+v", q, p.Items)
		}
	}
	if p := search(crm.CustomerFilter{Query: "100%_"}); p.Total != 0 {
		t.Fatal("LIKE wildcards must be escaped")
	}
	p, err := e.svc.ListCustomers(e.ctx, s.Ioana, crm.CustomerFilter{OwnerID: "me"})
	must(t, err)
	if p.Total != 2 {
		t.Fatal("portfolio filter")
	}
	if p := search(crm.CustomerFilter{Ownership: "pool", Sort: "name"}); p.Total != 2 || p.Items[0].Name != "Ana Pop" {
		t.Fatal("ownership filter and sort")
	}
	p1, p2 := search(crm.CustomerFilter{Limit: 3}), search(crm.CustomerFilter{Limit: 3, Offset: 3})
	if p1.Total != 4 || len(p1.Items) != 3 || len(p2.Items) != 1 || p1.Items[0].Name != "Ana Pop" {
		t.Fatal("paging or recent-first order")
	}
	_, err = e.svc.ListCustomers(e.ctx, s.Ioana, crm.CustomerFilter{Sort: "drop table"})
	is(t, err, apperr.ErrValidation)
}

func TestReportUsesStoreDaysAndRecordedEvents(t *testing.T) {
	e := setup(t)
	s := e.st
	c := e.customer(t, s.Ioana, "Raport Client", "0722 000 500")
	// Bucharest is UTC+3 in September: 2026-09-28 local runs from 27th 21:00 to 28th 21:00 UTC.
	visitAt := func(at string, steps ...int) {
		t.Helper()
		e.now, _ = time.Parse(time.RFC3339, at)
		_, err := e.svc.RecordVisit(e.ctx, s.Ioana, c.ID, crm.VisitInput{Steps: steps})
		must(t, err)
	}
	visitAt("2026-09-27T20:59:59Z", 0)
	visitAt("2026-09-27T21:00:00Z", 0, 3, 4)
	visitAt("2026-09-28T20:59:59Z", 5, 6)
	visitAt("2026-09-28T21:00:00Z", 0)
	rep, err := e.svc.ManagerDashboard(e.ctx, s.Mgr, "2026-09-28", "2026-09-28")
	must(t, err)
	if rep.Summary.Visits != 2 || rep.Summary.CustomersHandled != 1 {
		t.Fatalf("local-day boundary: %+v", rep.Summary)
	}
	// Funnel counts visits by furthest step reached; rates are relative to the previous level.
	want := []int{2, 2, 2, 1, 1, 0}
	for i, level := range rep.Funnel {
		if level.Count != want[i] {
			t.Fatalf("funnel level %d (%s) = %d, want %d", i, level.Label, level.Count, want[i])
		}
	}
	if rep.Funnel[3].Rate != 0.5 || rep.StepIncidence[0].Count != 1 || rep.StepIncidence[5].Count != 1 {
		t.Fatalf("rates/incidence: %+v %+v", rep.Funnel, rep.StepIncidence)
	}
	var ioana crm.EmployeeReport
	for _, r := range rep.Employees {
		if r.EmployeeID == s.Ioana.ID {
			ioana = r
		}
	}
	if ioana.Visits != 2 || len(rep.Employees) != 3 {
		t.Fatalf("employee rows: %+v", rep.Employees)
	}
	act, err := e.svc.EmployeeActivity(e.ctx, s.Mgr, s.Ioana.ID, "2026-09-28", "2026-09-28", 0, 10)
	must(t, err)
	if act.Total != 2 || act.Items[0].Steps[0] != 5 {
		t.Fatalf("activity: %+v", act)
	}
	_, err = e.svc.EmployeeActivity(e.ctx, s.Andrei, s.Ioana.ID, "", "", 0, 10)
	is(t, err, apperr.ErrForbidden)
	_, err = e.svc.ManagerDashboard(e.ctx, s.Ioana, "", "")
	is(t, err, apperr.ErrForbidden)
	for _, r := range [][2]string{{"2026-09-28", ""}, {"bad", "bad"}, {"2026-09-29", "2026-09-28"}} {
		_, err = e.svc.ManagerDashboard(e.ctx, s.Mgr, r[0], r[1])
		is(t, err, apperr.ErrValidation)
	}
}

func TestVisitNotesAndCustomerEdits(t *testing.T) {
	e := setup(t)
	s := e.st
	c := e.customer(t, s.Ioana, "Note Client", "0722 000 600")
	res, err := e.svc.RecordVisit(e.ctx, s.Ioana, c.ID, crm.VisitInput{Steps: []int{0}, Notes: "Prima versiune", Ownership: "owned"})
	must(t, err)
	_, err = e.svc.UpdateVisitNotes(e.ctx, s.Andrei, res.Visit.ID, "Altceva")
	is(t, err, apperr.ErrForbidden)
	v, err := e.svc.UpdateVisitNotes(e.ctx, s.Ioana, res.Visit.ID, "Versiune corectată")
	must(t, err)
	if v.NotesEditedAt == nil || v.Notes != "Versiune corectată" {
		t.Fatal("note edit")
	}
	e.now = e.now.Add(8 * 24 * time.Hour)
	_, err = e.svc.UpdateVisitNotes(e.ctx, s.Ioana, res.Visit.ID, "Prea târziu")
	is(t, err, apperr.ErrForbidden)
	_, err = e.svc.UpdateVisitNotes(e.ctx, s.Mgr, res.Visit.ID, "Corectat de manager")
	must(t, err)
	var revisions int
	must(t, e.db.Pool.QueryRow(e.ctx, `SELECT count(*) FROM visit_note_revisions WHERE visit_id = $1`, res.Visit.ID).Scan(&revisions))
	if revisions != 2 {
		t.Fatalf("revisions = %d", revisions)
	}

	name := "Andrei Nou"
	_, err = e.svc.UpdateCustomer(e.ctx, s.Andrei, c.ID, crm.CustomerPatch{Name: &name})
	is(t, err, apperr.ErrForbidden)
	phone, tags := "+40 733 000 600", []string{"VIP", "vip", "Familie"}
	up, err := e.svc.UpdateCustomer(e.ctx, s.Ioana, c.ID, crm.CustomerPatch{Phone: &phone, Tags: &tags})
	must(t, err)
	if up.Phone != "+40733000600" || len(up.Tags) != 2 {
		t.Fatalf("edit: %+v", up)
	}
	archived := crm.CustomerArchived
	_, err = e.svc.UpdateCustomer(e.ctx, s.Ioana, c.ID, crm.CustomerPatch{Status: &archived})
	is(t, err, apperr.ErrForbidden)

	is(t, e.svc.AnonymizeCustomer(e.ctx, s.Ioana, c.ID), apperr.ErrForbidden)
	must(t, e.svc.AnonymizeCustomer(e.ctx, s.Mgr, c.ID))
	p, err := e.svc.CustomerProfile(e.ctx, s.Mgr, c.ID, 0)
	must(t, err)
	if p.Customer.Name != "Client anonimizat" || p.Customer.Phone != "" || p.Visits[0].Notes != "" || p.Total != 1 {
		t.Fatalf("anonymization: %+v", p.Customer)
	}
	if l, _ := e.svc.ListCustomers(e.ctx, s.Mgr, crm.CustomerFilter{}); l.Total != 0 {
		t.Fatal("anonymized customer listed")
	}
	_, err = e.svc.RecordVisit(e.ctx, s.Mgr, c.ID, crm.VisitInput{Steps: []int{0}})
	is(t, err, apperr.ErrConflict)
}

func TestWorkspaceAndDashboardScope(t *testing.T) {
	e := setup(t)
	s := e.st
	mine := e.customer(t, s.Ioana, "Clientul Ioanei", "0722 000 700")
	theirs := e.customer(t, s.Andrei, "Clientul lui Andrei", "0722 000 701")
	e.customer(t, s.Andrei, "Client Magazin", "0722 000 702")
	_, err := e.svc.RecordVisit(e.ctx, s.Ioana, mine.ID, crm.VisitInput{Steps: []int{0}, Ownership: "owned", NextAction: "Call", Due: "2026-09-28", Opportunities: []crm.OpportunityDraft{{Product: "A"}}})
	must(t, err)
	_, err = e.svc.RecordVisit(e.ctx, s.Andrei, theirs.ID, crm.VisitInput{Steps: []int{0}, Ownership: "owned", NextAction: "Call", Due: "2026-09-20"})
	must(t, err)

	ws, err := e.svc.Workspace(e.ctx, s.Ioana)
	must(t, err)
	if len(ws.Customers) != 1 || len(ws.FollowUps) != 1 || len(ws.Opportunities) != 1 || ws.Today != "2026-09-28" {
		t.Fatalf("employee workspace: customers=%d followUps=%d", len(ws.Customers), len(ws.FollowUps))
	}
	ws, err = e.svc.Workspace(e.ctx, s.Mgr)
	must(t, err)
	if len(ws.FollowUps) != 2 || len(ws.Customers) != 3 {
		t.Fatalf("manager workspace: customers=%d followUps=%d", len(ws.Customers), len(ws.FollowUps))
	}
	d, err := e.svc.EmployeeDashboard(e.ctx, s.Ioana)
	must(t, err)
	if d.MyCustomers != 1 || d.DueToday != 1 || d.Overdue != 0 || d.ActiveOpportunities != 1 || d.Pipeline["identified"] != 1 || len(d.RecentlyAssigned) != 1 {
		t.Fatalf("dashboard: %+v", d)
	}
	d, err = e.svc.EmployeeDashboard(e.ctx, s.Andrei)
	must(t, err)
	if d.Overdue != 1 {
		t.Fatal("overdue count")
	}
}
