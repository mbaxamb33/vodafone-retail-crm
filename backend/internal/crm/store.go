package crm

import (
	"context"
	"time"
)

// Store is the persistence boundary. Every read is scoped to a store ID; implementations
// return apperr not-found errors for rows outside that store.
type Store interface {
	Reader
	// InTx runs fn in a transaction. Returning an error rolls back every write made through tx.
	InTx(ctx context.Context, fn func(tx Tx) error) error
}

type Reader interface {
	StoreByID(ctx context.Context, storeID string) (RetailStore, error)
	StoreUsers(ctx context.Context, storeID string) ([]User, error)
	Catalog(ctx context.Context, storeID string) ([]CatalogItem, error)

	Customer(ctx context.Context, storeID, id string) (Customer, error)
	Customers(ctx context.Context, storeID string, f CustomerFilter) ([]Customer, int, error)
	CustomersByID(ctx context.Context, storeID string, ids []string) ([]Customer, error)

	Visit(ctx context.Context, storeID, id string) (Visit, error)
	Visits(ctx context.Context, storeID string, f VisitFilter) ([]Visit, int, error)

	FollowUps(ctx context.Context, storeID string, f FollowUpFilter) ([]FollowUp, int, error)
	Opportunities(ctx context.Context, storeID string, f OpportunityFilter) ([]Opportunity, int, error)

	Audit(ctx context.Context, storeID string, f AuditFilter) ([]Audit, error)
	Notifications(ctx context.Context, storeID, userID string, unreadOnly bool, limit int) ([]Notification, int, error)

	ReportCounts(ctx context.Context, storeID string, r TimeRange, today string) (ReportCounts, error)
	EmployeeCounts(ctx context.Context, storeID string, r TimeRange, today string) ([]EmployeeCounts, error)
}

type Tx interface {
	Reader
	LockCustomer(ctx context.Context, storeID, id string) (Customer, error)
	LockVisit(ctx context.Context, storeID, id string) (Visit, error)
	LockFollowUp(ctx context.Context, storeID, id string) (FollowUp, error)
	LockOpportunity(ctx context.Context, storeID, id string) (Opportunity, error)

	InsertCustomer(ctx context.Context, c Customer) error
	UpdateCustomer(ctx context.Context, c Customer) error
	AnonymizeCustomer(ctx context.Context, storeID, id string, at time.Time) error
	InsertVisit(ctx context.Context, v Visit) error
	UpdateVisitNotes(ctx context.Context, v Visit, previous string, editorID string, revisionID string) error
	InsertOpportunity(ctx context.Context, o Opportunity) error
	UpdateOpportunity(ctx context.Context, o Opportunity) error
	InsertStageEvent(ctx context.Context, e StageEvent) error
	InsertFollowUp(ctx context.Context, f FollowUp) error
	UpdateFollowUp(ctx context.Context, f FollowUp) error
	InsertAudit(ctx context.Context, a Audit) error
	InsertNotification(ctx context.Context, n Notification) error
	MarkNotificationsRead(ctx context.Context, storeID, userID string, ids []string, at time.Time) error
}

type CustomerFilter struct {
	Query     string // name fragment or phone
	Phone     string // normalized phone fragment derived from Query
	OwnerID   string
	Ownership string
	Status    string // empty means active and archived, never anonymized unless requested
	Sort      string // recent, name, newest, followup
	Offset    int
	Limit     int
}

type VisitFilter struct {
	CustomerID string
	EmployeeID string
	Range      TimeRange
	Offset     int
	Limit      int
}

type FollowUpFilter struct {
	CustomerID           string
	EmployeeID           string
	OpportunityID        string
	Statuses             []string
	DueBefore            string // exclusive
	DueFrom              string // inclusive
	DueTo                string // inclusive
	CompletedFrom        *time.Time
	OpenOrCompletedSince *time.Time // open follow-ups plus those completed after this time
	Offset               int
	Limit                int
}

type OpportunityFilter struct {
	CustomerID  string
	EmployeeID  string
	Stages      []string
	ActiveOnly  bool
	ClosedSince *time.Time // with ActiveOnly false: active ones plus those closed after this time
	Offset      int
	Limit       int
}

type AuditFilter struct {
	CustomerID string
	ActorID    string
	Actions    []string
	Since      *time.Time
	Limit      int
}

// TimeRange is half-open [From, To). A zero range means all time.
type TimeRange struct {
	From time.Time
	To   time.Time
}

func (r TimeRange) All() bool { return r.From.IsZero() && r.To.IsZero() }

type ReportCounts struct {
	Visits               int
	CustomersHandled     int
	NewCustomers         int
	OpportunitiesCreated int
	Offers               int
	Won                  int
	Lost                 int
	FurthestAtLeast      [8]int // visits whose furthest step >= index
	StepIncidence        [8]int // visits that included step index
	PoolCustomers        int
	ActiveOpportunities  int
	FollowUpsDueToday    int
	OverdueFollowUps     int
}

type EmployeeCounts struct {
	EmployeeID           string
	Visits               int
	CustomersHandled     int
	NewCustomers         int
	OpportunitiesCreated int
	Offers               int
	Won                  int
	PortfolioCustomers   int
	ActiveOpportunities  int
	OpenFollowUps        int
	OverdueFollowUps     int
}
