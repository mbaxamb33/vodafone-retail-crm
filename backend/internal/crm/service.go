package crm

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"vodafone/store/internal/apperr"
)

type Service struct {
	store Store
	now   func() time.Time
}

type Option func(*Service)

// WithClock replaces the wall clock; tests use it for deterministic dates.
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

func NewService(store Store, opts ...Option) *Service {
	s := &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// location returns the store's time zone, used for calendar dates and report boundaries.
func (s *Service) location(ctx context.Context, storeID string) (*time.Location, error) {
	st, err := s.store.StoreByID(ctx, storeID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		return time.UTC, nil
	}
	return loc, nil
}

func (s *Service) today(ctx context.Context, storeID string) (string, error) {
	loc, err := s.location(ctx, storeID)
	if err != nil {
		return "", err
	}
	return s.clock().In(loc).Format(time.DateOnly), nil
}

// Event is a business action recorded in the audit log. Notifications for affected colleagues
// are derived from it inside the same transaction, so later integrations can hook in here.
type Event struct {
	Action     string
	EntityType string
	EntityID   string
	CustomerID string
	Detail     string
	Data       map[string]any
	// Notify lists users who should receive an in-app notification, with the kind to use.
	Notify []Notice
}

type Notice struct {
	UserID  string
	Kind    string
	Message string
}

func (s *Service) emit(ctx context.Context, tx Tx, actor User, at time.Time, ev Event) error {
	data := []byte("{}")
	if ev.Data != nil {
		var err error
		if data, err = json.Marshal(ev.Data); err != nil {
			return err
		}
	}
	if err := tx.InsertAudit(ctx, Audit{ID: NewID(), StoreID: actor.StoreID, CustomerID: ev.CustomerID, ActorID: actor.ID, Action: ev.Action, EntityType: ev.EntityType, EntityID: ev.EntityID, Detail: ev.Detail, Data: data, At: at}); err != nil {
		return err
	}
	for _, n := range ev.Notify {
		if n.UserID == "" || n.UserID == actor.ID {
			continue
		}
		if err := tx.InsertNotification(ctx, Notification{ID: NewID(), StoreID: actor.StoreID, UserID: n.UserID, Kind: n.Kind, CustomerID: ev.CustomerID, EntityID: ev.EntityID, Message: n.Message, CreatedAt: at}); err != nil {
			return err
		}
	}
	return nil
}

// member checks that id is an active user of the actor's store.
func (s *Service) member(ctx context.Context, r Reader, actor User, id string) (User, error) {
	users, err := r.StoreUsers(ctx, actor.StoreID)
	if err != nil {
		return User{}, err
	}
	for _, u := range users {
		if u.ID == id && u.Active {
			return u, nil
		}
	}
	return User{}, apperr.ErrEmployeeNotFound
}

func (s *Service) Users(ctx context.Context, actor User) ([]User, error) {
	users, err := s.store.StoreUsers(ctx, actor.StoreID)
	if err != nil {
		return nil, err
	}
	out := users[:0]
	for _, u := range users {
		if u.Active {
			u.Email = ""
			out = append(out, u)
		}
	}
	return out, nil
}

func (s *Service) Catalog(ctx context.Context, actor User) ([]CatalogItem, error) {
	return s.store.Catalog(ctx, actor.StoreID)
}

func (s *Service) StoreInfo(ctx context.Context, actor User) (RetailStore, error) {
	return s.store.StoreByID(ctx, actor.StoreID)
}

func catalogLabel(items []CatalogItem, kind, code string) (string, bool) {
	for _, it := range items {
		if it.Kind == kind && it.Code == code {
			return it.Label, true
		}
	}
	return "", false
}

// notFoundAs maps a generic not-found from the store to a specific error code.
func notFoundAs(err error, specific *apperr.Error) error {
	if errors.Is(err, apperr.ErrNotFound) {
		return specific
	}
	return err
}
