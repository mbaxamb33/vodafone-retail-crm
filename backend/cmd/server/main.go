package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"vodafone/store/internal/crm"
)

type session struct {
	User    crm.User
	Expires time.Time
}
type app struct {
	s        *crm.Service
	mu       sync.Mutex
	sessions map[string]session
	attempts map[string][]time.Time
	demo     bool
	origin   string
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, e error) {
	status, code, message := 500, "INTERNAL_ERROR", "Solicitarea nu a putut fi procesată."
	switch {
	case errors.Is(e, crm.ErrInvalid):
		status, code, message = 422, "VALIDATION_FAILED", "Verifică datele introduse."
	case errors.Is(e, crm.ErrForbidden):
		status, code, message = 403, "FORBIDDEN", "Nu ai permisiunea necesară."
	case errors.Is(e, crm.ErrNotFound):
		status, code, message = 404, "NOT_FOUND", "Înregistrarea nu a fost găsită."
	case errors.Is(e, crm.ErrConflict):
		status, code, message = 409, "CONFLICT", "Înregistrarea este deja închisă."
	}
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		fail(w, crm.ErrInvalid)
		return false
	}
	return true
}
func (a *app) user(r *http.Request) (crm.User, bool) {
	c, e := r.Cookie("vf_session")
	if e != nil {
		return crm.User{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[c.Value]
	if !ok || time.Now().After(s.Expires) {
		delete(a.sessions, c.Value)
		return crm.User{}, false
	}
	return s.User, true
}
func (a *app) serve(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rid := crm.ID()
	w.Header().Set("X-Request-ID", rid)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	defer func() {
		slog.Info("request", "request_id", rid, "method", r.Method, "duration_ms", time.Since(start).Milliseconds())
	}()
	if r.URL.Path == "/health" || r.URL.Path == "/ready" {
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method != "GET" && r.Header.Get("Origin") != a.origin {
		fail(w, crm.ErrForbidden)
		return
	}
	if r.URL.Path == "/api/v1/auth/login" && r.Method == "POST" {
		if !a.demo {
			fail(w, crm.ErrForbidden)
			return
		}
		a.mu.Lock()
		key := r.RemoteAddr
		if i := strings.LastIndex(key, ":"); i >= 0 {
			key = key[:i]
		}
		recent := []time.Time{}
		for _, t := range a.attempts[key] {
			if time.Since(t) < time.Minute {
				recent = append(recent, t)
			}
		}
		a.attempts[key] = append(recent, time.Now())
		a.mu.Unlock()
		if len(recent) >= 20 {
			respond(w, 429, map[string]any{"error": map[string]string{"code": "RATE_LIMITED", "message": "Încearcă din nou peste un minut."}})
			return
		}
		var in struct {
			Role string `json:"role"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Role != "employee" && in.Role != "manager" {
			fail(w, crm.ErrInvalid)
			return
		}
		for _, u := range a.s.Snapshot().Users {
			if u.Role == in.Role {
				token := crm.ID() + crm.ID()
				a.mu.Lock()
				for k, v := range a.sessions {
					if time.Now().After(v.Expires) {
						delete(a.sessions, k)
					}
				}
				a.sessions[token] = session{u, time.Now().Add(8 * time.Hour)}
				a.mu.Unlock()
				http.SetCookie(w, &http.Cookie{Name: "vf_session", Value: token, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(a.origin, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: 28800})
				respond(w, 200, u)
				return
			}
		}
		fail(w, crm.ErrForbidden)
		return
	}
	u, ok := a.user(r)
	if !ok {
		respond(w, 401, map[string]any{"error": map[string]string{"code": "UNAUTHORIZED", "message": "Autentifică-te pentru a continua."}})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	parts := strings.Split(path, "/")
	if path == "auth/me" && r.Method == "GET" {
		respond(w, 200, u)
		return
	}
	if path == "auth/logout" && r.Method == "POST" {
		c, _ := r.Cookie("vf_session")
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "vf_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if path == "workspace" && r.Method == "GET" {
		st := a.s.Snapshot()
		out := crm.State{Users: []crm.User{}, Customers: []crm.Customer{}, Visits: []crm.Visit{}, FollowUps: []crm.FollowUp{}, Opportunities: []crm.Opportunity{}, Audit: []crm.Audit{}}
		ids := map[string]bool{}
		for _, v := range st.Users {
			if v.StoreID == u.StoreID {
				out.Users = append(out.Users, v)
			}
		}
		for _, c := range st.Customers {
			if c.StoreID == u.StoreID {
				out.Customers = append(out.Customers, c)
				ids[c.ID] = true
			}
		}
		for _, f := range st.FollowUps {
			if ids[f.CustomerID] && (f.EmployeeID == u.ID || u.Role == "manager") {
				out.FollowUps = append(out.FollowUps, f)
			}
		}
		for _, o := range st.Opportunities {
			if ids[o.CustomerID] && (o.EmployeeID == u.ID || u.Role == "manager") {
				out.Opportunities = append(out.Opportunities, o)
			}
		}
		respond(w, 200, out)
		return
	}
	if path == "customers" && r.Method == "POST" {
		var in struct {
			Name  string `json:"name"`
			Phone string `json:"phone"`
		}
		if !decode(w, r, &in) {
			return
		}
		c, e := a.s.CreateCustomer(r.Context(), u, in.Name, in.Phone)
		if e != nil {
			fail(w, e)
			return
		}
		respond(w, 201, c)
		return
	}
	if len(parts) == 2 && parts[0] == "customers" && r.Method == "GET" {
		c, e := a.s.Customer(u, parts[1])
		if e != nil {
			fail(w, e)
			return
		}
		st := a.s.Snapshot()
		visits := []crm.Visit{}
		audit := []crm.Audit{}
		for i := len(st.Visits) - 1; i >= 0; i-- {
			v := st.Visits[i]
			if v.CustomerID == c.ID {
				visits = append(visits, v)
			}
		}
		for i := len(st.Audit) - 1; i >= 0; i-- {
			v := st.Audit[i]
			if v.CustomerID == c.ID && u.Role == "manager" {
				audit = append(audit, v)
			}
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if offset < 0 {
			offset = 0
		}
		total := len(visits)
		if offset > total {
			offset = total
		}
		end := min(offset+20, total)
		followUps := []crm.FollowUp{}
		opportunities := []crm.Opportunity{}
		for _, f := range st.FollowUps {
			if f.CustomerID == c.ID {
				followUps = append(followUps, f)
			}
		}
		for _, o := range st.Opportunities {
			if o.CustomerID == c.ID {
				opportunities = append(opportunities, o)
			}
		}
		var lastVisit *crm.Visit
		if len(visits) > 0 {
			lastVisit = &visits[0]
		}
		respond(w, 200, map[string]any{"customer": c, "visits": visits[offset:end], "lastVisit": lastVisit, "total": total, "audit": audit[:min(50, len(audit))], "followUps": followUps, "opportunities": opportunities})
		return
	}
	if len(parts) == 3 && parts[0] == "customers" && parts[2] == "visits" && r.Method == "POST" {
		var in crm.VisitInput
		if !decode(w, r, &in) {
			return
		}
		if e := a.s.RecordVisit(r.Context(), u, parts[1], in); e != nil {
			fail(w, e)
			return
		}
		respond(w, 201, map[string]bool{"ok": true})
		return
	}
	if len(parts) == 3 && parts[0] == "customers" && parts[2] == "ownership" && r.Method == "POST" {
		var in crm.OwnershipInput
		if !decode(w, r, &in) {
			return
		}
		if e := a.s.ChangeOwnership(r.Context(), u, parts[1], in); e != nil {
			fail(w, e)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if path == "follow-ups" && r.Method == "POST" {
		var in crm.FollowUpInput
		if !decode(w, r, &in) {
			return
		}
		f, e := a.s.ScheduleFollowUp(r.Context(), u, in)
		if e != nil {
			fail(w, e)
			return
		}
		respond(w, 201, f)
		return
	}
	if len(parts) == 2 && parts[0] == "follow-ups" && r.Method == "PATCH" {
		var in crm.FollowUpUpdate
		if !decode(w, r, &in) {
			return
		}
		if in.Status == "" {
			in.Status = "done"
		}
		if e := a.s.UpdateFollowUp(r.Context(), u, parts[1], in); e != nil {
			fail(w, e)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if len(parts) == 2 && parts[0] == "opportunities" && r.Method == "PATCH" {
		var in struct {
			Stage string `json:"stage"`
		}
		if !decode(w, r, &in) {
			return
		}
		if e := a.s.MoveOpportunity(r.Context(), u, parts[1], in.Stage); e != nil {
			fail(w, e)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if path == "manager/dashboard" && r.Method == "GET" {
		out, e := a.s.Report(u, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
		if e != nil {
			fail(w, e)
			return
		}
		respond(w, 200, out)
		return
	}

	fail(w, crm.ErrNotFound)
}
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	demo := os.Getenv("DEMO_MODE") == "true"
	if !demo {
		slog.Error("Authentication provider not configured. DEMO_MODE=true is for local fictional data only.")
		os.Exit(1)
	}
	s, e := crm.New(context.Background(), crm.FileRepository{Path: env("DATA_PATH", "data/store.json")}, demo)
	if e != nil {
		slog.Error("Repository unavailable")
		os.Exit(1)
	}
	a := &app{s: s, sessions: map[string]session{}, attempts: map[string][]time.Time{}, demo: demo, origin: env("APP_ORIGIN", "http://127.0.0.1:5173")}
	srv := &http.Server{Addr: env("LISTEN_ADDR", "127.0.0.1:8080"), Handler: http.HandlerFunc(a.serve), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	slog.Info("CRM server starting", "address", srv.Addr, "demo", demo)
	if e = srv.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
		slog.Error("Server failed")
		os.Exit(1)
	}
}
