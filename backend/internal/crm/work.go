package crm

import (
	"context"
	"time"

	"vodafone/store/internal/apperr"
)

// ---- Opportunities ----

type OpportunityInput struct {
	CustomerID string `json:"customerId"`
	EmployeeID string `json:"employeeId"`
	OpportunityDraft
	Notes string `json:"notes"`
}

func (s *Service) insertOpportunity(ctx context.Context, tx Tx, actor User, customerID, employeeID string, d OpportunityDraft, notes, nextStep, visitID string, now time.Time) (Opportunity, error) {
	o := Opportunity{ID: NewID(), StoreID: actor.StoreID, CustomerID: customerID, EmployeeID: employeeID, SourceVisitID: visitID, Product: d.Product, Category: d.Category, Stage: StageIdentified, NextStep: nextStep, EstimatedValue: d.EstimatedValue, Notes: notes, CreatedAt: now, UpdatedAt: now, StageChangedAt: now}
	if err := tx.InsertOpportunity(ctx, o); err != nil {
		return o, err
	}
	if err := tx.InsertStageEvent(ctx, StageEvent{ID: NewID(), StoreID: actor.StoreID, OpportunityID: o.ID, CustomerID: customerID, ActorID: actor.ID, ToStage: StageIdentified, At: now}); err != nil {
		return o, err
	}
	return o, s.emit(ctx, tx, actor, now, Event{Action: "opportunity.created", EntityType: "opportunity", EntityID: o.ID, CustomerID: customerID, Detail: "Oportunitate: " + o.Product, Data: map[string]any{"category": o.Category},
		Notify: []Notice{{UserID: employeeID, Kind: "opportunity_assigned", Message: "Ți-a fost atribuită o oportunitate."}}})
}

func (s *Service) CreateOpportunity(ctx context.Context, actor User, in OpportunityInput) (Opportunity, error) {
	catalog, err := s.store.Catalog(ctx, actor.StoreID)
	if err != nil {
		return Opportunity{}, err
	}
	f := apperr.Fields{}
	validateOpportunityDraft(&in.OpportunityDraft, catalog, "", f)
	in.Notes = cleanText(in.Notes)
	f.Check(lengthBetween(in.Notes, 0, 2000), "notes", "Notele pot avea cel mult 2000 de caractere.")
	if err := f.Err(); err != nil {
		return Opportunity{}, err
	}
	if in.EmployeeID == "" {
		in.EmployeeID = actor.ID
	}
	if !ValidID(in.CustomerID) {
		return Opportunity{}, apperr.ErrCustomerNotFound
	}
	if !ValidID(in.EmployeeID) {
		return Opportunity{}, apperr.ErrEmployeeNotFound
	}
	var out Opportunity
	err = s.store.InTx(ctx, func(tx Tx) error {
		c, err := tx.LockCustomer(ctx, actor.StoreID, in.CustomerID)
		if err != nil {
			return notFoundAs(err, apperr.ErrCustomerNotFound)
		}
		if c.Status == CustomerAnonymized {
			return apperr.ErrConflict
		}
		if _, err := s.member(ctx, tx, actor, in.EmployeeID); err != nil {
			return err
		}
		if in.EmployeeID != actor.ID && !actor.Can(PermAssignAnyone) {
			return apperr.ErrForbidden
		}
		out, err = s.insertOpportunity(ctx, tx, actor, c.ID, in.EmployeeID, in.OpportunityDraft, in.Notes, "", "", s.clock())
		return err
	})
	return out, err
}

type OpportunityPatch struct {
	Stage          *string  `json:"stage"`
	Product        *string  `json:"product"`
	Category       *string  `json:"category"`
	EstimatedValue *float64 `json:"estimatedValue"`
	Notes          *string  `json:"notes"`
}

