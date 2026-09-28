package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vodafone/store/internal/crm"
	"vodafone/store/internal/httpapi"
	"vodafone/store/internal/postgres/pgtest"
)

const origin = "http://app.test"

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

// do sends a request with the app origin and decodes the JSON response into out when given.
func (c *client) do(method, path string, body any, out any) (int, map[string]any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, c.base+"/api/v1/"+path, r)
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.Header.Get("X-Request-ID") == "" {
		c.t.Fatal("missing X-Request-ID")
	}
	raw, _ := io.ReadAll(res.Body)
	var errBody map[string]any
	if res.StatusCode >= 400 {
		_ = json.Unmarshal(raw, &errBody)
	} else if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: %v: %s", method, path, err, raw)
		}
	}
	return res.StatusCode, errBody
}

func (c *client) login(email string) {
	c.t.Helper()
	if code, body := c.do("POST", "auth/login", map[string]string{"email": email, "password": pgtest.Password}, nil); code != 200 {
		c.t.Fatalf("login %s: %d %v", email, code, body)
	}
}

func errorCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func setup(t *testing.T) (*httptest.Server, pgtest.Store, *bool) {
	db := pgtest.New(t)
	a := pgtest.FastAuth(db)
	st := pgtest.NewStore(t, db, a, "HTTP Store")
	down := false
	ready := func(ctx context.Context) error {
		if down {
			return errors.New("down")
		}
		return db.Ping(ctx)
	}
	srv := httptest.NewServer(httpapi.New(crm.NewService(db), a, httpapi.Config{AppOrigin: origin, SessionTTL: time.Hour}, slog.New(slog.NewTextHandler(io.Discard, nil)), ready).Handler())
	t.Cleanup(srv.Close)
	return srv, st, &down
}

func TestAuthBoundaries(t *testing.T) {
	srv, st, down := setup(t)
	anon := newClient(t, srv.URL)
	if code, body := anon.do("GET", "workspace", nil, nil); code != 401 || errorCode(body) != "UNAUTHORIZED" {
		t.Fatal("anonymous access", code)
	}
	if code, body := anon.do("POST", "auth/login", map[string]string{"email": st.Ioana.Email, "password": "nope-nope-nope"}, nil); code != 401 || errorCode(body) != "INVALID_CREDENTIALS" {
		t.Fatal("bad password", code, body)
	}
	emp := newClient(t, srv.URL)
	emp.login(st.Ioana.Email)
	if code, _ := emp.do("GET", "manager/dashboard", nil, nil); code != 403 {
		t.Fatal("employee reached manager analytics")
	}
	if code, _ := emp.do("GET", "follow-ups?employee=all", nil, nil); code != 403 {
		t.Fatal("employee listed store work")
	}

	// A request from another origin is refused even with a valid session.
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/customers", strings.NewReader(`{"name":"Evil","phone":"0722000000"}`))
	req.Header.Set("Origin", "https://evil.example")
	res, err := emp.http.Do(req)
	if err != nil || res.StatusCode != 403 {
		t.Fatal("cross-origin mutation accepted")
	}
	res.Body.Close()

	if code, body := emp.do("POST", "customers", map[string]any{"name": "X", "phone": "1", "extra": true}, nil); code != 422 || errorCode(body) != "VALIDATION_FAILED" {
		t.Fatal("unknown fields must be rejected", code)
	}
	code, body := emp.do("POST", "customers", map[string]any{"name": "X", "phone": "1"}, nil)
	fields, _ := body["error"].(map[string]any)["fields"].(map[string]any)
	if code != 422 || fields["name"] == nil || fields["phone"] == nil {
		t.Fatal("field-level errors", body)
	}
	if code, body := emp.do("GET", "customers/not-an-id", nil, nil); code != 404 || errorCode(body) != "CUSTOMER_NOT_FOUND" {
		t.Fatal("malformed id", code, body)
	}
	if code, _ := emp.do("POST", "auth/logout", map[string]any{}, nil); code != 200 {
		t.Fatal("logout")
	}
	if code, _ := emp.do("GET", "workspace", nil, nil); code != 401 {
		t.Fatal("session survived logout")
	}

	res, _ = http.Get(srv.URL + "/ready")
	if res.StatusCode != 200 {
		t.Fatal("ready")
	}
	*down = true
	res, _ = http.Get(srv.URL + "/ready")
	if res.StatusCode != 503 {
		t.Fatal("readiness should fail when the database is unavailable")
	}
}

