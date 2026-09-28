// Package httpapi exposes the CRM over a versioned JSON HTTP API.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vodafone/store/internal/apperr"
	"vodafone/store/internal/auth"
	"vodafone/store/internal/crm"
)

const sessionCookie = "vf_session"

type Config struct {
	AppOrigin  string
	SessionTTL time.Duration
	Features   []string
	// StaticDir, when set, serves the built frontend with index.html as the SPA fallback.
	StaticDir string
	// TrustProxy takes the client IP from the last X-Forwarded-For entry, which the
	// reverse proxy in front of the server sets. Enable only behind such a proxy.
	TrustProxy bool
}

type Server struct {
	crm   *crm.Service
	auth  *auth.Service
	cfg   Config
	log   *slog.Logger
	ready func(context.Context) error
}

func New(c *crm.Service, a *auth.Service, cfg Config, log *slog.Logger, ready func(context.Context) error) *Server {
	if cfg.Features == nil {
		cfg.Features = []string{}
	}
	return &Server{crm: c, auth: a, cfg: cfg, log: log, ready: ready}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /ready", s.readiness)

	api := func(pattern string, h http.HandlerFunc) {
		method, path, _ := strings.Cut(pattern, " ")
		mux.Handle(method+" /api/v1"+path, s.sameOrigin(s.authenticated(h)))
	}
	mux.Handle("POST /api/v1/auth/login", s.sameOrigin(http.HandlerFunc(s.login)))
	api("POST /auth/logout", s.logout)
	api("GET /auth/me", s.me)
	api("POST /auth/password", s.changePassword)

	api("GET /catalog", s.catalog)
	api("GET /users", s.users)
	api("GET /workspace", s.workspace)
	api("GET /employees/me/dashboard", s.employeeDashboard)

	api("GET /customers", s.listCustomers)
	api("POST /customers", s.createCustomer)
	api("GET /customers/{id}", s.customerProfile)
	api("PATCH /customers/{id}", s.updateCustomer)
	api("POST /customers/{id}/anonymize", s.anonymizeCustomer)
	api("POST /customers/{id}/ownership", s.changeOwnership)
	api("GET /customers/{id}/visits", s.listVisits)
	api("POST /customers/{id}/visits", s.recordVisit)
	api("PATCH /visits/{id}", s.updateVisitNotes)

	api("GET /follow-ups", s.listFollowUps)
	api("POST /follow-ups", s.scheduleFollowUp)
	api("PATCH /follow-ups/{id}", s.updateFollowUp)

	api("GET /opportunities", s.listOpportunities)
	api("POST /opportunities", s.createOpportunity)
	api("PATCH /opportunities/{id}", s.updateOpportunity)

	api("GET /manager/dashboard", s.managerDashboard)
	api("GET /manager/employees/{id}/activity", s.employeeActivity)

	api("GET /notifications", s.notifications)
	api("POST /notifications/read", s.readNotifications)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, r, apperr.ErrNotFound) })
	if s.cfg.StaticDir != "" {
		mux.Handle("/", staticFiles(s.cfg.StaticDir))
	}
	return s.observe(securityHeaders(mux))
}

func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.ready(ctx); err != nil {
		writeError(w, r, apperr.ErrUnavailable)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func queryInt(r *http.Request, key string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(key))
	return n
}

// respond writes v or the classified error.
func respond(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, status, v)
}

var ok = map[string]bool{"ok": true}

// ---- Authentication ----

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.cfg.AppOrigin, "https://"),
		SameSite: http.SameSiteStrictMode, MaxAge: int(s.cfg.SessionTTL.Seconds())})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.cfg.AppOrigin, "https://"),
		SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

type meResponse struct {
	User     crm.User        `json:"user"`
	Store    crm.RetailStore `json:"store"`
	Features []string        `json:"features"`
}

