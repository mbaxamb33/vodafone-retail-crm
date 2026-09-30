package crm

import (
	"context"
	"slices"

	"vodafone/store/internal/apperr"
)

type CustomerInput struct {
	Name  string   `json:"name"`
	Phone string   `json:"phone"`
	Tags  []string `json:"tags"`
}

func validateCustomer(name, phone *string, tags *[]string) error {
	f := apperr.Fields{}
	if name != nil {
		*name = cleanText(*name)
		// The phone number identifies the customer; a name is optional.
		f.Check(lengthBetween(*name, 0, 120), "name", "Numele poate avea cel mult 120 de caractere.")
	}
	if phone != nil {
		*phone = NormalizePhone(*phone)
		f.Check(ValidPhone(*phone), "phone", "Introdu un număr de telefon valid, de exemplu 0722 345 678.")
	}
	if tags != nil {
		clean, ok := validTags(*tags)
		f.Check(ok, "tags", "Folosește cel mult 10 etichete de până la 40 de caractere.")
		*tags = clean
	}
	return f.Err()
}

// CreateCustomer adds a customer to the actor's store pool. Shared phone numbers are allowed.
func (s *Service) CreateCustomer(ctx context.Context, actor User, in CustomerInput) (Customer, error) {
	if in.Tags == nil {
		in.Tags = []string{}
	}
	if err := validateCustomer(&in.Name, &in.Phone, &in.Tags); err != nil {
		return Customer{}, err
	}
	now := s.clock()
	c := Customer{ID: NewID(), StoreID: actor.StoreID, Name: in.Name, Phone: in.Phone, Status: CustomerActive, Ownership: OwnershipPool, Tags: in.Tags, CreatedBy: actor.ID, CreatedAt: now, UpdatedAt: now}
	err := s.store.InTx(ctx, func(tx Tx) error {
		if err := tx.InsertCustomer(ctx, c); err != nil {
			return err
		}
		return s.emit(ctx, tx, actor, now, Event{Action: "customer.created", EntityType: "customer", EntityID: c.ID, CustomerID: c.ID, Detail: "Profil creat"})
	})
	return c, err
}

type CustomerPatch struct {
	Name   *string   `json:"name"`
	Phone  *string   `json:"phone"`
	Tags   *[]string `json:"tags"`
	Status *string   `json:"status"`
}

func (s *Service) UpdateCustomer(ctx context.Context, actor User, id string, in CustomerPatch) (Customer, error) {
	if err := validateCustomer(in.Name, in.Phone, in.Tags); err != nil {
		return Customer{}, err
	}
	if in.Status != nil && *in.Status != CustomerActive && *in.Status != CustomerArchived {
		return Customer{}, apperr.Validation(map[string]string{"status": "Status invalid."})
	}
	var out Customer
	err := s.store.InTx(ctx, func(tx Tx) error {
		c, err := tx.LockCustomer(ctx, actor.StoreID, id)
		if err != nil {
			return notFoundAs(err, apperr.ErrCustomerNotFound)
		}
		if c.Status == CustomerAnonymized {
			return apperr.ErrConflict
		}
		if !actor.canEditCustomer(c) {
			return apperr.ErrForbidden
		}
		changed := []string{}
		if in.Name != nil && *in.Name != c.Name {
			c.Name = *in.Name
			changed = append(changed, "name")
		}
		if in.Phone != nil && *in.Phone != c.Phone {
			c.Phone = *in.Phone
			changed = append(changed, "phone")
		}
		if in.Tags != nil && !slices.Equal(*in.Tags, c.Tags) {
			c.Tags = *in.Tags
			changed = append(changed, "tags")
		}
		if in.Status != nil && *in.Status != c.Status {
			if err := actor.require(PermArchiveCustomer); err != nil {
				return err
			}
			c.Status = *in.Status
			changed = append(changed, "status")
		}
		out = c
		if len(changed) == 0 {
			return nil
		}
		now := s.clock()
		c.UpdatedAt = now
		out = c
		if err := tx.UpdateCustomer(ctx, c); err != nil {
			return err
		}
		// Changed field names only: the audit log should not duplicate personal data.
		return s.emit(ctx, tx, actor, now, Event{Action: "customer.updated", EntityType: "customer", EntityID: c.ID, CustomerID: c.ID, Detail: "Profil actualizat", Data: map[string]any{"fields": changed}})
	})
	return out, err
}