// TestEmployeeWorkflow: log in, search, create, record a visit taking ownership with a
// follow-up, and find the customer in the portfolio.
func TestEmployeeWorkflow(t *testing.T) {
	srv, st, _ := setup(t)
	emp := newClient(t, srv.URL)
	emp.login(st.Ioana.Email)

	var found crm.Page[crm.Customer]
	emp.do("GET", "customers?q=0722%20345%20678", nil, &found)
	if found.Total != 0 {
		t.Fatal("unexpected customer")
	}
	var c crm.Customer
	if code, body := emp.do("POST", "customers", map[string]any{"name": "Client Nou", "phone": "0722 345 678"}, &c); code != 201 {
		t.Fatal(code, body)
	}
	var visit crm.VisitResult
	code, body := emp.do("POST", "customers/"+c.ID+"/visits", map[string]any{
		"reasonCode": "renewal", "steps": []int{0, 1, 3}, "notes": "Revine joi.", "ownership": "owned",
		"nextAction": "Sună clientul", "due": "2026-10-01", "opportunities": []map[string]any{{"product": "Red Unlimited", "category": "mobile"}},
	}, &visit)
	if code != 201 || visit.FollowUp == nil || visit.Customer.OwnerID != st.Ioana.ID {
		t.Fatal("visit", code, body)
	}
	emp.do("GET", "customers?owner=me", nil, &found)
	if found.Total != 1 || found.Items[0].ID != c.ID || found.Items[0].NextFollowUpDue != "2026-10-01" {
		t.Fatalf("portfolio: %+v", found)
	}
	var fu crm.Page[crm.FollowUp]
	emp.do("GET", "follow-ups", nil, &fu)
	if fu.Total != 1 {
		t.Fatal("follow-up missing from work queue")
	}
	var done crm.FollowUp
	if code, _ := emp.do("PATCH", "follow-ups/"+fu.Items[0].ID, map[string]any{"status": "done"}, &done); code != 200 || done.CompletedAt == nil {
		t.Fatal("complete follow-up")
	}
	var opp crm.Opportunity
	if code, _ := emp.do("PATCH", "opportunities/"+visit.Opportunities[0].ID, map[string]any{"stage": "won"}, &opp); code != 200 || opp.Stage != "won" {
		t.Fatal("stage change")
	}
	if code, body := emp.do("PATCH", "opportunities/"+opp.ID, map[string]any{"stage": "offer"}, nil); code != 409 || errorCode(body) != "INVALID_STAGE_TRANSITION" {
		t.Fatal("reopened a won opportunity", code)
	}
	var catalog map[string]json.RawMessage
	emp.do("GET", "catalog", nil, &catalog)
	if len(catalog["visitReasons"]) < 10 || len(catalog["journeySteps"]) == 0 {
		t.Fatal("catalog")
	}
}

