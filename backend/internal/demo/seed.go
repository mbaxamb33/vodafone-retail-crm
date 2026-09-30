// Package demo creates a fictional store with realistic history for local development.
// All history is produced through the CRM service with a simulated clock, so audit events,
// stage transitions and reports behave exactly as they would for real usage.
package demo

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"vodafone/store/internal/apperr"
	"vodafone/store/internal/auth"
	"vodafone/store/internal/crm"
	"vodafone/store/internal/postgres"
)

const StoreName = "Magazin București · Demo"

type Account struct {
	Email string
	Name  string
	Role  string
}

var Accounts = []Account{
	{"ioana.marinescu@demo.local", "Ioana Marinescu", crm.RoleEmployee},
	{"andrei.popescu@demo.local", "Andrei Popescu", crm.RoleEmployee},
	{"elena.dumitrescu@demo.local", "Elena Dumitrescu", crm.RoleManager},
}

var ErrAlreadySeeded = errors.New("demo store already exists")

// Seed creates the demo store, accounts and about two months of activity.
func Seed(ctx context.Context, db *postgres.DB, authSvc *auth.Service, password string, now time.Time) error {
	if _, err := db.StoreByName(ctx, StoreName); err == nil {
		return ErrAlreadySeeded
	} else if !errors.Is(err, apperr.ErrNotFound) {
		return err
	}
	store := crm.RetailStore{ID: crm.NewID(), Name: StoreName, Timezone: "Europe/Bucharest"}
	if err := db.InsertStore(ctx, store); err != nil {
		return err
	}
	users := map[string]crm.User{}
	for _, a := range Accounts {
		u, err := authSvc.CreateUser(ctx, auth.NewUser{StoreID: store.ID, Email: a.Email, Name: a.Name, Role: a.Role, Password: password})
		if err != nil {
			return fmt.Errorf("create %s: %w", a.Email, err)
		}
		first, _, _ := strings.Cut(a.Name, " ")
		users[first] = u
	}
	ioana, andrei, elena := users["Ioana"], users["Andrei"], users["Elena"]

	loc, _ := time.LoadLocation(store.Timezone)
	today := now.In(loc)
	at := today
	svc := crm.NewService(db, crm.WithClock(func() time.Time { return at }))
	dueIn := func(days int) string { return today.AddDate(0, 0, days).Format(time.DateOnly) }
	moment := func(daysAgo, hour int) time.Time {
		d := today.AddDate(0, 0, -daysAgo)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, 15*(daysAgo%4), 0, 0, loc)
	}

	type plan struct {
		name, phone string
		by          crm.User
		daysAgo     int
		reason      string
		steps       []int
		ownership   string
		product     string
		category    string
		value       float64
		stages      []string // later stage changes, one every three days
		nextAction  string
		due         int // days from today
		notes       string
		returnVisit []int // steps of a second visit a week later
	}
	plans := []plan{
		{"Alexandru Ionescu", "0722 000 101", ioana, 58, "renewal", []int{0, 1, 3, 4}, "owned", "Reînnoire Red Unlimited", "renewal", 65, []string{"qualified", "presentation", "offer"}, "Discută oferta", -3, "Contractul expiră luna viitoare. Preferă să fie sunat după ora 17.", []int{5, 6}},
		{"Cristina Dumitru", "0722 000 102", ioana, 52, "device", []int{0, 2, 3, 4, 5}, "owned", "iPhone 16 Pro", "device", 1400, []string{"presentation", "offer", "won"}, "", 0, "Interesată de schimbarea telefonului cu rate.", []int{6, 7}},
		{"Mihai Popa", "0722 000 103", andrei, 47, "internet", []int{0, 1, 3}, "owned", "Internet acasă", "home_internet", 40, []string{"qualified", "verification"}, "Verifică eligibilitatea", -1, "Verificăm acoperirea la adresa nouă.", nil},
		{"Andreea Stan", "0722 000 104", ioana, 41, "billing", []int{0, 1}, "pool", "", "", 0, nil, "", 0, "Întrebare despre factură rezolvată.", nil},
		{"Radu Georgescu", "0722 000 105", andrei, 36, "new_subscription", []int{0, 2, 3, 4, 5, 6}, "owned", "Abonament Red 11", "mobile", 55, []string{"offer", "waiting"}, "Clientul revine", 0, "Compară cu oferta actuală; revine după salariu.", nil},
		{"Diana Rusu", "0722 000 106", ioana, 30, "tv", []int{0, 1, 3, 5}, "owned", "Internet + TV", "tv", 70, []string{"presentation", "lost"}, "", 0, "A ales alt furnizor pentru TV.", nil},
		{"Gabriel Munteanu", "0722 000 107", andrei, 24, "support", []int{0, 1}, "pool", "", "", 0, nil, "", 0, "Înlocuire SIM.", nil},
		{"Maria Dobre", "0722 000 108", andrei, 19, "renewal", []int{0, 3, 4}, "owned", "Reînnoire abonament", "renewal", 50, []string{"qualified"}, "Sună clientul", 1, "Dorește mai mult trafic de date.", []int{4, 5}},
		{"Vlad Petrescu", "0722 000 109", ioana, 14, "accessories", []int{0, 1, 2}, "unassigned", "", "", 0, nil, "", 0, "Doar accesorii, fără interes pentru abonament.", nil},
		{"Elisabeta Nistor", "0722 000 110", ioana, 9, "mobile_plan", []int{0, 1, 3, 4, 5}, "owned", "Samsung Galaxy S25", "device", 1100, []string{"offer"}, "Pregătește oferta", 2, "Vrea telefon pentru fiică, buget limitat.", nil},
		{"Sorin Constantin", "0722 000 111", andrei, 5, "technical_issue", []int{0, 1, 2, 3}, "owned", "Internet acasă 1 Gbps", "home_internet", 45, nil, "Sună clientul", 4, "Problemă de semnal rezolvată; interesat de fibră.", nil},
		{"Ana Vasilescu", "0722 000 112", ioana, 2, "contract_question", []int{0, 1}, "pool", "", "", 0, nil, "", 0, "", nil},
	}

	for _, p := range plans {
		at = moment(p.daysAgo, 10+p.daysAgo%7)
		c, err := svc.CreateCustomer(ctx, p.by, crm.CustomerInput{Name: p.name, Phone: p.phone})
		if err != nil {
			return fmt.Errorf("customer %s: %w", p.name, err)
		}
		at = at.Add(4 * time.Minute)
		in := crm.VisitInput{ReasonCode: p.reason, Steps: p.steps, Notes: p.notes, Resolution: resolutionFor(p.reason, p.steps)}
		if p.ownership == crm.OwnershipOwned {
			in.Ownership = crm.OwnershipOwned
		}
		if p.product != "" {
			value := p.value
			in.Opportunities = []crm.OpportunityDraft{{Product: p.product, Category: p.category, EstimatedValue: &value}}
		}
		if p.nextAction != "" {
			action := demoActions[p.nextAction]
			in.NextAction, in.ActionDetails, in.AgreedDate, in.Due = action[0], action[1], true, dueIn(p.due)
		}
		if in.Resolution != nil && in.Resolution.Status == "pending" {
			in.Reminder = &crm.ReminderInput{Due: dueIn(p.due + 10), Notes: "S-a rezolvat problema tehnică?"}
		}
		res, err := svc.RecordVisit(ctx, p.by, c.ID, in)
		if err != nil {
			return fmt.Errorf("visit %s: %w", p.name, err)
		}
		for i, stage := range p.stages {
			at = moment(p.daysAgo-3*(i+1), 12)
			if _, err := svc.UpdateOpportunity(ctx, p.by, res.Opportunities[0].ID, crm.OpportunityPatch{Stage: &stage}); err != nil {
				return fmt.Errorf("stage %s: %w", p.name, err)
			}
		}
		if p.returnVisit != nil {
			at = moment(p.daysAgo-7, 16)
			if _, err := svc.RecordVisit(ctx, p.by, c.ID, crm.VisitInput{ReasonCode: p.reason, Steps: p.returnVisit, Notes: "A revenit pentru a continua discuția.", Resolution: resolutionFor(p.reason, p.returnVisit)}); err != nil {
				return fmt.Errorf("return visit %s: %w", p.name, err)
			}
		}
	}

	// A customer returned to the pool and claimed by a colleague, and a colleague visit, so
	// notifications and audit have variety.
	at = moment(3, 11)
	page, err := svc.ListCustomers(ctx, elena, crm.CustomerFilter{Query: "Maria Dobre", Limit: 1})
	if err != nil || len(page.Items) == 0 {
		return fmt.Errorf("find reassignment customer: %w", err)
	}
	if _, err := svc.ChangeOwnership(ctx, andrei, page.Items[0].ID, crm.OwnershipInput{Ownership: crm.OwnershipPool}); err != nil {
		return err
	}
	at = at.Add(time.Hour)
	if _, err := svc.ChangeOwnership(ctx, ioana, page.Items[0].ID, crm.OwnershipInput{Ownership: crm.OwnershipOwned}); err != nil {
		return err
	}
	at = moment(1, 15)
	page, err = svc.ListCustomers(ctx, andrei, crm.CustomerFilter{Query: "Alexandru", Limit: 1})
	if err != nil || len(page.Items) == 0 {
		return fmt.Errorf("find colleague visit customer: %w", err)
	}
	_, err = svc.RecordVisit(ctx, andrei, page.Items[0].ID, crm.VisitInput{ReasonCode: "support", Steps: []int{0, 1}, Notes: "A trecut pentru o problemă de roaming; Ioana are discuția comercială.",
		Resolution: &crm.ResolutionInput{Type: "other", Status: "resolved"}})
	return err
}

// demoActions maps the demo's next steps to catalog codes and details.
var demoActions = map[string][2]string{
	"Discută oferta":          {"thinking", ""},
	"Verifică eligibilitatea": {"other", "Verificăm eligibilitatea la adresa nouă"},
	"Clientul revine":         {"thinking", ""},
	"Sună clientul":           {"keep_in_touch", ""},
	"Pregătește oferta":       {"other", "Pregătim oferta pentru telefon"},
}

// resolutionFor describes the request whenever the request-resolution step was performed.
func resolutionFor(reason string, steps []int) *crm.ResolutionInput {
	if !slices.Contains(steps, crm.ResolutionStep) {
		return nil
	}
	switch reason {
	case "billing":
		return &crm.ResolutionInput{Type: "invoice", Holder: "holder"}
	case "technical_issue", "internet":
		return &crm.ResolutionInput{Type: "other", Status: "pending"}
	}
	return &crm.ResolutionInput{Type: "other", Status: "resolved"}
}