// AnonymizeCustomer irreversibly removes identifying data while keeping aggregate history.
func (s *Service) AnonymizeCustomer(ctx context.Context, actor User, id string) error {
	if err := actor.require(PermAnonymize); err != nil {
		return err
	}
	return s.store.InTx(ctx, func(tx Tx) error {
		c, err := tx.LockCustomer(ctx, actor.StoreID, id)
		if err != nil {
			return notFoundAs(err, apperr.ErrCustomerNotFound)
		}
		if c.Status == CustomerAnonymized {
			return nil
		}
		now := s.clock()
		if err := tx.AnonymizeCustomer(ctx, actor.StoreID, id, now); err != nil {
			return err
		}
		return s.emit(ctx, tx, actor, now, Event{Action: "customer.anonymized", EntityType: "customer", EntityID: id, CustomerID: id, Detail: "Date personale anonimizate"})
	})
}

func (s *Service) ListCustomers(ctx context.Context, actor User, f CustomerFilter) (Page[Customer], error) {
	f.Offset, f.Limit = clampPage(f.Offset, f.Limit, 24, 100)
	if f.ExactPhone != "" {
		if f.ExactPhone = NormalizePhone(f.ExactPhone); !ValidPhone(f.ExactPhone) {
			return Page[Customer]{}, apperr.Validation(map[string]string{"phone": "Număr de telefon invalid."})
		}
	}
	f.Query = cleanText(f.Query)
	f.Phone = PhoneSearchFragment(f.Query)
	f.Query = FoldName(f.Query)
	if f.OwnerID == "me" {
		f.OwnerID = actor.ID
	}
	if f.OwnerID != "" && !ValidID(f.OwnerID) {
		return Page[Customer]{}, apperr.Validation(map[string]string{"owner": "Responsabil invalid."})
	}
	if f.Ownership != "" && f.Ownership != OwnershipOwned && f.Ownership != OwnershipPool && f.Ownership != OwnershipUnassigned {
		return Page[Customer]{}, apperr.Validation(map[string]string{"ownership": "Filtru invalid."})
	}
	if f.Status != "" && f.Status != CustomerActive && f.Status != CustomerArchived {
		return Page[Customer]{}, apperr.Validation(map[string]string{"status": "Filtru invalid."})
	}
	switch f.Sort {
	case "", "recent", "name", "newest", "followup":
	default:
		return Page[Customer]{}, apperr.Validation(map[string]string{"sort": "Sortare invalidă."})
	}
	items, total, err := s.store.Customers(ctx, actor.StoreID, f)
	return Page[Customer]{Items: items, Total: total, Offset: f.Offset, Limit: f.Limit}, err
}

type Profile struct {
	Customer         Customer      `json:"customer"`
	LastVisit        *Visit        `json:"lastVisit"`
	Visits           []Visit       `json:"visits"`
	Total            int           `json:"total"`
	FollowUps        []FollowUp    `json:"followUps"`
	Opportunities    []Opportunity `json:"opportunities"`
	OwnershipHistory []Audit       `json:"ownershipHistory"`
	Audit            []Audit       `json:"audit"`
}

// CustomerProfile returns store-level relationship context. Audit history is restricted.
func (s *Service) CustomerProfile(ctx context.Context, actor User, id string, offset int) (Profile, error) {
	if !ValidID(id) {
		return Profile{}, apperr.ErrCustomerNotFound
	}
	c, err := s.store.Customer(ctx, actor.StoreID, id)
	if err != nil {
		return Profile{}, notFoundAs(err, apperr.ErrCustomerNotFound)
	}
	p := Profile{Customer: c, Audit: []Audit{}}
	offset, limit := clampPage(offset, 20, 20, 20)
	if p.Visits, p.Total, err = s.store.Visits(ctx, actor.StoreID, VisitFilter{CustomerID: id, Offset: offset, Limit: limit}); err != nil {
		return Profile{}, err
	}
	latest := p.Visits
	if offset > 0 {
		if latest, _, err = s.store.Visits(ctx, actor.StoreID, VisitFilter{CustomerID: id, Limit: 1}); err != nil {
			return Profile{}, err
		}
	}
	if len(latest) > 0 {
		v := latest[0]
		p.LastVisit = &v
	}
	if p.FollowUps, _, err = s.store.FollowUps(ctx, actor.StoreID, FollowUpFilter{CustomerID: id, Limit: 200}); err != nil {
		return Profile{}, err
	}
	if p.Opportunities, _, err = s.store.Opportunities(ctx, actor.StoreID, OpportunityFilter{CustomerID: id, Limit: 200}); err != nil {
		return Profile{}, err
	}
	if p.OwnershipHistory, err = s.store.Audit(ctx, actor.StoreID, AuditFilter{CustomerID: id, Actions: []string{"customer.owner_changed"}, Limit: 50}); err != nil {
		return Profile{}, err
	}
	if actor.Can(PermViewAudit) {
		if p.Audit, err = s.store.Audit(ctx, actor.StoreID, AuditFilter{CustomerID: id, Limit: 50}); err != nil {
			return Profile{}, err
		}
	}
	return p, nil
}