// UpdateOpportunity changes stage or details. Won and lost are final. Repeating the current
// stage records nothing and does not reset the stage age.
func (s *Service) UpdateOpportunity(ctx context.Context, actor User, id string, in OpportunityPatch) (Opportunity, error) {
	f := apperr.Fields{}
	if in.Stage != nil {
		f.Check(validStage(*in.Stage), "stage", "Etapă invalidă.")
	}
	catalog, err := s.store.Catalog(ctx, actor.StoreID)
	if err != nil {
		return Opportunity{}, err
	}
	if in.Product != nil || in.Category != nil || in.EstimatedValue != nil {
		d := OpportunityDraft{Product: "placeholder", EstimatedValue: in.EstimatedValue}
		if in.Product != nil {
			d.Product = *in.Product
		}
		if in.Category != nil {
			d.Category = *in.Category
		}
		validateOpportunityDraft(&d, catalog, "", f)
		if in.Product != nil {
			*in.Product = d.Product
		}
	}
	if in.Notes != nil {
		*in.Notes = cleanText(*in.Notes)
		f.Check(lengthBetween(*in.Notes, 0, 2000), "notes", "Notele pot avea cel mult 2000 de caractere.")
	}
	if err := f.Err(); err != nil {
		return Opportunity{}, err
	}
	if !ValidID(id) {
		return Opportunity{}, apperr.ErrOpportunityNotFound
	}
	var out Opportunity
	err = s.store.InTx(ctx, func(tx Tx) error {
		o, err := tx.LockOpportunity(ctx, actor.StoreID, id)
		if err != nil {
			return notFoundAs(err, apperr.ErrOpportunityNotFound)
		}
		if !actor.canWork(o.EmployeeID) {
			return apperr.ErrForbidden
		}
		out = o
		now := s.clock()
		changed := false
		if in.Product != nil && *in.Product != o.Product || in.Category != nil && *in.Category != o.Category || in.EstimatedValue != nil {
			if o.Closed() {
				return apperr.ErrInvalidStageTransition
			}
			if in.Product != nil {
				o.Product = *in.Product
			}
			if in.Category != nil {
				o.Category = *in.Category
			}
			if in.EstimatedValue != nil {
				o.EstimatedValue = in.EstimatedValue
			}
			changed = true
		}
		if in.Notes != nil && *in.Notes != o.Notes {
			o.Notes = *in.Notes
			changed = true
		}
		stageChanged := in.Stage != nil && *in.Stage != o.Stage
		if stageChanged {
			if o.Closed() {
				return apperr.ErrInvalidStageTransition
			}
			from := o.Stage
			o.Stage, o.StageChangedAt = *in.Stage, now
			if o.Closed() {
				o.ClosedAt = &now
			}
			if err := tx.InsertStageEvent(ctx, StageEvent{ID: NewID(), StoreID: actor.StoreID, OpportunityID: o.ID, CustomerID: o.CustomerID, ActorID: actor.ID, FromStage: from, ToStage: o.Stage, At: now}); err != nil {
				return err
			}
			if err := s.emit(ctx, tx, actor, now, Event{Action: "opportunity.stage_changed", EntityType: "opportunity", EntityID: o.ID, CustomerID: o.CustomerID, Detail: o.Product + ": " + from + " → " + o.Stage, Data: map[string]any{"from": from, "to": o.Stage},
				Notify: []Notice{{UserID: o.EmployeeID, Kind: "opportunity_updated", Message: "Etapa unei oportunități a fost schimbată de un coleg."}}}); err != nil {
				return err
			}
		}
		if !changed && !stageChanged {
			return nil
		}
		o.UpdatedAt = now
		if err := tx.UpdateOpportunity(ctx, o); err != nil {
			return err
		}
		out = o
		if changed {
			return s.emit(ctx, tx, actor, now, Event{Action: "opportunity.updated", EntityType: "opportunity", EntityID: o.ID, CustomerID: o.CustomerID, Detail: "Detalii oportunitate actualizate: " + o.Product})
		}
		return nil
	})
	return out, err
}

type WorkQuery struct {
	Employee   string // "me" (default), "all" or a user ID
	CustomerID string
	Status     string // follow-ups: active (default), done, open, waiting, unreachable, all
	Due        string // follow-ups: overdue, today, upcoming
	Stage      string // opportunities: a stage, "active" (default) or "all"
	Offset     int
	Limit      int
}

// employeeScope resolves the Employee filter. Only users with PermViewStoreWork may list colleagues' work.
func employeeScope(actor User, employee string) (string, error) {
	switch employee {
	case "", "me":
		return actor.ID, nil
	case "all":
		if !actor.Can(PermViewStoreWork) {
			return "", apperr.ErrForbidden
		}
		return "", nil
	}
	if employee != actor.ID && !actor.Can(PermViewStoreWork) {
		return "", apperr.ErrForbidden
	}
	if !ValidID(employee) {
		return "", apperr.ErrEmployeeNotFound
	}
	return employee, nil
}

