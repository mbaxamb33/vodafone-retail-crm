package crm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type User struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	StoreID string `json:"storeId"`
}
type Customer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	StoreID   string    `json:"storeId"`
	OwnerID   string    `json:"ownerId"`
	Ownership string    `json:"ownership"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type Visit struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customerId"`
	EmployeeID string    `json:"employeeId"`
	At         time.Time `json:"at"`
	Reason     string    `json:"reason"`
	Steps      []int     `json:"steps"`
	Notes      string    `json:"notes"`
}
type FollowUp struct {
	ID            string     `json:"id"`
	CustomerID    string     `json:"customerId"`
	EmployeeID    string     `json:"employeeId"`
	Type          string     `json:"type"`
	Due           string     `json:"due"`
	Status        string     `json:"status"`
	OpportunityID string     `json:"opportunityId,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}
type Opportunity struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customerId"`
	EmployeeID string    `json:"employeeId"`
	Product    string    `json:"product"`
	Stage      string    `json:"stage"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
type Audit struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customerId"`
	ActorID    string    `json:"actorId"`
	Action     string    `json:"action"`
	At         time.Time `json:"at"`
	Detail     string    `json:"detail"`
}
type State struct {
	Users         []User        `json:"users"`
	Customers     []Customer    `json:"customers"`
	Visits        []Visit       `json:"visits"`
	FollowUps     []FollowUp    `json:"followUps"`
	Opportunities []Opportunity `json:"opportunities"`
	Audit         []Audit       `json:"audit"`
}
type Repository interface {
	Load(context.Context) (State, error)
	Save(context.Context, State) error
}
type FileRepository struct{ Path string }

func (r FileRepository) Load(ctx context.Context) (State, error) {
	var s State
	if err := ctx.Err(); err != nil {
		return s, err
	}
	b, e := os.ReadFile(r.Path)
	if e != nil {
		return s, e
	}
	e = json.Unmarshal(b, &s)
	return s, e
}
func (r FileRepository) Save(ctx context.Context, s State) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	b, e := json.Marshal(s)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(r.Path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(r.Path), "state-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), r.Path)
}

type Service struct {
	mu    sync.RWMutex
	repo  Repository
	state State
}

var ErrInvalid = errors.New("validation failed")
var ErrNotFound = errors.New("not found")
var ErrForbidden = errors.New("forbidden")
var ErrConflict = errors.New("conflict")

func ID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func New(ctx context.Context, r Repository, demo bool) (*Service, error) {
	s, e := r.Load(ctx)
	if errors.Is(e, os.ErrNotExist) {
		s = State{Users: []User{}, Customers: []Customer{}, Visits: []Visit{}, FollowUps: []FollowUp{}, Opportunities: []Opportunity{}, Audit: []Audit{}}
		if demo {
			s = Seed()
		}
		e = r.Save(ctx, s)
	}
	return &Service{repo: r, state: s}, e
}
func (s *Service) Snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, _ := json.Marshal(s.state)
	var out State
	_ = json.Unmarshal(b, &out)
	return out
}
func (s *Service) update(ctx context.Context, fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.state)
	var next State
	_ = json.Unmarshal(b, &next)
	if e := fn(&next); e != nil {
		return e
	}
	if e := s.repo.Save(ctx, next); e != nil {
		return e
	}
	s.state = next
	return nil
}

var phonePattern = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

func NormalizePhone(p string) string {
	p = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(p))
	if strings.HasPrefix(p, "00") {
		p = "+" + p[2:]
	}
	if len(p) == 10 && strings.HasPrefix(p, "0") {
		p = "+40" + p[1:]
	}
	return p
}
func (s *Service) Customer(u User, id string) (Customer, error) {
	for _, c := range s.Snapshot().Customers {
		if c.ID == id && c.StoreID == u.StoreID {
			return c, nil
		}
	}
	return Customer{}, ErrNotFound
}
func audit(st *State, u User, cid, action, detail string) {
	st.Audit = append(st.Audit, Audit{ID(), cid, u.ID, action, time.Now().UTC(), detail})
}
func (s *Service) CreateCustomer(ctx context.Context, u User, name, phone string) (Customer, error) {
	name = strings.TrimSpace(name)
	phone = NormalizePhone(phone)
	if len(name) < 2 || len(name) > 120 || !phonePattern.MatchString(phone) {
		return Customer{}, ErrInvalid
	}
	c := Customer{ID(), name, phone, u.StoreID, "", "pool", []string{}, time.Now().UTC(), time.Now().UTC()}
	e := s.update(ctx, func(st *State) error {
		st.Customers = append(st.Customers, c)
		audit(st, u, c.ID, "customer.created", "Profil creat")
		return nil
	})
	return c, e
}

type VisitInput struct {
	Reason     string `json:"reason"`
	Steps      []int  `json:"steps"`
	Notes      string `json:"notes"`
	Ownership  string `json:"ownership"`
	NextAction string `json:"nextAction"`
	Due        string `json:"due"`
	Product    string `json:"product"`
}

