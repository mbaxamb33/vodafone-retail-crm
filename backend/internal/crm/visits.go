package crm

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"vodafone/store/internal/apperr"
)

type OpportunityDraft struct {
	Product        string   `json:"product"`
	Category       string   `json:"category"`
	EstimatedValue *float64 `json:"estimatedValue"`
}

type VisitInput struct {
	ReasonCode string `json:"reasonCode"`
	Steps      []int  `json:"steps"`
	Notes      string `json:"notes"`
	// Ownership is keep (default), owned, pool or unassigned.
	Ownership     string             `json:"ownership"`
	NextAction    string             `json:"nextAction"`
	Due           string             `json:"due"`
	Opportunities []OpportunityDraft `json:"opportunities"`
}

type VisitResult struct {
	Visit         Visit         `json:"visit"`
	Opportunities []Opportunity `json:"opportunities"`
	FollowUp      *FollowUp     `json:"followUp"`
	Customer      Customer      `json:"customer"`
}

func validateVisit(in *VisitInput, catalog []CatalogItem) error {
	f := apperr.Fields{}
	in.Notes = cleanText(in.Notes)
	in.NextAction = cleanText(in.NextAction)
	f.Check(len(in.Steps) > 0, "steps", "Selectează cel puțin un pas parcurs.")
	seen := map[int]bool{}
	for _, n := range in.Steps {
		if n < 0 || n >= len(JourneySteps) || seen[n] {
			f.Add("steps", "Pașii selectați nu sunt valizi.")
		}
		seen[n] = true
	}
	if in.ReasonCode != "" {
		_, ok := catalogLabel(catalog, "visit_reason", in.ReasonCode)
		f.Check(ok, "reasonCode", "Alege un motiv din listă.")
	}
	f.Check(lengthBetween(in.Notes, 0, 4000), "notes", "Notele pot avea cel mult 4000 de caractere.")
	f.Check(lengthBetween(in.NextAction, 0, 100), "nextAction", "Următorul pas poate avea cel mult 100 de caractere.")
	if in.NextAction != "" {
		_, ok := ParseDate(in.Due)
		f.Check(ok, "due", "Alege data revenirii.")
	}
	switch in.Ownership {
	case "", "keep", OwnershipOwned, OwnershipPool, OwnershipUnassigned:
	default:
		f.Add("ownership", "Alege cine preia clientul.")
	}
	f.Check(len(in.Opportunities) <= 5, "opportunities", "Poți adăuga cel mult 5 oportunități la o vizită.")
	for i := range in.Opportunities {
		validateOpportunityDraft(&in.Opportunities[i], catalog, fmt.Sprintf("opportunities.%d.", i), f)
	}
	return f.Err()
}

func validateOpportunityDraft(d *OpportunityDraft, catalog []CatalogItem, prefix string, f apperr.Fields) {
	d.Product = cleanText(d.Product)
	f.Check(lengthBetween(d.Product, 1, 120), prefix+"product", "Descrie produsul în cel mult 120 de caractere.")
	if d.Category != "" {
		_, ok := catalogLabel(catalog, "product_category", d.Category)
		f.Check(ok, prefix+"category", "Alege o categorie din listă.")
	}
	if d.EstimatedValue != nil {
		f.Check(*d.EstimatedValue >= 0 && *d.EstimatedValue < 1e10, prefix+"estimatedValue", "Valoarea estimată nu este validă.")
	}
}

