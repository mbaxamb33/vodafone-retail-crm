package crm

import (
	"context"
	"time"

	"vodafone/store/internal/apperr"
)

// Workspace is the signed-in user's working set: store colleagues, their follow-ups and
// opportunities (store-wide for managers) and the customers those refer to or that they own.
// The full store directory is served by the paginated customers endpoint instead.
type Workspace struct {
	Users         []User        `json:"users"`
	Customers     []Customer    `json:"customers"`
	FollowUps     []FollowUp    `json:"followUps"`
	Opportunities []Opportunity `json:"opportunities"`
	Today         string        `json:"today"`
}

// Completed follow-ups and closed opportunities stay in the workspace for this long.
const (
	workspaceDoneWindow   = 30 * 24 * time.Hour
	workspaceClosedWindow = 90 * 24 * time.Hour
	workspaceMaxRows      = 2000
)

func (s *Service) Workspace(ctx context.Context, actor User) (Workspace, error) {
	var w Workspace
	var err error
	if w.Today, err = s.today(ctx, actor.StoreID); err != nil {
		return w, err
	}
	if w.Users, err = s.Users(ctx, actor); err != nil {
		return w, err
	}
	employee := actor.ID
	if actor.Can(PermViewStoreWork) {
		employee = ""
	}
	now := s.clock()
	doneSince, closedSince := now.Add(-workspaceDoneWindow), now.Add(-workspaceClosedWindow)
	if w.FollowUps, _, err = s.store.FollowUps(ctx, actor.StoreID, FollowUpFilter{EmployeeID: employee, OpenOrCompletedSince: &doneSince, Limit: workspaceMaxRows}); err != nil {
		return w, err
	}
	if w.Opportunities, _, err = s.store.Opportunities(ctx, actor.StoreID, OpportunityFilter{EmployeeID: employee, ClosedSince: &closedSince, Limit: workspaceMaxRows}); err != nil {
		return w, err
	}
	seen := map[string]bool{}
	add := func(cs []Customer) {
		for _, c := range cs {
			if !seen[c.ID] {
				seen[c.ID] = true
				w.Customers = append(w.Customers, c)
			}
		}
	}
	w.Customers = []Customer{}
	owned, _, err := s.store.Customers(ctx, actor.StoreID, CustomerFilter{OwnerID: actor.ID, Sort: "recent", Limit: workspaceMaxRows})
	if err != nil {
		return w, err
	}
	add(owned)
	if actor.Can(PermViewStoreWork) {
		pool, _, err := s.store.Customers(ctx, actor.StoreID, CustomerFilter{Ownership: OwnershipPool, Status: CustomerActive, Sort: "recent", Limit: workspaceMaxRows})
		if err != nil {
			return w, err
		}
		add(pool)
	}
	missing := []string{}
	for _, f := range w.FollowUps {
		if !seen[f.CustomerID] {
			missing = append(missing, f.CustomerID)
			seen[f.CustomerID] = true
		}
	}
	for _, o := range w.Opportunities {
		if !seen[o.CustomerID] {
			missing = append(missing, o.CustomerID)
			seen[o.CustomerID] = true
		}
	}
	if len(missing) > 0 {
		referenced, err := s.store.CustomersByID(ctx, actor.StoreID, missing)
		if err != nil {
			return w, err
		}
		w.Customers = append(w.Customers, referenced...)
	}
	return w, nil
}

type EmployeeDashboard struct {
	Today               string         `json:"today"`
	MyCustomers         int            `json:"myCustomers"`
	ActiveOpportunities int            `json:"activeOpportunities"`
	Overdue             int            `json:"overdue"`
	DueToday            int            `json:"dueToday"`
	Upcoming            int            `json:"upcoming"`
	Waiting             int            `json:"waiting"`
	OffersAwaiting      int            `json:"offersAwaiting"`
	Pipeline            map[string]int `json:"pipeline"`
	RecentCustomers     []Customer     `json:"recentCustomers"`
	RecentlyAssigned    []Customer     `json:"recentlyAssigned"`
	UnreadNotifications int            `json:"unreadNotifications"`
}

