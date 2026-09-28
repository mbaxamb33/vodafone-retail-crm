// Package crm holds the store CRM domain: entities, validation, authorization and the
// application services that keep related changes consistent.
package crm

import (
	"encoding/json"
	"time"
)

const (
	RoleEmployee = "employee"
	RoleManager  = "manager"

	OwnershipOwned      = "owned"
	OwnershipPool       = "pool"
	OwnershipUnassigned = "unassigned"

	CustomerActive     = "active"
	CustomerArchived   = "archived"
	CustomerAnonymized = "anonymized"

	FollowUpOpen        = "open"
	FollowUpWaiting     = "waiting"
	FollowUpUnreachable = "unreachable"
	FollowUpDone        = "done"

	StageIdentified   = "identified"
	StageQualified    = "qualified"
	StageVerification = "verification"
	StagePresentation = "presentation"
	StageOffer        = "offer"
	StageWaiting      = "waiting"
	StageWon          = "won"
	StageLost         = "lost"
	StagePaused       = "paused"
)

// Stages lists opportunity stages in display order.
var Stages = []string{StageIdentified, StageQualified, StageVerification, StagePresentation, StageOffer, StageWaiting, StageWon, StageLost, StagePaused}

// JourneySteps is the eight-step store conversation. Visits record step indices into this list.
var JourneySteps = []string{"Welcome", "Rezolvarea solicitării", "Small talk", "Atenție și permisiune", "Verificare", "Prezentare", "Ofertă", "Contractare"}

// CommercialStep is the first journey step that counts as a commercial conversation.
const CommercialStep = 3

type User struct {
	ID      string `json:"id"`
	StoreID string `json:"storeId"`
	Email   string `json:"email,omitempty"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Active  bool   `json:"-"`
}

type RetailStore struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

type Customer struct {
	ID                string     `json:"id"`
	StoreID           string     `json:"storeId"`
	Name              string     `json:"name"`
	Phone             string     `json:"phone"`
	Status            string     `json:"status"`
	Ownership         string     `json:"ownership"`
	OwnerID           string     `json:"ownerId"`
	Tags              []string   `json:"tags"`
	CreatedBy         string     `json:"createdBy"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	LastInteractionAt *time.Time `json:"lastInteractionAt"`
	// NextFollowUpDue is the earliest open follow-up date; filled by list and profile reads.
	NextFollowUpDue string `json:"nextFollowUpDue,omitempty"`
}

type Visit struct {
	ID            string     `json:"id"`
	StoreID       string     `json:"-"`
	CustomerID    string     `json:"customerId"`
	EmployeeID    string     `json:"employeeId"`
	At            time.Time  `json:"at"`
	ReasonCode    string     `json:"reasonCode"`
	Reason        string     `json:"reason"`
	Steps         []int      `json:"steps"`
	FurthestStep  int        `json:"furthestStep"`
	Notes         string     `json:"notes"`
	NotesEditedAt *time.Time `json:"notesEditedAt,omitempty"`
}

type FollowUp struct {
	ID            string     `json:"id"`
	StoreID       string     `json:"-"`
	CustomerID    string     `json:"customerId"`
	EmployeeID    string     `json:"employeeId"`
	OpportunityID string     `json:"opportunityId,omitempty"`
	SourceVisitID string     `json:"sourceVisitId,omitempty"`
	Type          string     `json:"type"`
	Due           string     `json:"due"`
	Status        string     `json:"status"`
	Notes         string     `json:"notes"`
	CreatedBy     string     `json:"createdBy"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

type Opportunity struct {
	ID             string     `json:"id"`
	StoreID        string     `json:"-"`
	CustomerID     string     `json:"customerId"`
	EmployeeID     string     `json:"employeeId"`
	SourceVisitID  string     `json:"sourceVisitId,omitempty"`
	Product        string     `json:"product"`
	Category       string     `json:"category"`
	Stage          string     `json:"stage"`
	EstimatedValue *float64   `json:"estimatedValue"`
	Notes          string     `json:"notes"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	StageChangedAt time.Time  `json:"stageChangedAt"`
	ClosedAt       *time.Time `json:"closedAt,omitempty"`
}

func (o Opportunity) Closed() bool { return o.Stage == StageWon || o.Stage == StageLost }

type StageEvent struct {
	ID            string    `json:"id"`
	StoreID       string    `json:"-"`
	OpportunityID string    `json:"opportunityId"`
	CustomerID    string    `json:"customerId"`
	ActorID       string    `json:"actorId"`
	FromStage     string    `json:"fromStage"`
	ToStage       string    `json:"toStage"`
	At            time.Time `json:"at"`
}

type Audit struct {
	ID         string          `json:"id"`
	StoreID    string          `json:"-"`
	CustomerID string          `json:"customerId,omitempty"`
	ActorID    string          `json:"actorId"`
	Action     string          `json:"action"`
	EntityType string          `json:"entityType"`
	EntityID   string          `json:"entityId"`
	Detail     string          `json:"detail"`
	Data       json.RawMessage `json:"data"`
	At         time.Time       `json:"at"`
}

type Notification struct {
	ID           string     `json:"id"`
	StoreID      string     `json:"-"`
	UserID       string     `json:"userId"`
	Kind         string     `json:"kind"`
	CustomerID   string     `json:"customerId,omitempty"`
	CustomerName string     `json:"customerName,omitempty"`
	EntityID     string     `json:"entityId,omitempty"`
	Message      string     `json:"message"`
	CreatedAt    time.Time  `json:"createdAt"`
	ReadAt       *time.Time `json:"readAt,omitempty"`
}

type CatalogItem struct {
	Kind  string `json:"kind"`
	Code  string `json:"code"`
	Label string `json:"label"`
}

type Page[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}