func (s *Server) meFor(ctx context.Context, u crm.User) (meResponse, error) {
	st, err := s.crm.StoreInfo(ctx, u)
	return meResponse{User: u, Store: st, Features: s.cfg.Features}, err
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	session, err := s.auth.Login(r.Context(), in.Email, in.Password, s.clientIP(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	if info, ok := r.Context().Value(requestInfoKey).(*requestInfo); ok {
		info.UserID, info.StoreID = session.User.ID, session.User.StoreID
	}
	me, err := s.meFor(r.Context(), session.User)
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.setSessionCookie(w, session.Token)
	writeJSON(w, 200, me)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(sessionCookie)
	if err := s.auth.Logout(r.Context(), c.Value); err != nil {
		writeError(w, r, err)
		return
	}
	s.clearSessionCookie(w)
	writeJSON(w, 200, ok)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	me, err := s.meFor(r.Context(), currentUser(r))
	respond(w, r, 200, me, err)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, _ := r.Cookie(sessionCookie)
	err := s.auth.ChangePassword(r.Context(), currentUser(r).ID, c.Value, in.CurrentPassword, in.NewPassword)
	respond(w, r, 200, ok, err)
}

// ---- Reference data ----

func (s *Server) catalog(w http.ResponseWriter, r *http.Request) {
	items, err := s.crm.Catalog(r.Context(), currentUser(r))
	out := map[string]any{"visitReasons": []crm.CatalogItem{}, "nextActions": []crm.CatalogItem{}, "productCategories": []crm.CatalogItem{}, "journeySteps": crm.JourneySteps, "stages": crm.Stages}
	for _, it := range items {
		key := map[string]string{"visit_reason": "visitReasons", "next_action": "nextActions", "product_category": "productCategories"}[it.Kind]
		out[key] = append(out[key].([]crm.CatalogItem), it)
	}
	respond(w, r, 200, out, err)
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	users, err := s.crm.Users(r.Context(), currentUser(r))
	respond(w, r, 200, users, err)
}

func (s *Server) workspace(w http.ResponseWriter, r *http.Request) {
	ws, err := s.crm.Workspace(r.Context(), currentUser(r))
	respond(w, r, 200, ws, err)
}

func (s *Server) employeeDashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.crm.EmployeeDashboard(r.Context(), currentUser(r))
	respond(w, r, 200, d, err)
}

// ---- Customers and visits ----

func (s *Server) listCustomers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := s.crm.ListCustomers(r.Context(), currentUser(r), crm.CustomerFilter{Query: q.Get("q"), OwnerID: q.Get("owner"), Ownership: q.Get("ownership"),
		Status: q.Get("status"), Sort: q.Get("sort"), Offset: queryInt(r, "offset"), Limit: queryInt(r, "limit")})
	respond(w, r, 200, page, err)
}

func (s *Server) createCustomer(w http.ResponseWriter, r *http.Request) {
	var in crm.CustomerInput
	if !decode(w, r, &in) {
		return
	}
	c, err := s.crm.CreateCustomer(r.Context(), currentUser(r), in)
	respond(w, r, 201, c, err)
}

func (s *Server) customerProfile(w http.ResponseWriter, r *http.Request) {
	p, err := s.crm.CustomerProfile(r.Context(), currentUser(r), r.PathValue("id"), queryInt(r, "offset"))
	respond(w, r, 200, p, err)
}

func (s *Server) updateCustomer(w http.ResponseWriter, r *http.Request) {
	var in crm.CustomerPatch
	if !decode(w, r, &in) {
		return
	}
	if !crm.ValidID(r.PathValue("id")) {
		writeError(w, r, apperr.ErrCustomerNotFound)
		return
	}
	c, err := s.crm.UpdateCustomer(r.Context(), currentUser(r), r.PathValue("id"), in)
	respond(w, r, 200, c, err)
}

func (s *Server) anonymizeCustomer(w http.ResponseWriter, r *http.Request) {
	if !crm.ValidID(r.PathValue("id")) {
		writeError(w, r, apperr.ErrCustomerNotFound)
		return
	}
	respond(w, r, 200, ok, s.crm.AnonymizeCustomer(r.Context(), currentUser(r), r.PathValue("id")))
}

func (s *Server) changeOwnership(w http.ResponseWriter, r *http.Request) {
	var in crm.OwnershipInput
	if !decode(w, r, &in) {
		return
	}
	if !crm.ValidID(r.PathValue("id")) {
		writeError(w, r, apperr.ErrCustomerNotFound)
		return
	}
	c, err := s.crm.ChangeOwnership(r.Context(), currentUser(r), r.PathValue("id"), in)
	respond(w, r, 200, c, err)
}

