// Package auth implements email/password authentication with server-side sessions.
// Only a SHA-256 digest of each session token is stored; passwords are hashed with bcrypt.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"vodafone/store/internal/apperr"
	"vodafone/store/internal/crm"
)

type Store interface {
	UserByEmail(ctx context.Context, email string) (crm.User, string, error)
	UserByID(ctx context.Context, id string) (crm.User, string, error)
	InsertUser(ctx context.Context, u crm.User, passwordHash string) error
	// SetPasswordHash replaces the hash and revokes the user's sessions except keepSession.
	SetPasswordHash(ctx context.Context, userID, passwordHash string, keepSession []byte) error
	InsertSession(ctx context.Context, tokenHash []byte, userID string, expires time.Time) error
	SessionUser(ctx context.Context, tokenHash []byte, now time.Time) (crm.User, time.Time, error)
	TouchSession(ctx context.Context, tokenHash []byte, now time.Time) error
	DeleteSession(ctx context.Context, tokenHash []byte) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error
	RecordLoginAttempt(ctx context.Context, email, ip string, success bool, at time.Time) error
	RecentFailures(ctx context.Context, email, ip string, since time.Time) (byEmail int, byIP int, err error)
}

type Config struct {
	SessionTTL time.Duration
	// Failed attempts allowed within ThrottleWindow before login is refused.
	MaxFailuresPerEmail int
	MaxFailuresPerIP    int
	ThrottleWindow      time.Duration
	BcryptCost          int
}

func DefaultConfig() Config {
	return Config{SessionTTL: 8 * time.Hour, MaxFailuresPerEmail: 5, MaxFailuresPerIP: 30, ThrottleWindow: 15 * time.Minute, BcryptCost: 12}
}

type Service struct {
	store Store
	cfg   Config
	now   func() time.Time
	dummy []byte
}

func NewService(store Store, cfg Config) *Service {
	// A fixed hash compared against for unknown emails, so response time does not reveal accounts.
	dummy, _ := bcrypt.GenerateFromPassword([]byte("unused-password-for-timing"), cfg.BcryptCost)
	return &Service{store: store, cfg: cfg, now: time.Now, dummy: dummy}
}

type Session struct {
	User    crm.User
	Token   string
	Expires time.Time
}

func NormalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func tokenHash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Login verifies credentials and opens a session. Unknown emails, wrong passwords and
// disabled accounts all return the same error.
func (s *Service) Login(ctx context.Context, email, password, clientIP string) (Session, error) {
	email = NormalizeEmail(email)
	now := s.now().UTC()
	byEmail, byIP, err := s.store.RecentFailures(ctx, email, clientIP, now.Add(-s.cfg.ThrottleWindow))
	if err != nil {
		return Session{}, err
	}
	if byEmail >= s.cfg.MaxFailuresPerEmail || byIP >= s.cfg.MaxFailuresPerIP {
		return Session{}, apperr.ErrRateLimited
	}
	u, hash, err := s.store.UserByEmail(ctx, email)
	if err != nil && !errors.Is(err, apperr.ErrNotFound) {
		return Session{}, err
	}
	if err != nil || len(password) > 72 {
		_ = bcrypt.CompareHashAndPassword(s.dummy, []byte(password))
		return Session{}, s.fail(ctx, email, clientIP, now)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || !u.Active {
		return Session{}, s.fail(ctx, email, clientIP, now)
	}
	if err := s.store.RecordLoginAttempt(ctx, email, clientIP, true, now); err != nil {
		return Session{}, err
	}
	_ = s.store.DeleteExpiredSessions(ctx, now)
	token := newToken()
	expires := now.Add(s.cfg.SessionTTL)
	if err := s.store.InsertSession(ctx, tokenHash(token), u.ID, expires); err != nil {
		return Session{}, err
	}
	return Session{User: u, Token: token, Expires: expires}, nil
}

func (s *Service) fail(ctx context.Context, email, ip string, now time.Time) error {
	if err := s.store.RecordLoginAttempt(ctx, email, ip, false, now); err != nil {
		return err
	}
	return apperr.ErrInvalidCredentials
}

// touchInterval limits how often last_seen_at is written for an active session.
const touchInterval = 5 * time.Minute

// Authenticate resolves a session token to its active user.
func (s *Service) Authenticate(ctx context.Context, token string) (crm.User, error) {
	if token == "" || len(token) > 128 {
		return crm.User{}, apperr.ErrUnauthorized
	}
	now := s.now().UTC()
	h := tokenHash(token)
	u, lastSeen, err := s.store.SessionUser(ctx, h, now)
	if errors.Is(err, apperr.ErrNotFound) {
		return crm.User{}, apperr.ErrUnauthorized
	}
	if err != nil {
		return crm.User{}, err
	}
	if !u.Active {
		return crm.User{}, apperr.ErrUnauthorized
	}
	if now.Sub(lastSeen) > touchInterval {
		_ = s.store.TouchSession(ctx, h, now)
	}
	return u, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, tokenHash(token))
}

