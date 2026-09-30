package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"vodafone/store/internal/apperr"
	"vodafone/store/internal/auth"
	"vodafone/store/internal/postgres/pgtest"
)

func TestLoginSessionsAndThrottling(t *testing.T) {
	db := pgtest.New(t)
	a := pgtest.FastAuth(db)
	st := pgtest.NewStore(t, db, a, "Auth Store")
	ctx := context.Background()
	email := st.Ioana.Email

	if _, err := a.Login(ctx, "nobody@example.com", pgtest.Password, "10.0.0.1"); !errors.Is(err, apperr.ErrInvalidCredentials) {
		t.Fatal("unknown email must look like a wrong password", err)
	}
	s, err := a.Login(ctx, "  "+upper(email)+" ", pgtest.Password, "10.0.0.1")
	if err != nil {
		t.Fatal("email should be case-insensitive:", err)
	}
	u, err := a.Authenticate(ctx, s.Token)
	if err != nil || u.ID != st.Ioana.ID {
		t.Fatal("session lookup", err)
	}
	if _, err := a.Authenticate(ctx, s.Token+"x"); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatal("tampered token accepted")
	}

	// Five failures lock the email for the window, even with the right password afterwards.
	for i := 0; i < 5; i++ {
		if _, err := a.Login(ctx, email, "wrong-password", "10.0.0.2"); !errors.Is(err, apperr.ErrInvalidCredentials) {
			t.Fatal(err)
		}
	}
	if _, err := a.Login(ctx, email, pgtest.Password, "10.0.0.3"); !errors.Is(err, apperr.ErrRateLimited) {
		t.Fatal("throttle not applied:", err)
	}
	// Other accounts are unaffected.
	if _, err := a.Login(ctx, st.Andrei.Email, pgtest.Password, "10.0.0.3"); err != nil {
		t.Fatal(err)
	}

	// Changing the password keeps the current session and revokes the others.
	other, err := a.Login(ctx, st.Mgr.Email, pgtest.Password, "10.0.0.4")
	if err != nil {
		t.Fatal(err)
	}
	current, _ := a.Login(ctx, st.Mgr.Email, pgtest.Password, "10.0.0.4")
	if err := a.ChangePassword(ctx, st.Mgr.ID, current.Token, "wrong", "new-password-123"); !errors.Is(err, apperr.ErrValidation) {
		t.Fatal("wrong current password accepted")
	}
	if err := a.ChangePassword(ctx, st.Mgr.ID, current.Token, pgtest.Password, "short"); !errors.Is(err, apperr.ErrValidation) {
		t.Fatal("weak password accepted")
	}
	if err := a.ChangePassword(ctx, st.Mgr.ID, current.Token, pgtest.Password, "new-password-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(ctx, other.Token); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatal("other session survived a password change")
	}
	if _, err := a.Authenticate(ctx, current.Token); err != nil {
		t.Fatal("current session revoked", err)
	}

	if err := a.Logout(ctx, current.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(ctx, current.Token); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatal("logout did not revoke session")
	}

	// Disabled accounts cannot sign in and lose their sessions.
	s, _ = a.Login(ctx, st.Andrei.Email, pgtest.Password, "10.0.0.5")
	if err := db.SetUserActive(ctx, st.Andrei.Email, false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Authenticate(ctx, s.Token); !errors.Is(err, apperr.ErrUnauthorized) {
		t.Fatal("disabled account kept its session")
	}
	if _, err := a.Login(ctx, st.Andrei.Email, pgtest.Password, "10.0.0.5"); !errors.Is(err, apperr.ErrInvalidCredentials) {
		t.Fatal("disabled account signed in")
	}
	if _, err := a.CreateUser(ctx, newUser(st.ID, st.Ioana.Email)); !errors.Is(err, apperr.ErrEmailTaken) {
		t.Fatal("duplicate email", err)
	}
}

func upper(s string) string { return strings.ToUpper(s) }

func newUser(storeID, email string) auth.NewUser {
	return auth.NewUser{StoreID: storeID, Email: email, Name: "Duplicate", Role: "employee", Password: pgtest.Password}
}

func TestUsernameLogin(t *testing.T) {
	db := pgtest.New(t)
	a := pgtest.FastAuth(db)
	st := pgtest.NewStore(t, db, a, "Username Store")
	ctx := context.Background()
	u, err := a.CreateUser(ctx, auth.NewUser{StoreID: st.ID, Email: " Oana.Boboc ", Name: "Oana Boboc", Role: "employee", Password: pgtest.Password})
	if err != nil || u.Email != "oana.boboc" {
		t.Fatal("username account", err, u.Email)
	}
	if _, err := a.Login(ctx, "OANA.BOBOC", pgtest.Password, "10.0.0.9"); err != nil {
		t.Fatal("username login should be case-insensitive:", err)
	}
	for _, bad := range []string{"", ".oana", "oana boboc", "oana@", "ana/../x"} {
		if _, err := a.CreateUser(ctx, auth.NewUser{StoreID: st.ID, Email: bad, Name: "X", Role: "employee", Password: pgtest.Password}); !errors.Is(err, apperr.ErrValidation) {
			t.Errorf("accepted invalid login %q", bad)
		}
	}
}