func (s *Service) ListOpportunities(ctx context.Context, actor User, q WorkQuery) (Page[Opportunity], error) {
	if q.CustomerID != "" && !ValidID(q.CustomerID) {
		return Page[Opportunity]{}, apperr.ErrCustomerNotFound
	}
	employee, err := employeeScope(actor, q.Employee)
	if err != nil {
		return Page[Opportunity]{}, err
	}
	f := OpportunityFilter{EmployeeID: employee, CustomerID: q.CustomerID}
	f.Offset, f.Limit = clampPage(q.Offset, q.Limit, 50, 200)
	switch q.Stage {
	case "", "active":
		f.ActiveOnly = true
	case "all":
	default:
		if !validStage(q.Stage) {
			return Page[Opportunity]{}, apperr.Validation(map[string]string{"stage": "Etapă invalidă."})
		}
		f.Stages = []string{q.Stage}
	}
	items, total, err := s.store.Opportunities(ctx, actor.StoreID, f)
	return Page[Opportunity]{Items: items, Total: total, Offset: f.Offset, Limit: f.Limit}, err
}

// ---- Follow-ups ----

type FollowUpInput struct {
	CustomerID    string `json:"customerId"`
	OpportunityID string `json:"opportunityId"`
	EmployeeID    string `json:"employeeId"`
	Type          string `json:"type"`
	Due           string `json:"due"`
	Notes         string `json:"notes"`
}

func (s *Service) insertFollowUp(ctx context.Context, tx Tx, actor User, f FollowUp, now time.Time) error {
	if err := tx.InsertFollowUp(ctx, f); err != nil {
		return err
	}
	return s.emit(ctx, tx, actor, now, Event{Action: "followup.created", EntityType: "follow_up", EntityID: f.ID, CustomerID: f.CustomerID, Detail: f.Type + " · " + f.Due, Data: map[string]any{"employeeId": f.EmployeeID, "due": f.Due, "opportunityId": f.OpportunityID},
		Notify: []Notice{{UserID: f.EmployeeID, Kind: "followup_assigned", Message: "Ți-a fost atribuit un follow-up."}}})
}

func (s *Service) ScheduleFollowUp(ctx context.Context, actor User, in FollowUpInput) (FollowUp, error) {
	fields := apperr.Fields{}
	in.Type, in.Notes = cleanText(in.Type), cleanText(in.Notes)
	fields.Check(lengthBetween(in.Type, 1, 100), "type", "Descrie pe scurt ce trebuie făcut (cel mult 100 de caractere).")
	_, ok := ParseDate(in.Due)
	fields.Check(ok, "due", "Alege data revenirii.")
	fields.Check(lengthBetween(in.Notes, 0, 2000), "notes", "Notele pot avea cel mult 2000 de caractere.")
	if err := fields.Err(); err != nil {
		return FollowUp{}, err
	}
	if in.EmployeeID == "" {
		in.EmployeeID = actor.ID
	}
	if !ValidID(in.CustomerID) {
		return FollowUp{}, apperr.ErrCustomerNotFound
	}
	if !ValidID(in.EmployeeID) {
		return FollowUp{}, apperr.ErrEmployeeNotFound
	}
	if in.OpportunityID != "" && !ValidID(in.OpportunityID) {
		return FollowUp{}, apperr.ErrOpportunityNotFound
	}
	now := s.clock()
	f := FollowUp{ID: NewID(), StoreID: actor.StoreID, CustomerID: in.CustomerID, EmployeeID: in.EmployeeID, OpportunityID: in.OpportunityID, Kind: FollowUpTask, Type: in.Type, Due: in.Due, Status: FollowUpOpen, Notes: in.Notes, CreatedBy: actor.ID, CreatedAt: now, UpdatedAt: now}
	err := s.store.InTx(ctx, func(tx Tx) error {
		c, err := tx.LockCustomer(ctx, actor.StoreID, in.CustomerID)
		if err != nil {
			return notFoundAs(err, apperr.ErrCustomerNotFound)
		}
		if c.Status == CustomerAnonymized {
			return apperr.ErrConflict
		}
		if _, err := s.member(ctx, tx, actor, in.EmployeeID); err != nil {
			return err
		}
		if in.EmployeeID != actor.ID && !actor.Can(PermAssignAnyone) {
			return apperr.ErrForbidden
		}
		if in.OpportunityID != "" {
			o, err := tx.LockOpportunity(ctx, actor.StoreID, in.OpportunityID)
			if err != nil {
				return notFoundAs(err, apperr.ErrOpportunityNotFound)
			}
			if o.CustomerID != c.ID {
				return apperr.Validation(map[string]string{"opportunityId": "Oportunitatea aparține altui client."})
			}
			if !actor.canWork(o.EmployeeID) {
				return apperr.ErrForbidden
			}
			if o.Closed() {
				return apperr.ErrInvalidStageTransition
			}
		}
		return s.insertFollowUp(ctx, tx, actor, f, now)
	})
	return f, err
}