type OwnershipInput struct {
	Ownership string `json:"ownership"`
	OwnerID   string `json:"ownerId"`
}

func (s *Service) ChangeOwnership(ctx context.Context, actor User, id string, in OwnershipInput) (Customer, error) {
	var out Customer
	err := s.store.InTx(ctx, func(tx Tx) error {
		c, err := tx.LockCustomer(ctx, actor.StoreID, id)
		if err != nil {
			return notFoundAs(err, apperr.ErrCustomerNotFound)
		}
		if err := s.changeOwnership(ctx, tx, actor, &c, in); err != nil {
			return err
		}
		out = c
		return nil
	})
	return out, err
}

// changeOwnership applies an ownership change to a locked customer. Customers are claimed only
// from the store pool, by the signed-in user for themselves. Only the owner, or a user with
// PermReturnToPool, returns a customer to the pool. Nobody hands a customer directly to a
// colleague. It never creates a visit, changes the last interaction or moves existing tasks.
func (s *Service) changeOwnership(ctx context.Context, tx Tx, actor User, c *Customer, in OwnershipInput) error {
	if c.Status == CustomerAnonymized {
		return apperr.ErrConflict
	}
	switch in.Ownership {
	case OwnershipOwned:
		if in.OwnerID != "" && in.OwnerID != actor.ID {
			return apperr.ErrForbidden
		}
		if c.OwnerID == actor.ID {
			return nil
		}
		if c.OwnerID != "" {
			return apperr.ErrCustomerAlreadyOwned
		}
	case OwnershipPool:
		if in.OwnerID != "" {
			return apperr.Validation(map[string]string{"ownerId": "Un client din portofoliul magazinului nu are responsabil."})
		}
		if c.Ownership == OwnershipPool {
			return nil
		}
		if c.OwnerID != "" && c.OwnerID != actor.ID && !actor.Can(PermReturnToPool) {
			return apperr.ErrForbidden
		}
	default:
		return apperr.Validation(map[string]string{"ownership": "Poți prelua un client din portofoliul magazinului sau îl poți returna magazinului."})
	}
	owner := ""
	if in.Ownership == OwnershipOwned {
		owner = actor.ID
	}
	now := s.clock()
	previousOwner, previousOwnership := c.OwnerID, c.Ownership
	c.OwnerID, c.Ownership, c.UpdatedAt = owner, in.Ownership, now
	if err := tx.UpdateCustomer(ctx, *c); err != nil {
		return err
	}
	notify := []Notice{}
	if previousOwner != "" && previousOwner != owner {
		notify = append(notify, Notice{UserID: previousOwner, Kind: "customer_reassigned", Message: "Un client din portofoliul tău a fost returnat magazinului."})
	}
	return s.emit(ctx, tx, actor, now, Event{
		Action: "customer.owner_changed", EntityType: "customer", EntityID: c.ID, CustomerID: c.ID,
		Detail: ownershipDetail(in.Ownership),
		Data:   map[string]any{"fromOwnership": previousOwnership, "fromOwnerId": previousOwner, "toOwnership": in.Ownership, "toOwnerId": owner},
		Notify: notify,
	})
}

// ReleasePortfolio returns every customer owned by user to the store pool, for example when the
// account is disabled. Each change is audited with the user as actor.
func (s *Service) ReleasePortfolio(ctx context.Context, user User) (int, error) {
	released := 0
	err := s.store.InTx(ctx, func(tx Tx) error {
		owned, _, err := tx.Customers(ctx, user.StoreID, CustomerFilter{OwnerID: user.ID, Limit: 100000})
		if err != nil {
			return err
		}
		for _, c := range owned {
			locked, err := tx.LockCustomer(ctx, user.StoreID, c.ID)
			if err != nil {
				return err
			}
			if err := s.changeOwnership(ctx, tx, user, &locked, OwnershipInput{Ownership: OwnershipPool}); err != nil {
				return err
			}
			released++
		}
		return nil
	})
	return released, err
}

func ownershipDetail(ownership string) string {
	switch ownership {
	case OwnershipOwned:
		return "Client alocat unui responsabil"
	case OwnershipPool:
		return "Client trecut în portofoliul magazinului"
	default:
		return "Client lăsat fără urmărire activă"
	}
}