// TestManagerWorkflow: a manager opens the team dashboard, drills into an employee and inspects
// their activity and pipeline, then reassigns a customer.
func TestManagerWorkflow(t *testing.T) {
	srv, st, _ := setup(t)
	emp := newClient(t, srv.URL)
	emp.login(st.Ioana.Email)
	var c crm.Customer
	emp.do("POST", "customers", map[string]any{"name": "Client Echipa", "phone": "0722 111 000"}, &c)
	emp.do("POST", "customers/"+c.ID+"/visits", map[string]any{"steps": []int{0, 3, 4, 5}, "ownership": "owned", "opportunities": []map[string]any{{"product": "Internet"}}}, nil)

	mgr := newClient(t, srv.URL)
	mgr.login(st.Mgr.Email)
	var rep crm.ManagerReport
	if code, _ := mgr.do("GET", "manager/dashboard", nil, &rep); code != 200 {
		t.Fatal("dashboard")
	}
	if rep.Summary.Visits != 1 || rep.Summary.NewCustomers != 1 || rep.Funnel[3].Count != 1 || rep.Funnel[4].Count != 0 || len(rep.Employees) != 3 {
		t.Fatalf("report: %+v", rep)
	}
	var act crm.EmployeeActivity
	if code, _ := mgr.do("GET", "manager/employees/"+st.Ioana.ID+"/activity", nil, &act); code != 200 || act.Total != 1 || act.Summary.OpportunitiesCreated != 1 || len(act.Customers) != 1 {
		t.Fatalf("activity: %+v", act)
	}
	var opps crm.Page[crm.Opportunity]
	mgr.do("GET", "opportunities?employee="+st.Ioana.ID, nil, &opps)
	if opps.Total != 1 {
		t.Fatal("employee pipeline")
	}
	var profile crm.Profile
	mgr.do("GET", "customers/"+c.ID, nil, &profile)
	if len(profile.Audit) == 0 {
		t.Fatal("manager should see audit history")
	}
	var moved crm.Customer
	if code, _ := mgr.do("POST", "customers/"+c.ID+"/ownership", map[string]any{"ownership": "owned", "ownerId": st.Andrei.ID}, &moved); code != 200 || moved.OwnerID != st.Andrei.ID {
		t.Fatal("reassign")
	}
	var notes crm.NotificationList
	emp.do("GET", "notifications", nil, &notes)
	if notes.Unread != 1 || notes.Items[0].Kind != "customer_reassigned" {
		t.Fatalf("previous owner not notified: %+v", notes)
	}
	if code, _ := emp.do("POST", "notifications/read", map[string]any{"ids": []string{notes.Items[0].ID}}, nil); code != 200 {
		t.Fatal("mark read")
	}
	emp.do("GET", "customers/"+c.ID, nil, &profile)
	if len(profile.Audit) != 0 || len(profile.OwnershipHistory) != 2 {
		t.Fatal("employees see ownership history but not the audit log")
	}
}

func TestStaticFrontendAndProxiedClientIP(t *testing.T) {
	db := pgtest.New(t)
	a := pgtest.FastAuth(db)
	st := pgtest.NewStore(t, db, a, "Static Store")
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>app"), 0o600))
	must(os.MkdirAll(filepath.Join(dir, "assets"), 0o700))
	must(os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("js"), 0o600))
	h := httpapi.New(crm.NewService(db), a, httpapi.Config{AppOrigin: origin, SessionTTL: time.Hour, StaticDir: dir, TrustProxy: true}, slog.New(slog.NewTextHandler(io.Discard, nil)), db.Ping).Handler()
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	if w := get("/customers/abc"); w.Code != 200 || !strings.Contains(w.Body.String(), "app") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("client route should serve index.html with the page CSP", w.Code)
	}
	if w := get("/assets/app.js"); w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("hashed assets should be cacheable")
	}
	if w := get("/../../etc/passwd"); strings.Contains(w.Body.String(), "root:") {
		t.Fatal("path traversal")
	}
	if w := get("/api/v1/unknown"); w.Code != 401 && w.Code != 404 {
		t.Fatal("api paths must not fall back to index.html", w.Code)
	}
	// Failures from one forwarded client must not lock out others behind the same proxy.
	login := func(ip, password string) int {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"email":"`+st.Ioana.Email+`","password":"`+password+`"}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Forwarded-For", ip)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for i := 0; i < 30; i++ {
		login("203.0.113.9", "wrong")
	}
	var ip string
	must(db.Pool.QueryRow(context.Background(), `SELECT client_ip FROM login_attempts ORDER BY id DESC LIMIT 1`).Scan(&ip))
	if ip != "203.0.113.9" {
		t.Fatalf("recorded ip %q", ip)
	}
}