type FollowUpUpdate struct {
	Status string  `json:"status"`
	Due    string  `json:"due"`
	Notes  *string `json:"notes"`
}

// UpdateFollowUp reschedules, records an outcome or completes a follow-up. Waiting and
// unreachable outcomes need a next check-in date. Completion keeps the due date and is final.
func (s *Service) UpdateFollowUp(ctx context.Context, actor User, id string, in FollowUpUpdate) (FollowUp, error) {
	fields := apperr.Fields{}
	switch in.Status {
	case FollowUpOpen, FollowUpWaiting, FollowUpUnreachable:
		_, ok := ParseDate(in.Due)
		fields.Check(ok, "due", "Alege data următoarei reveniri.")
	case FollowUpDone:
	default:
		fields.Add("status", "Alege rezultatul revenirii.")
	}
	if in.Notes != nil {
		*in.Notes = cleanText(*in.Notes)
		fields.Check(lengthBetween(*in.Notes, 0, 2000), "notes", "Notele pot avea cel mult 2000 de caractere.")
	}
	if err := fields.Err(); err != nil {
		return FollowUp{}, err
	}
	if !ValidID(id) {
		return FollowUp{}, apperr.ErrFollowUpNotFound
	}
	var out FollowUp
	err := s.store.InTx(ctx, func(tx Tx) error {
		f, err := tx.LockFollowUp(ctx, actor.StoreID, id)
		if err != nil {
			return notFoundAs(err, apperr.ErrFollowUpNotFound)
		}
		if !actor.canWork(f.EmployeeID) {
			return apperr.ErrForbidden
		}
		out = f
		if f.Status == FollowUpDone {
			if in.Status == FollowUpDone {
				return nil
			}
			return apperr.ErrConflict
		}
		now := s.clock()
		from := f.Status + " / " + f.Due
		f.Status, f.UpdatedAt = in.Status, now
		if in.Notes != nil {
			f.Notes = *in.Notes
		}
		action := "followup.updated"
		if in.Status == FollowUpDone {
			f.CompletedAt = &now
			action = "followup.completed"
		} else {
			f.Due = in.Due
		}
		if err := tx.UpdateFollowUp(ctx, f); err != nil {
			return err
		}
		out = f
		return s.emit(ctx, tx, actor, now, Event{Action: action, EntityType: "follow_up", EntityID: f.ID, CustomerID: f.CustomerID, Detail: f.Type + ": " + from + " → " + f.Status + " / " + f.Due, Data: map[string]any{"status": f.Status, "due": f.Due}})
	})
	return out, err
}

func (s *Service) ListFollowUps(ctx context.Context, actor User, q WorkQuery) (Page[FollowUp], error) {
	if q.CustomerID != "" && !ValidID(q.CustomerID) {
		return Page[FollowUp]{}, apperr.ErrCustomerNotFound
	}
	employee, err := employeeScope(actor, q.Employee)
	if err != nil {
		return Page[FollowUp]{}, err
	}
	today, err := s.today(ctx, actor.StoreID)
	if err != nil {
		return Page[FollowUp]{}, err
	}
	f := FollowUpFilter{EmployeeID: employee, CustomerID: q.CustomerID}
	f.Offset, f.Limit = clampPage(q.Offset, q.Limit, 50, 200)
	switch q.Status {
	case "", "active":
		f.Statuses = []string{FollowUpOpen, FollowUpWaiting, FollowUpUnreachable}
	case "all":
	case FollowUpOpen, FollowUpWaiting, FollowUpUnreachable, FollowUpDone:
		f.Statuses = []string{q.Status}
	default:
		return Page[FollowUp]{}, apperr.Validation(map[string]string{"status": "Filtru invalid."})
	}
	switch q.Due {
	case "":
	case "overdue":
		f.DueBefore = today
	case "today":
		f.DueFrom, f.DueTo = today, today
	case "upcoming":
		t, _ := ParseDate(today)
		f.DueFrom = t.AddDate(0, 0, 1).Format("2006-01-02")
	default:
		return Page[FollowUp]{}, apperr.Validation(map[string]string{"due": "Filtru invalid."})
	}
	items, total, err := s.store.FollowUps(ctx, actor.StoreID, f)
	return Page[FollowUp]{Items: items, Total: total, Offset: f.Offset, Limit: f.Limit}, err
}