// RecordVisit saves a visit and, atomically, any ownership change, new opportunities and the
// next-action follow-up. Steps are stored exactly as selected; missing steps are never inferred.
func (s *Service) RecordVisit(ctx context.Context, actor User, customerID string, in VisitInput) (VisitResult, error) {
	catalog, err := s.store.Catalog(ctx, actor.StoreID)
	if err != nil {
		return VisitResult{}, err
	}
	if err := validateVisit(&in, catalog); err != nil {
		return VisitResult{}, err
	}
	if !ValidID(customerID) {
		return VisitResult{}, apperr.ErrCustomerNotFound
	}
	steps := slices.Clone(in.Steps)
	slices.Sort(steps)
	reason, _ := catalogLabel(catalog, "visit_reason", in.ReasonCode)
	var res VisitResult
	err = s.store.InTx(ctx, func(tx Tx) error {
		c, err := tx.LockCustomer(ctx, actor.StoreID, customerID)
		if err != nil {
			return notFoundAs(err, apperr.ErrCustomerNotFound)
		}
		if c.Status == CustomerAnonymized {
			return apperr.ErrConflict
		}
		if in.Ownership != "" && in.Ownership != "keep" {
			if err := s.changeOwnership(ctx, tx, actor, &c, OwnershipInput{Ownership: in.Ownership}); err != nil {
				return err
			}
		}
		now := s.clock()
		v := Visit{ID: NewID(), StoreID: actor.StoreID, CustomerID: c.ID, EmployeeID: actor.ID, At: now, ReasonCode: in.ReasonCode, Reason: reason, Steps: steps, FurthestStep: steps[len(steps)-1], Notes: in.Notes}
		if err := tx.InsertVisit(ctx, v); err != nil {
			return err
		}
		c.LastInteractionAt, c.UpdatedAt = &now, now
		if c.Status == CustomerArchived {
			c.Status = CustomerActive
		}
		if err := tx.UpdateCustomer(ctx, c); err != nil {
			return err
		}
		var notify []Notice
		if c.OwnerID != "" {
			notify = []Notice{{UserID: c.OwnerID, Kind: "customer_returned", Message: "Un client din portofoliul tău a revenit în magazin."}}
		}
		if err := s.emit(ctx, tx, actor, now, Event{Action: "visit.recorded", EntityType: "visit", EntityID: v.ID, CustomerID: c.ID, Detail: "Vizită înregistrată · " + JourneySteps[v.FurthestStep], Data: map[string]any{"steps": steps, "reasonCode": in.ReasonCode}, Notify: notify}); err != nil {
			return err
		}
		res = VisitResult{Visit: v, Opportunities: []Opportunity{}}
		for _, d := range in.Opportunities {
			o, err := s.insertOpportunity(ctx, tx, actor, c.ID, actor.ID, d, "", v.ID, now)
			if err != nil {
				return err
			}
			res.Opportunities = append(res.Opportunities, o)
		}
		if in.NextAction != "" {
			f := FollowUp{ID: NewID(), StoreID: actor.StoreID, CustomerID: c.ID, EmployeeID: actor.ID, SourceVisitID: v.ID, Type: in.NextAction, Due: in.Due, Status: FollowUpOpen, CreatedBy: actor.ID, CreatedAt: now, UpdatedAt: now}
			// A next action belongs to the visit's opportunity only when that link is unambiguous.
			if len(res.Opportunities) == 1 {
				f.OpportunityID = res.Opportunities[0].ID
			}
			if err := s.insertFollowUp(ctx, tx, actor, f, now); err != nil {
				return err
			}
			res.FollowUp = &f
		}
		res.Customer = c
		return nil
	})
	return res, err
}

func (s *Service) ListVisits(ctx context.Context, actor User, customerID string, offset, limit int) (Page[Visit], error) {
	if !ValidID(customerID) {
		return Page[Visit]{}, apperr.ErrCustomerNotFound
	}
	if _, err := s.store.Customer(ctx, actor.StoreID, customerID); err != nil {
		return Page[Visit]{}, notFoundAs(err, apperr.ErrCustomerNotFound)
	}
	offset, limit = clampPage(offset, limit, 20, 100)
	items, total, err := s.store.Visits(ctx, actor.StoreID, VisitFilter{CustomerID: customerID, Offset: offset, Limit: limit})
	return Page[Visit]{Items: items, Total: total, Offset: offset, Limit: limit}, err
}

// notesEditWindow limits how long authors may correct their own visit notes.
const notesEditWindow = 7 * 24 * time.Hour

// UpdateVisitNotes corrects a visit's notes. The previous text is kept as a revision and the
// edit is audited. Authors may edit within a week; managers at any time.
func (s *Service) UpdateVisitNotes(ctx context.Context, actor User, visitID, notes string) (Visit, error) {
	notes = strings.TrimSpace(notes)
	if !lengthBetween(notes, 0, 4000) {
		return Visit{}, apperr.Validation(map[string]string{"notes": "Notele pot avea cel mult 4000 de caractere."})
	}
	if !ValidID(visitID) {
		return Visit{}, apperr.ErrVisitNotFound
	}
	var out Visit
	err := s.store.InTx(ctx, func(tx Tx) error {
		v, err := tx.LockVisit(ctx, actor.StoreID, visitID)
		if err != nil {
			return notFoundAs(err, apperr.ErrVisitNotFound)
		}
		now := s.clock()
		if !actor.Can(PermEditAnyNotes) && (v.EmployeeID != actor.ID || now.Sub(v.At) > notesEditWindow) {
			return apperr.ErrForbidden
		}
		out = v
		if v.Notes == notes {
			return nil
		}
		previous := v.Notes
		v.Notes, v.NotesEditedAt = notes, &now
		if err := tx.UpdateVisitNotes(ctx, v, previous, actor.ID, NewID()); err != nil {
			return err
		}
		out = v
		return s.emit(ctx, tx, actor, now, Event{Action: "visit.notes_updated", EntityType: "visit", EntityID: v.ID, CustomerID: v.CustomerID, Detail: "Note de vizită corectate"})
	})
	return out, err
}
