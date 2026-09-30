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

type ResolutionInput struct {
	Type   string `json:"type"`
	Holder string `json:"holder"`
	Status string `json:"status"`
}

type ReminderInput struct {
	Due   string `json:"due"`
	Notes string `json:"notes"`
}

type VisitInput struct {
	ReasonCode string `json:"reasonCode"`
	Steps      []int  `json:"steps"`
	Notes      string `json:"notes"`
	// Ownership is keep (default) or owned, which claims a customer from the store pool.
	Ownership string `json:"ownership"`
	// NextAction is a next_action catalog code for the step agreed with the customer; empty means none.
	NextAction    string `json:"nextAction"`
	ActionDetails string `json:"actionDetails"`
	// AgreedDate records that a date was agreed with the customer; Due is then required.
	AgreedDate bool   `json:"agreedDate"`
	Due        string `json:"due"`
	// Resolution is required when the request-resolution step was performed, and only then.
	Resolution *ResolutionInput `json:"resolution"`
	// Reminder is an internal check for a request that is not yet resolved.
	Reminder      *ReminderInput     `json:"reminder"`
	Opportunities []OpportunityDraft `json:"opportunities"`
}

type VisitResult struct {
	Visit         Visit         `json:"visit"`
	Opportunities []Opportunity `json:"opportunities"`
	FollowUp      *FollowUp     `json:"followUp"`
	Reminder      *FollowUp     `json:"reminder"`
	Customer      Customer      `json:"customer"`
}

// ReminderType is the follow-up title for internal resolution reminders.
const ReminderType = "Verifică rezolvarea problemei"

