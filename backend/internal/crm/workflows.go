package crm

import (
	"context"
	"strings"
	"time"
)

type OwnershipInput struct {
	Ownership string `json:"ownership"`
	OwnerID   string `json:"ownerId"`
}

func customerInStore(st *State, u User, id string) (*Customer, error) {
	for i := range st.Customers {
		c := &st.Customers[i]
		if c.ID == id && c.StoreID == u.StoreID {
			return c, nil
		}
	}
	return nil, ErrNotFound
}
func employeeInStore(st *State, u User, id string) bool {
	for _, v := range st.Users {
		if v.ID == id && v.StoreID == u.StoreID {
			return true
		}
	}
	return false
}
func changeOwnership(st *State, u User, c *Customer, in OwnershipInput) error {
	if in.Ownership != "owned" && in.Ownership != "pool" && in.Ownership != "unassigned" {
		return ErrInvalid
	}
	owner := in.OwnerID
	if in.Ownership == "owned" {
		if owner == "" {
			owner = u.ID
		}
		if !employeeInStore(st, u, owner) {
			return ErrInvalid
		}
		if u.Role != "manager" && owner != u.ID {
			return ErrForbidden
		}
	} else if owner != "" {
		return ErrInvalid
	}
	if u.Role != "manager" && c.OwnerID != "" && c.OwnerID != u.ID {
		return ErrForbidden
	}
	if c.OwnerID == owner && c.Ownership == in.Ownership {
		return nil
	}
	audit(st, u, c.ID, "customer.owner_changed", c.Ownership+" / "+c.OwnerID+" → "+in.Ownership+" / "+owner)
	c.OwnerID = owner
	c.Ownership = in.Ownership
	// Ownership does not invent an interaction or transfer existing tasks.
	return nil
}
func (s *Service) ChangeOwnership(ctx context.Context, u User, id string, in OwnershipInput) error {
	return s.update(ctx, func(st *State) error {
		c, e := customerInStore(st, u, id)
		if e != nil {
			return e
		}
		return changeOwnership(st, u, c, in)
	})
}

type FollowUpInput struct {
	CustomerID    string `json:"customerId"`
	OpportunityID string `json:"opportunityId"`
	EmployeeID    string `json:"employeeId"`
	Type          string `json:"type"`
	Due           string `json:"due"`
}

func validDue(d string) bool { _, e := time.Parse("2006-01-02", d); return e == nil }
func (s *Service) ScheduleFollowUp(ctx context.Context, u User, in FollowUpInput) (FollowUp, error) {
	in.Type = strings.TrimSpace(in.Type)
	if in.Type == "" || len(in.Type) > 100 || !validDue(in.Due) {
		return FollowUp{}, ErrInvalid
	}
	if in.EmployeeID == "" {
		in.EmployeeID = u.ID
	}
	f := FollowUp{ID: ID(), CustomerID: in.CustomerID, OpportunityID: in.OpportunityID, EmployeeID: in.EmployeeID, Type: in.Type, Due: in.Due, Status: "open"}
	e := s.update(ctx, func(st *State) error {
		if _, e := customerInStore(st, u, in.CustomerID); e != nil {
			return e
		}
		if !employeeInStore(st, u, in.EmployeeID) {
			return ErrInvalid
		}
		if u.Role != "manager" && in.EmployeeID != u.ID {
			return ErrForbidden
		}
		if in.OpportunityID != "" {
			found := false
			for i := range st.Opportunities {
				o := &st.Opportunities[i]
				if o.ID != in.OpportunityID {
					continue
				}
				if o.CustomerID != in.CustomerID {
					return ErrInvalid
				}
				if o.EmployeeID != u.ID && u.Role != "manager" {
					return ErrForbidden
				}
				if o.Stage == "won" || o.Stage == "lost" {
					return ErrConflict
				}
				found = true
			}
			if !found {
				return ErrNotFound
			}
		}
		st.FollowUps = append(st.FollowUps, f)
		audit(st, u, in.CustomerID, "followup.created", in.Type+" · "+in.Due)
		return nil
	})
	return f, e
}

type FollowUpUpdate struct {
	Status string `json:"status"`
	Due    string `json:"due"`
}

func (s *Service) UpdateFollowUp(ctx context.Context, u User, id string, in FollowUpUpdate) error {
	if in.Status != "open" && in.Status != "waiting" && in.Status != "unreachable" && in.Status != "done" {
		return ErrInvalid
	}
	if in.Status != "done" && !validDue(in.Due) {
		return ErrInvalid
	}
	return s.update(ctx, func(st *State) error {
		for i := range st.FollowUps {
			f := &st.FollowUps[i]
			if f.ID != id {
				continue
			}
			if _, e := customerInStore(st, u, f.CustomerID); e != nil {
				return e
			}
			if f.EmployeeID != u.ID && u.Role != "manager" {
				return ErrForbidden
			}
			if f.Status == "done" {
				if in.Status == "done" {
					return nil
				}
				return ErrConflict
			}
			old := f.Status + " / " + f.Due
			f.Status = in.Status
			action := "followup.updated"
			if in.Status == "done" {
				now := time.Now().UTC()
				f.CompletedAt = &now
				action = "followup.completed"
			} else {
				f.Due = in.Due
			}
			audit(st, u, f.CustomerID, action, f.Type+": "+old+" → "+f.Status+" / "+f.Due)
			return nil
		}
		return ErrNotFound
	})
}

type StageEvent struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customerId"`
	EmployeeID string    `json:"employeeId"`
	Stage      string    `json:"stage"`
	At         time.Time `json:"at"`
}
type Report struct {
	Visits       []Visit      `json:"visits"`
	NewCustomers []Customer   `json:"newCustomers"`
	Events       []StageEvent `json:"events"`
	From         string       `json:"from"`
	To           string       `json:"to"`
}

// Date bounds use the store timezone; the upper bound is exclusive next midnight.
func (s *Service) Report(u User, from, to string) (Report, error) {
	out := Report{Visits: []Visit{}, NewCustomers: []Customer{}, Events: []StageEvent{}, From: from, To: to}
	if u.Role != "manager" {
		return out, ErrForbidden
	}
	var start, end time.Time
	var e error
	if (from == "") != (to == "") {
		return out, ErrInvalid
	}
	if from != "" {
		start, e = time.ParseInLocation("2006-01-02", from, mustLocation())
		if e != nil {
			return out, ErrInvalid
		}
		end, e = time.ParseInLocation("2006-01-02", to, mustLocation())
		if e != nil || end.Before(start) {
			return out, ErrInvalid
		}
		end = end.AddDate(0, 0, 1)
	}
	contains := func(at time.Time) bool { return from == "" || (!at.Before(start) && at.Before(end)) }
	st := s.Snapshot()
	ids := map[string]bool{}
	for _, c := range st.Customers {
		if c.StoreID == u.StoreID {
			ids[c.ID] = true
			if contains(c.CreatedAt) {
				out.NewCustomers = append(out.NewCustomers, c)
			}
		}
	}
	for _, v := range st.Visits {
		if ids[v.CustomerID] && contains(v.At) {
			out.Visits = append(out.Visits, v)
		}
	}
	for _, a := range st.Audit {
		if !ids[a.CustomerID] || a.Action != "opportunity.stage_changed" || !contains(a.At) {
			continue
		}
		parts := strings.Split(a.Detail, " → ")
		if len(parts) == 2 {
			out.Events = append(out.Events, StageEvent{a.ID, a.CustomerID, a.ActorID, parts[1], a.At})
		}
	}
	return out, nil
}