func (s *Server) listVisits(w http.ResponseWriter, r *http.Request) {
	page, err := s.crm.ListVisits(r.Context(), currentUser(r), r.PathValue("id"), queryInt(r, "offset"), queryInt(r, "limit"))
	respond(w, r, 200, page, err)
}

func (s *Server) recordVisit(w http.ResponseWriter, r *http.Request) {
	var in crm.VisitInput
	if !decode(w, r, &in) {
		return
	}
	res, err := s.crm.RecordVisit(r.Context(), currentUser(r), r.PathValue("id"), in)
	respond(w, r, 201, res, err)
}

func (s *Server) updateVisitNotes(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Notes string `json:"notes"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := s.crm.UpdateVisitNotes(r.Context(), currentUser(r), r.PathValue("id"), in.Notes)
	respond(w, r, 200, v, err)
}

// ---- Follow-ups and opportunities ----

func workQuery(r *http.Request) crm.WorkQuery {
	q := r.URL.Query()
	return crm.WorkQuery{Employee: q.Get("employee"), CustomerID: q.Get("customerId"), Status: q.Get("status"), Due: q.Get("due"), Stage: q.Get("stage"),
		Offset: queryInt(r, "offset"), Limit: queryInt(r, "limit")}
}

func (s *Server) listFollowUps(w http.ResponseWriter, r *http.Request) {
	page, err := s.crm.ListFollowUps(r.Context(), currentUser(r), workQuery(r))
	respond(w, r, 200, page, err)
}

func (s *Server) scheduleFollowUp(w http.ResponseWriter, r *http.Request) {
	var in crm.FollowUpInput
	if !decode(w, r, &in) {
		return
	}
	f, err := s.crm.ScheduleFollowUp(r.Context(), currentUser(r), in)
	respond(w, r, 201, f, err)
}

func (s *Server) updateFollowUp(w http.ResponseWriter, r *http.Request) {
	var in crm.FollowUpUpdate
	if !decode(w, r, &in) {
		return
	}
	f, err := s.crm.UpdateFollowUp(r.Context(), currentUser(r), r.PathValue("id"), in)
	respond(w, r, 200, f, err)
}

func (s *Server) listOpportunities(w http.ResponseWriter, r *http.Request) {
	page, err := s.crm.ListOpportunities(r.Context(), currentUser(r), workQuery(r))
	respond(w, r, 200, page, err)
}

func (s *Server) createOpportunity(w http.ResponseWriter, r *http.Request) {
	var in crm.OpportunityInput
	if !decode(w, r, &in) {
		return
	}
	o, err := s.crm.CreateOpportunity(r.Context(), currentUser(r), in)
	respond(w, r, 201, o, err)
}

func (s *Server) updateOpportunity(w http.ResponseWriter, r *http.Request) {
	var in crm.OpportunityPatch
	if !decode(w, r, &in) {
		return
	}
	o, err := s.crm.UpdateOpportunity(r.Context(), currentUser(r), r.PathValue("id"), in)
	respond(w, r, 200, o, err)
}

// ---- Reporting and notifications ----

func (s *Server) managerDashboard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rep, err := s.crm.ManagerDashboard(r.Context(), currentUser(r), q.Get("from"), q.Get("to"))
	respond(w, r, 200, rep, err)
}

func (s *Server) employeeActivity(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a, err := s.crm.EmployeeActivity(r.Context(), currentUser(r), r.PathValue("id"), q.Get("from"), q.Get("to"), queryInt(r, "offset"), queryInt(r, "limit"))
	respond(w, r, 200, a, err)
}

func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	n, err := s.crm.Notifications(r.Context(), currentUser(r), r.URL.Query().Get("unread") == "true")
	respond(w, r, 200, n, err)
}

func (s *Server) readNotifications(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	respond(w, r, 200, ok, s.crm.MarkNotificationsRead(r.Context(), currentUser(r), in.IDs))
}