func validateVisit(in *VisitInput, catalog []CatalogItem, today string) error {
	f := apperr.Fields{}
	in.Notes = cleanText(in.Notes)
	in.ActionDetails = cleanText(in.ActionDetails)
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

	if in.NextAction == "" {
		in.NextAction = NextActionNone
	}
	_, ok := catalogLabel(catalog, "next_action", in.NextAction)
	f.Check(ok, "nextAction", "Alege următorul pas din listă.")
	if in.NextAction == NextActionOther {
		f.Check(lengthBetween(in.ActionDetails, 1, 500), "actionDetails", "Descrie ce ați stabilit (cel mult 500 de caractere).")
	} else {
		in.ActionDetails = ""
	}
	if in.AgreedDate {
		d, ok := ParseDate(in.Due)
		f.Check(ok, "due", "Alege data stabilită cu clientul.")
		f.Check(!ok || d.Format(time.DateOnly) >= today, "due", "Data stabilită nu poate fi în trecut.")
	} else {
		in.Due = ""
	}

	performedResolution := seen[ResolutionStep]
	switch {
	case !performedResolution && in.Resolution != nil:
		f.Add("resolution", "Detaliile solicitării se completează doar când pasul a fost parcurs.")
	case performedResolution && in.Resolution == nil:
		f.Add("resolution", "Spune ce solicitare a avut clientul.")
	case in.Resolution != nil:
		r := in.Resolution
		switch r.Type {
		case "invoice":
			f.Check(r.Holder == "holder" || r.Holder == "other", "resolution.holder", "Alege dacă a venit titularul.")
			f.Check(r.Status == "", "resolution.status", "Încasarea facturii nu are stare de rezolvare.")
		case "other":
			f.Check(r.Status == "resolved" || r.Status == "unresolved" || r.Status == "pending", "resolution.status", "Alege starea solicitării.")
			f.Check(r.Holder == "", "resolution.holder", "Titularul se completează doar la încasarea facturii.")
		default:
			f.Add("resolution.type", "Alege tipul solicitării.")
		}
	}
	if in.Reminder != nil {
		open := in.Resolution != nil && in.Resolution.Type == "other" && (in.Resolution.Status == "unresolved" || in.Resolution.Status == "pending")
		f.Check(open, "reminder", "Reminderul se poate seta doar pentru o solicitare nerezolvată.")
		in.Reminder.Notes = cleanText(in.Reminder.Notes)
		d, ok := ParseDate(in.Reminder.Due)
		f.Check(ok && d.Format(time.DateOnly) >= today, "reminder.due", "Alege o dată de azi sau din viitor.")
		f.Check(lengthBetween(in.Reminder.Notes, 1, 500), "reminder.notes", "Spune ce trebuie verificat (cel mult 500 de caractere).")
	}

	switch in.Ownership {
	case "", "keep", OwnershipOwned:
	default:
		f.Add("ownership", "La vizită poți doar să preiei un client din portofoliul magazinului.")
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

// RecordVisit saves a visit and, atomically, a claim from the store pool, new opportunities, the
// follow-up for a date agreed with the customer and an internal reminder. Steps are stored exactly
// as selected; missing steps are never inferred. Without an agreed date no follow-up is created.
func (s *Service) RecordVisit(ctx context.Context, actor User, customerID string, in VisitInput) (VisitResult, error) {
	catalog, err := s.store.Catalog(ctx, actor.StoreID)
	if err != nil {
		return VisitResult{}, err
	}
	today, err := s.today(ctx, actor.StoreID)
	if err != nil {
		return VisitResult{}, err
	}
	if err := validateVisit(&in, catalog, today); err != nil {
		return VisitResult{}, err
	}
	if !ValidID(customerID) {
		return VisitResult{}, apperr.ErrCustomerNotFound
	}
	steps := slices.Clone(in.Steps)
	slices.Sort(steps)
	reason, _ := catalogLabel(catalog, "visit_reason", in.ReasonCode)
	actionLabel, _ := catalogLabel(catalog, "next_action", in.NextAction)
	details := VisitDetails{NextAction: in.NextAction, NextActionLabel: actionLabel, ActionDetails: in.ActionDetails,
		ContactConsent: in.NextAction == NextActionKeepInTouch, AgreedDate: in.AgreedDate, Due: in.Due}
	if in.Resolution != nil {
		details.Resolution = &Resolution{Type: in.Resolution.Type, Holder: in.Resolution.Holder, Status: in.Resolution.Status}
	}
	var res VisitResult
	err = s.store.InTx(ctx, func(tx Tx) error {
		c, err := tx.LockCustomer(ctx, actor.StoreID, customerID)
		if err != nil {
			return notFoundAs(err, apperr.ErrCustomerNotFound)
		}
		if c.Status == CustomerAnonymized {
			return apperr.ErrConflict
		}
		if in.Ownership == OwnershipOwned && c.OwnerID != actor.ID {
			if err := s.changeOwnership(ctx, tx, actor, &c, OwnershipInput{Ownership: OwnershipOwned}); err != nil {
				return err
			}
		}
		now := s.clock()
		v := Visit{ID: NewID(), StoreID: actor.StoreID, CustomerID: c.ID, EmployeeID: actor.ID, At: now, ReasonCode: in.ReasonCode, Reason: reason, Steps: steps,
			FurthestStep: steps[len(steps)-1], Notes: in.Notes, Details: details}
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
		data := map[string]any{"steps": steps, "reasonCode": in.ReasonCode, "nextAction": in.NextAction, "agreedDate": in.AgreedDate}
		if details.ContactConsent {
			// Consent is recorded with its author and time through this audit event.
			data["contactConsent"] = true
		}
		if err := s.emit(ctx, tx, actor, now, Event{Action: "visit.recorded", EntityType: "visit", EntityID: v.ID, CustomerID: c.ID, Detail: "Vizită înregistrată · " + JourneySteps[v.FurthestStep], Data: data, Notify: notify}); err != nil {
			return err
		}
		res = VisitResult{Visit: v, Opportunities: []Opportunity{}}
		nextStep := ""
		if in.NextAction != NextActionNone {
			nextStep = in.NextAction
		}
		for _, d := range in.Opportunities {
			o, err := s.insertOpportunity(ctx, tx, actor, c.ID, actor.ID, d, "", nextStep, v.ID, now)
			if err != nil {
				return err
			}
			res.Opportunities = append(res.Opportunities, o)
		}
		if in.AgreedDate {
			title := actionLabel
			switch in.NextAction {
			case NextActionNone:
				title = "Revenire stabilită cu clientul"
			case NextActionOther:
				title = truncate(in.ActionDetails, 100)
			}
			f := FollowUp{ID: NewID(), StoreID: actor.StoreID, CustomerID: c.ID, EmployeeID: actor.ID, SourceVisitID: v.ID, Kind: FollowUpAgreed, Type: title, Due: in.Due,
				Status: FollowUpOpen, CreatedBy: actor.ID, CreatedAt: now, UpdatedAt: now}
			// A next action belongs to the visit's opportunity only when that link is unambiguous.
			if len(res.Opportunities) == 1 {
				f.OpportunityID = res.Opportunities[0].ID
			}
			if err := s.insertFollowUp(ctx, tx, actor, f, now); err != nil {
				return err
			}
			res.FollowUp = &f
		}
		if in.Reminder != nil {
			r := FollowUp{ID: NewID(), StoreID: actor.StoreID, CustomerID: c.ID, EmployeeID: actor.ID, SourceVisitID: v.ID, Kind: FollowUpReminder, Type: ReminderType,
				Due: in.Reminder.Due, Status: FollowUpOpen, Notes: in.Reminder.Notes, CreatedBy: actor.ID, CreatedAt: now, UpdatedAt: now}
			if err := s.insertFollowUp(ctx, tx, actor, r, now); err != nil {
				return err
			}
			res.Reminder = &r
		}
		res.Customer = c
		return nil
	})
	return res, err
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
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
