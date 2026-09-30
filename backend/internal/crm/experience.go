package crm

import (
	"context"
	"strconv"
	"strings"
	"time"

	"vodafone/store/internal/apperr"
)

// experienceWindow is how many past days of visits stay on an employee's experience list.
const experienceWindow = 7

// experienceRange returns the store-local days [from, to) whose visits produce experience tasks:
// from the last week (never before the feature started in the store) up to, but excluding, today.
func (s *Service) experienceRange(ctx context.Context, storeID string) (RetailStore, string, string, error) {
	st, err := s.store.StoreByID(ctx, storeID)
	if err != nil {
		return st, "", "", err
	}
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		loc = time.UTC
	}
	today := s.clock().In(loc)
	from := today.AddDate(0, 0, -experienceWindow).Format(time.DateOnly)
	if st.ExperienceSince > from {
		from = st.ExperienceSince
	}
	return st, from, today.Format(time.DateOnly), nil
}

// Experience lists the customers the signed-in employee served on previous days, so they can
// check how the visit went. Tasks are derived from visits; only their status is stored.
func (s *Service) Experience(ctx context.Context, actor User) ([]ExperienceTask, error) {
	st, from, to, err := s.experienceRange(ctx, actor.StoreID)
	if err != nil {
		return nil, err
	}
	return s.store.ExperienceTasks(ctx, actor.StoreID, actor.ID, st.Timezone, from, to)
}

// UpdateExperience sets a task to open, unreachable (retry later) or done. The ID is
// "customerID:day" and must refer to a task currently on the actor's list.
func (s *Service) UpdateExperience(ctx context.Context, actor User, id, status string) (ExperienceTask, error) {
	if status != ExperienceOpen && status != ExperienceUnreachable && status != ExperienceDone {
		return ExperienceTask{}, apperr.Validation(map[string]string{"status": "Stare invalidă."})
	}
	customerID, day, _ := strings.Cut(id, ":")
	if !ValidID(customerID) {
		return ExperienceTask{}, apperr.ErrNotFound
	}
	tasks, err := s.Experience(ctx, actor)
	if err != nil {
		return ExperienceTask{}, err
	}
	for _, t := range tasks {
		if t.CustomerID != customerID || t.Day != day {
			continue
		}
		if t.Status == status {
			return t, nil
		}
		t.Status = status
		now := s.clock()
		return t, s.store.InTx(ctx, func(tx Tx) error {
			if err := tx.SetExperienceStatus(ctx, actor.StoreID, t, now); err != nil {
				return err
			}
			return s.emit(ctx, tx, actor, now, Event{Action: "experience.updated", EntityType: "customer", EntityID: customerID, CustomerID: customerID,
				Detail: "Revenire de experiență · " + day, Data: map[string]any{"day": day, "status": status}})
		})
	}
	return ExperienceTask{}, apperr.ErrNotFound
}

// ensureExperienceNotice creates at most one notification per store-local day telling the user
// how many experience checks are waiting.
func (s *Service) ensureExperienceNotice(ctx context.Context, actor User) error {
	st, _, to, err := s.experienceRange(ctx, actor.StoreID)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		loc = time.UTC
	}
	dayStart, _ := time.ParseInLocation(time.DateOnly, to, loc)
	sent, err := s.store.NotifiedSince(ctx, actor.StoreID, actor.ID, "experience", dayStart)
	if err != nil || sent {
		return err
	}
	tasks, err := s.Experience(ctx, actor)
	if err != nil {
		return err
	}
	pending := 0
	for _, t := range tasks {
		if t.Status != ExperienceDone {
			pending++
		}
	}
	if pending == 0 {
		return nil
	}
	now := s.clock()
	return s.store.InTx(ctx, func(tx Tx) error {
		return tx.InsertNotification(ctx, Notification{ID: NewID(), StoreID: actor.StoreID, UserID: actor.ID, Kind: "experience", CreatedAt: now,
			Message: experienceMessage(pending)})
	})
}

func experienceMessage(n int) string {
	if n == 1 {
		return "Un client așteaptă verificarea experienței."
	}
	return strconv.Itoa(n) + " clienți așteaptă verificarea experienței."
}