// EmployeeDashboard answers "what should I work on now?" for the signed-in user.
func (s *Service) EmployeeDashboard(ctx context.Context, actor User) (EmployeeDashboard, error) {
	d := EmployeeDashboard{Pipeline: map[string]int{}, RecentlyAssigned: []Customer{}}
	var err error
	if d.Today, err = s.today(ctx, actor.StoreID); err != nil {
		return d, err
	}
	if _, d.MyCustomers, err = s.store.Customers(ctx, actor.StoreID, CustomerFilter{OwnerID: actor.ID, Limit: 1}); err != nil {
		return d, err
	}
	active := []string{FollowUpOpen, FollowUpWaiting, FollowUpUnreachable}
	followUps, _, err := s.store.FollowUps(ctx, actor.StoreID, FollowUpFilter{EmployeeID: actor.ID, Statuses: active, Limit: workspaceMaxRows})
	if err != nil {
		return d, err
	}
	for _, f := range followUps {
		switch {
		case f.Due < d.Today:
			d.Overdue++
		case f.Due == d.Today:
			d.DueToday++
		default:
			d.Upcoming++
		}
		if f.Status != FollowUpOpen {
			d.Waiting++
		}
	}
	opps, _, err := s.store.Opportunities(ctx, actor.StoreID, OpportunityFilter{EmployeeID: actor.ID, ActiveOnly: true, Limit: workspaceMaxRows})
	if err != nil {
		return d, err
	}
	d.ActiveOpportunities = len(opps)
	for _, o := range opps {
		d.Pipeline[o.Stage]++
		if o.Stage == StageOffer || o.Stage == StageWaiting {
			d.OffersAwaiting++
		}
	}
	if d.RecentCustomers, _, err = s.store.Customers(ctx, actor.StoreID, CustomerFilter{Sort: "recent", Limit: 6}); err != nil {
		return d, err
	}
	since := s.clock().AddDate(0, 0, -14)
	assigned, err := s.store.Audit(ctx, actor.StoreID, AuditFilter{Actions: []string{"customer.owner_changed"}, Since: &since, Limit: 200})
	if err != nil {
		return d, err
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, a := range assigned {
		if a.CustomerID != "" && !seen[a.CustomerID] {
			seen[a.CustomerID] = true
			ids = append(ids, a.CustomerID)
		}
	}
	if len(ids) > 0 {
		cs, err := s.store.CustomersByID(ctx, actor.StoreID, ids)
		if err != nil {
			return d, err
		}
		for _, c := range cs {
			if c.OwnerID == actor.ID && len(d.RecentlyAssigned) < 6 {
				d.RecentlyAssigned = append(d.RecentlyAssigned, c)
			}
		}
	}
	_, d.UnreadNotifications, err = s.store.Notifications(ctx, actor.StoreID, actor.ID, true, 1)
	return d, err
}

type FunnelLevel struct {
	Step  int     `json:"step"`
	Label string  `json:"label"`
	Count int     `json:"count"`
	Rate  float64 `json:"rate"` // share of the previous level, 0 when the previous level is empty
}

type StepCount struct {
	Step  int    `json:"step"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type ReportSummary struct {
	Visits               int `json:"visits"`
	CustomersHandled     int `json:"customersHandled"`
	NewCustomers         int `json:"newCustomers"`
	OpportunitiesCreated int `json:"opportunitiesCreated"`
	Offers               int `json:"offers"`
	Contracts            int `json:"contracts"`
	Lost                 int `json:"lost"`
	PoolCustomers        int `json:"poolCustomers"`
	ActiveOpportunities  int `json:"activeOpportunities"`
	FollowUpsDueToday    int `json:"followUpsDueToday"`
	OverdueFollowUps     int `json:"overdueFollowUps"`
}

type EmployeeReport struct {
	EmployeeID           string `json:"employeeId"`
	Name                 string `json:"name"`
	Role                 string `json:"role"`
	Visits               int    `json:"visits"`
	CustomersHandled     int    `json:"customersHandled"`
	NewCustomers         int    `json:"newCustomers"`
	OpportunitiesCreated int    `json:"opportunitiesCreated"`
	Offers               int    `json:"offers"`
	Contracts            int    `json:"contracts"`
	PortfolioCustomers   int    `json:"portfolioCustomers"`
	ActiveOpportunities  int    `json:"activeOpportunities"`
	OpenFollowUps        int    `json:"openFollowUps"`
	OverdueFollowUps     int    `json:"overdueFollowUps"`
}

// ManagerReport is derived from recorded operational events only. Range figures (visits,
// offers, contracts) cover [from, to] in store-local dates; pool, pipeline and follow-up
// figures describe the current state.
type ManagerReport struct {
	From          string           `json:"from"`
	To            string           `json:"to"`
	Summary       ReportSummary    `json:"summary"`
	Funnel        []FunnelLevel    `json:"funnel"`
	StepIncidence []StepCount      `json:"stepIncidence"`
	Employees     []EmployeeReport `json:"employees"`
}

// funnelSteps are the journey steps shown as funnel levels after "all visits".
var funnelSteps = []int{CommercialStep, 4, 5, 6, 7}

func (s *Service) storeRange(ctx context.Context, storeID, from, to string) (TimeRange, error) {
	if (from == "") != (to == "") {
		return TimeRange{}, apperr.Validation(map[string]string{"range": "Alege ambele date ale intervalului."})
	}
	if from == "" {
		return TimeRange{}, nil
	}
	loc, err := s.location(ctx, storeID)
	if err != nil {
		return TimeRange{}, err
	}
	start, err1 := time.ParseInLocation(time.DateOnly, from, loc)
	end, err2 := time.ParseInLocation(time.DateOnly, to, loc)
	if err1 != nil || err2 != nil || end.Before(start) {
		return TimeRange{}, apperr.Validation(map[string]string{"range": "Alege un interval valid."})
	}
	return TimeRange{From: start, To: end.AddDate(0, 0, 1)}, nil
}

func (s *Service) ManagerDashboard(ctx context.Context, actor User, from, to string) (ManagerReport, error) {
	if err := actor.require(PermViewReports); err != nil {
		return ManagerReport{}, err
	}
	r, err := s.storeRange(ctx, actor.StoreID, from, to)
	if err != nil {
		return ManagerReport{}, err
	}
	today, err := s.today(ctx, actor.StoreID)
	if err != nil {
		return ManagerReport{}, err
	}
	c, err := s.store.ReportCounts(ctx, actor.StoreID, r, today)
	if err != nil {
		return ManagerReport{}, err
	}
	out := ManagerReport{From: from, To: to, Summary: ReportSummary{
		Visits: c.Visits, CustomersHandled: c.CustomersHandled, NewCustomers: c.NewCustomers, OpportunitiesCreated: c.OpportunitiesCreated,
		Offers: c.Offers, Contracts: c.Won, Lost: c.Lost, PoolCustomers: c.PoolCustomers, ActiveOpportunities: c.ActiveOpportunities,
		FollowUpsDueToday: c.FollowUpsDueToday, OverdueFollowUps: c.OverdueFollowUps,
	}}
	out.Funnel = []FunnelLevel{{Step: -1, Label: "Vizite în magazin", Count: c.Visits, Rate: 1}}
	for _, step := range funnelSteps {
		prev := out.Funnel[len(out.Funnel)-1].Count
		level := FunnelLevel{Step: step, Label: JourneySteps[step], Count: c.FurthestAtLeast[step]}
		if step == CommercialStep {
			level.Label = "Conversații comerciale"
		}
		if prev > 0 {
			level.Rate = float64(level.Count) / float64(prev)
		}
		out.Funnel = append(out.Funnel, level)
	}
	for i, label := range JourneySteps {
		out.StepIncidence = append(out.StepIncidence, StepCount{Step: i, Label: label, Count: c.StepIncidence[i]})
	}
	users, err := s.Users(ctx, actor)
	if err != nil {
		return out, err
	}
	counts, err := s.store.EmployeeCounts(ctx, actor.StoreID, r, today)
	if err != nil {
		return out, err
	}
	byID := map[string]EmployeeCounts{}
	for _, e := range counts {
		byID[e.EmployeeID] = e
	}
	out.Employees = []EmployeeReport{}
	for _, u := range users {
		e := byID[u.ID]
		out.Employees = append(out.Employees, EmployeeReport{EmployeeID: u.ID, Name: u.Name, Role: u.Role, Visits: e.Visits, CustomersHandled: e.CustomersHandled, NewCustomers: e.NewCustomers,
			OpportunitiesCreated: e.OpportunitiesCreated, Offers: e.Offers, Contracts: e.Won, PortfolioCustomers: e.PortfolioCustomers, ActiveOpportunities: e.ActiveOpportunities,
			OpenFollowUps: e.OpenFollowUps, OverdueFollowUps: e.OverdueFollowUps})
	}
	return out, nil
}

type EmployeeActivity struct {
	Page[Visit]
	Summary EmployeeReport `json:"summary"`
	// Customers referenced by the listed visits.
	Customers []Customer `json:"customers"`
}

// EmployeeActivity lists an employee's visits in a date range, newest first. Managers may view
// anyone in their store; employees only themselves.
func (s *Service) EmployeeActivity(ctx context.Context, actor User, employeeID, from, to string, offset, limit int) (EmployeeActivity, error) {
	if employeeID == "me" || employeeID == "" {
		employeeID = actor.ID
	}
	if employeeID != actor.ID && !actor.Can(PermViewReports) {
		return EmployeeActivity{}, apperr.ErrForbidden
	}
	if !ValidID(employeeID) {
		return EmployeeActivity{}, apperr.ErrEmployeeNotFound
	}
	u, err := s.member(ctx, s.store, actor, employeeID)
	if err != nil {
		return EmployeeActivity{}, err
	}
	r, err := s.storeRange(ctx, actor.StoreID, from, to)
	if err != nil {
		return EmployeeActivity{}, err
	}
	today, err := s.today(ctx, actor.StoreID)
	if err != nil {
		return EmployeeActivity{}, err
	}
	offset, limit = clampPage(offset, limit, 50, 200)
	items, total, err := s.store.Visits(ctx, actor.StoreID, VisitFilter{EmployeeID: employeeID, Range: r, Offset: offset, Limit: limit})
	if err != nil {
		return EmployeeActivity{}, err
	}
	out := EmployeeActivity{Page: Page[Visit]{Items: items, Total: total, Offset: offset, Limit: limit}, Customers: []Customer{}}
	ids := []string{}
	seen := map[string]bool{}
	for _, v := range items {
		if !seen[v.CustomerID] {
			seen[v.CustomerID] = true
			ids = append(ids, v.CustomerID)
		}
	}
	if len(ids) > 0 {
		if out.Customers, err = s.store.CustomersByID(ctx, actor.StoreID, ids); err != nil {
			return out, err
		}
	}
	counts, err := s.store.EmployeeCounts(ctx, actor.StoreID, r, today)
	if err != nil {
		return out, err
	}
	out.Summary = EmployeeReport{EmployeeID: u.ID, Name: u.Name, Role: u.Role}
	for _, e := range counts {
		if e.EmployeeID == employeeID {
			out.Summary = EmployeeReport{EmployeeID: u.ID, Name: u.Name, Role: u.Role, Visits: e.Visits, CustomersHandled: e.CustomersHandled, NewCustomers: e.NewCustomers,
				OpportunitiesCreated: e.OpportunitiesCreated, Offers: e.Offers, Contracts: e.Won, PortfolioCustomers: e.PortfolioCustomers, ActiveOpportunities: e.ActiveOpportunities,
				OpenFollowUps: e.OpenFollowUps, OverdueFollowUps: e.OverdueFollowUps}
		}
	}
	return out, nil
}

type NotificationList struct {
	Items  []Notification `json:"items"`
	Unread int            `json:"unread"`
}

func (s *Service) Notifications(ctx context.Context, actor User, unreadOnly bool) (NotificationList, error) {
	items, _, err := s.store.Notifications(ctx, actor.StoreID, actor.ID, unreadOnly, 50)
	if err != nil {
		return NotificationList{}, err
	}
	_, unread, err := s.store.Notifications(ctx, actor.StoreID, actor.ID, true, 1)
	return NotificationList{Items: items, Unread: unread}, err
}

// MarkNotificationsRead marks the given notifications, or all when ids is empty, as read.
func (s *Service) MarkNotificationsRead(ctx context.Context, actor User, ids []string) error {
	for _, id := range ids {
		if !ValidID(id) {
			return apperr.ErrNotFound
		}
	}
	return s.store.InTx(ctx, func(tx Tx) error {
		return tx.MarkNotificationsRead(ctx, actor.StoreID, actor.ID, ids, s.clock())
	})
}