func (s *Service) RecordVisit(ctx context.Context, u User, cid string, in VisitInput) error {
	if len(in.Notes) > 4000 || len(in.Reason) > 100 || len(in.NextAction) > 100 || len(in.Product) > 120 || len(in.Steps) == 0 {
		return ErrInvalid
	}
	seen := map[int]bool{}
	for _, n := range in.Steps {
		if n < 0 || n > 7 || seen[n] {
			return ErrInvalid
		}
		seen[n] = true
	}
	if in.Ownership != "" && in.Ownership != "keep" && in.Ownership != "owned" && in.Ownership != "pool" && in.Ownership != "unassigned" {
		return ErrInvalid
	}
	if in.NextAction != "" {
		if _, e := time.Parse("2006-01-02", in.Due); e != nil {
			return ErrInvalid
		}
	}
	return s.update(ctx, func(st *State) error {
		for i := range st.Customers {
			c := &st.Customers[i]
			if c.ID != cid || c.StoreID != u.StoreID {
				continue
			}
			if in.Ownership != "" && in.Ownership != "keep" {
				if e := changeOwnership(st, u, c, OwnershipInput{Ownership: in.Ownership}); e != nil {
					return e
				}
			}
			c.UpdatedAt = time.Now().UTC()
			sort.Ints(in.Steps)
			st.Visits = append(st.Visits, Visit{ID(), cid, u.ID, c.UpdatedAt, in.Reason, in.Steps, in.Notes})
			audit(st, u, cid, "visit.recorded", "Vizită înregistrată")
			opportunityID := ""
			if strings.TrimSpace(in.Product) != "" {
				opportunityID = ID()
				st.Opportunities = append(st.Opportunities, Opportunity{ID: opportunityID, CustomerID: cid, EmployeeID: u.ID, Product: strings.TrimSpace(in.Product), Stage: "identified", CreatedAt: c.UpdatedAt, UpdatedAt: c.UpdatedAt})
				audit(st, u, cid, "opportunity.created", in.Product)
			}
			if in.NextAction != "" {
				st.FollowUps = append(st.FollowUps, FollowUp{ID: ID(), CustomerID: cid, EmployeeID: u.ID, Type: in.NextAction, Due: in.Due, Status: "open", OpportunityID: opportunityID})
				audit(st, u, cid, "followup.created", in.NextAction)
			}

			return nil
		}
		return ErrNotFound
	})
}
func (s *Service) Complete(ctx context.Context, u User, id string) error {
	return s.UpdateFollowUp(ctx, u, id, FollowUpUpdate{Status: "done"})
}
func (s *Service) MoveOpportunity(ctx context.Context, u User, id, stage string) error {
	allowed := map[string]bool{"identified": true, "qualified": true, "verification": true, "presentation": true, "offer": true, "waiting": true, "won": true, "lost": true, "paused": true}
	if !allowed[stage] {
		return ErrInvalid
	}
	return s.update(ctx, func(st *State) error {
		for i := range st.Opportunities {
			o := &st.Opportunities[i]
			if o.ID != id {
				continue
			}
			found := false
			for _, c := range st.Customers {
				if c.ID == o.CustomerID && c.StoreID == u.StoreID {
					found = true
				}
			}
			if !found {
				return ErrNotFound
			}
			if o.EmployeeID != u.ID && u.Role != "manager" {
				return ErrForbidden
			}
			if o.Stage == "won" || o.Stage == "lost" {
				return ErrConflict
			}
			if o.Stage == stage {
				return nil
			}
			old := o.Stage
			o.Stage = stage
			o.UpdatedAt = time.Now().UTC()
			audit(st, u, o.CustomerID, "opportunity.stage_changed", old+" → "+stage)
			return nil
		}
		return ErrNotFound
	})
}
func Seed() State {
	now := time.Now().UTC()
	s := State{Users: []User{{"ioana", "Ioana Marinescu", "employee", "store-1"}, {"andrei", "Andrei Popescu", "employee", "store-1"}, {"elena", "Elena Dumitrescu", "manager", "store-1"}}, Customers: []Customer{}, Visits: []Visit{}, FollowUps: []FollowUp{}, Opportunities: []Opportunity{}, Audit: []Audit{}}
	names := []string{"Alexandru Ionescu", "Cristina Dumitru", "Mihai Popa", "Andreea Stan", "Radu Georgescu", "Diana Rusu", "Gabriel Munteanu", "Maria Dobre", "Vlad Petrescu"}
	products := []string{"Red Unlimited", "iPhone 16 Pro", "Internet acasă", "Reînnoire abonament", "Samsung Galaxy S25", "Internet + TV"}
	stages := []string{"offer", "verification", "presentation", "waiting", "qualified", "offer", "identified", "won", "presentation"}
	for i, n := range names {
		id := ID()
		owner := "ioana"
		ownership := "owned"
		if i == 6 {
			owner = ""
			ownership = "pool"
		}
		if i == 7 {
			owner = "andrei"
		}
		at := now.Add(-time.Duration(i*19+2) * time.Hour)
		s.Customers = append(s.Customers, Customer{id, n, "+40722000" + []string{"101", "102", "103", "104", "105", "106", "107", "108", "109"}[i], "store-1", owner, ownership, []string{products[i%6]}, at.AddDate(0, -2, 0), at})
		s.Visits = append(s.Visits, Visit{ID(), id, "ioana", at, "Reînnoire abonament", []int{0, 1, 3, 4}, "Am discutat despre opțiunile disponibile. Clientul dorește o ofertă adaptată nevoilor sale."})
		s.Opportunities = append(s.Opportunities, Opportunity{ID: ID(), CustomerID: id, EmployeeID: "ioana", Product: products[i%6], Stage: stages[i], CreatedAt: at, UpdatedAt: at})
		due := now.AddDate(0, 0, i-2).In(mustLocation()).Format("2006-01-02")
		s.FollowUps = append(s.FollowUps, FollowUp{ID: ID(), CustomerID: id, EmployeeID: "ioana", Type: []string{"Discută oferta", "Sună clientul", "Pregătește oferta"}[i%3], Due: due, Status: "open", OpportunityID: s.Opportunities[len(s.Opportunities)-1].ID})
	}
	return s
}
func mustLocation() *time.Location {
	l, e := time.LoadLocation("Europe/Bucharest")
	if e != nil {
		return time.UTC
	}
	return l
}
