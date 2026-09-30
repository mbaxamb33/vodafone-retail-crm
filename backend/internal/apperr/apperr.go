// Package apperr defines classified application errors that are safe to show to API clients.
package apperr

import (
	"errors"
	"net/http"
)

type Error struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
}

func (e *Error) Error() string { return e.Code }

// Is makes errors.Is match on code, so callers can compare against the sentinels below.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

var (
	ErrUnauthorized           = New(http.StatusUnauthorized, "UNAUTHORIZED", "Autentifică-te pentru a continua.")
	ErrInvalidCredentials     = New(http.StatusUnauthorized, "INVALID_CREDENTIALS", "Emailul sau parola nu sunt corecte.")
	ErrForbidden              = New(http.StatusForbidden, "FORBIDDEN", "Nu ai permisiunea necesară.")
	ErrOriginRejected         = New(http.StatusForbidden, "ORIGIN_REJECTED", "Cererea nu provine din aplicație.")
	ErrNotFound               = New(http.StatusNotFound, "NOT_FOUND", "Înregistrarea nu a fost găsită.")
	ErrCustomerNotFound       = New(http.StatusNotFound, "CUSTOMER_NOT_FOUND", "Clientul nu a fost găsit.")
	ErrEmployeeNotFound       = New(http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "Colegul nu a fost găsit în acest magazin.")
	ErrFollowUpNotFound       = New(http.StatusNotFound, "FOLLOW_UP_NOT_FOUND", "Follow-up-ul nu a fost găsit.")
	ErrOpportunityNotFound    = New(http.StatusNotFound, "OPPORTUNITY_NOT_FOUND", "Oportunitatea nu a fost găsită.")
	ErrVisitNotFound          = New(http.StatusNotFound, "VISIT_NOT_FOUND", "Vizita nu a fost găsită.")
	ErrInvalidStageTransition = New(http.StatusConflict, "INVALID_STAGE_TRANSITION", "Oportunitatea este închisă și nu mai poate fi modificată.")
	ErrConflict               = New(http.StatusConflict, "CONFLICT", "Înregistrarea este deja închisă.")
	ErrCustomerAlreadyOwned   = New(http.StatusConflict, "CUSTOMER_ALREADY_OWNED", "Clientul are deja un responsabil. Îl poate returna magazinului doar acesta.")
	ErrEmailTaken             = New(http.StatusConflict, "EMAIL_TAKEN", "Există deja un cont cu acest email.")
	ErrPayloadTooLarge        = New(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Cererea este prea mare.")
	ErrRateLimited            = New(http.StatusTooManyRequests, "RATE_LIMITED", "Prea multe încercări. Încearcă din nou peste câteva minute.")
	ErrInternal               = New(http.StatusInternalServerError, "INTERNAL_ERROR", "Solicitarea nu a putut fi procesată.")
	ErrUnavailable            = New(http.StatusServiceUnavailable, "UNAVAILABLE", "Serviciul nu este disponibil momentan.")
)

// Validation returns a VALIDATION_FAILED error carrying per-field messages.
func Validation(fields map[string]string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "VALIDATION_FAILED", Message: "Verifică datele introduse.", Fields: fields}
}

// ErrValidation matches any validation error via errors.Is.
var ErrValidation = Validation(nil)

// Fields accumulates field errors; the first message per field wins.
type Fields map[string]string

func (f Fields) Add(field, message string) {
	if _, ok := f[field]; !ok {
		f[field] = message
	}
}
func (f Fields) Check(ok bool, field, message string) {
	if !ok {
		f.Add(field, message)
	}
}
func (f Fields) Err() error {
	if len(f) == 0 {
		return nil
	}
	return Validation(f)
}

// From classifies any error; unknown errors become INTERNAL_ERROR.
func From(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return ErrInternal
}