func validatePassword(p string) error {
	switch {
	case utf8.RuneCountInString(p) < 10:
		return apperr.Validation(map[string]string{"password": "Parola trebuie să aibă cel puțin 10 caractere."})
	case len(p) > 72:
		return apperr.Validation(map[string]string{"password": "Parola poate avea cel mult 72 de octeți."})
	}
	return nil
}

func (s *Service) hash(p string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(p), s.cfg.BcryptCost)
	return string(h), err
}

// ChangePassword requires the current password and revokes the user's other sessions.
func (s *Service) ChangePassword(ctx context.Context, userID, currentToken, current, next string) error {
	_, hash, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if len(current) > 72 || bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
		return apperr.Validation(map[string]string{"currentPassword": "Parola actuală nu este corectă."})
	}
	if err := validatePassword(next); err != nil {
		return err
	}
	h, err := s.hash(next)
	if err != nil {
		return err
	}
	return s.store.SetPasswordHash(ctx, userID, h, tokenHash(currentToken))
}

type NewUser struct {
	StoreID  string
	Email    string
	Name     string
	Role     string
	Password string
}

// CreateUser is used by the administration CLI.
func (s *Service) CreateUser(ctx context.Context, in NewUser) (crm.User, error) {
	f := apperr.Fields{}
	in.Email = NormalizeEmail(in.Email)
	addr, err := mail.ParseAddress(in.Email)
	f.Check(err == nil && addr.Address == in.Email, "email", "Email invalid.")
	in.Name = strings.TrimSpace(in.Name)
	f.Check(in.Name != "" && utf8.RuneCountInString(in.Name) <= 120, "name", "Nume invalid.")
	f.Check(crm.ValidRole(in.Role), "role", "Rol invalid.")
	f.Check(crm.ValidID(in.StoreID), "storeId", "Magazin invalid.")
	if err := f.Err(); err != nil {
		return crm.User{}, err
	}
	if err := validatePassword(in.Password); err != nil {
		return crm.User{}, err
	}
	h, err := s.hash(in.Password)
	if err != nil {
		return crm.User{}, err
	}
	u := crm.User{ID: crm.NewID(), StoreID: in.StoreID, Email: in.Email, Name: in.Name, Role: in.Role, Active: true}
	return u, s.store.InsertUser(ctx, u, h)
}

// ResetPassword sets a new password without the current one and revokes all sessions (CLI only).
func (s *Service) ResetPassword(ctx context.Context, email, password string) error {
	u, _, err := s.store.UserByEmail(ctx, NormalizeEmail(email))
	if err != nil {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	h, err := s.hash(password)
	if err != nil {
		return err
	}
	return s.store.SetPasswordHash(ctx, u.ID, h, nil)
}
