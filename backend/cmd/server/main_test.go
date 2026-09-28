package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"vodafone/store/internal/crm"
)

func TestAuthAndManagerBoundary(t *testing.T) {
	s, e := crm.New(context.Background(), crm.FileRepository{Path: t.TempDir() + "/state.json"}, true)
	if e != nil {
		t.Fatal(e)
	}
	a := &app{s: s, demo: true, origin: "http://localhost", sessions: map[string]session{}, attempts: map[string][]time.Time{}}
	r := httptest.NewRequest("GET", "/api/v1/workspace", nil)
	w := httptest.NewRecorder()
	a.serve(w, r)
	if w.Code != 401 {
		t.Fatal("anonymous access")
	}
	r = httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"role":"employee"}`))
	r.Header.Set("Origin", a.origin)
	w = httptest.NewRecorder()
	a.serve(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	cookies := w.Result().Cookies()
	r = httptest.NewRequest("GET", "/api/v1/manager/dashboard", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	a.serve(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("employee accessed manager analytics")
	}
	r = httptest.NewRequest("POST", "/api/v1/customers", strings.NewReader(`{"name":"Example","phone":"0722345678"}`))
	r.AddCookie(cookies[0])
	r.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	a.serve(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin mutation accepted")
	}
}

func TestEmployeeToManagerWorkflow(t *testing.T) {
	s, e := crm.New(context.Background(), crm.FileRepository{Path: t.TempDir() + "/state.json"}, true)
	if e != nil {
		t.Fatal(e)
	}
	a := &app{s: s, demo: true, origin: "http://localhost", sessions: map[string]session{}, attempts: map[string][]time.Time{}}
	call := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/"+path, strings.NewReader(body))
		r.Header.Set("Origin", a.origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		a.serve(w, r)
		return w
	}
	login := call("POST", "auth/login", `{"role":"employee"}`, nil)
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	employee := login.Result().Cookies()[0]
	created := call("POST", "customers", `{"name":"Workflow Customer","phone":"0722 000 555"}`, employee)
	if created.Code != 201 {
		t.Fatal(created.Body.String())
	}
	var c crm.Customer
	if e := json.Unmarshal(created.Body.Bytes(), &c); e != nil {
		t.Fatal(e)
	}
	visit := call("POST", "customers/"+c.ID+"/visits", `{"reason":"Test","steps":[0,5],"notes":"Follow-up context","ownership":"owned","nextAction":"Call","due":"2026-10-01","product":"Internet"}`, employee)
	if visit.Code != 201 {
		t.Fatal(visit.Body.String())
	}
	w := call("GET", "workspace", "", employee)
	var state crm.State
	if e := json.Unmarshal(w.Body.Bytes(), &state); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, customer := range state.Customers {
		if customer.ID == c.ID && customer.OwnerID == "ioana" {
			found = true
		}
	}
	if !found {
		t.Fatal("customer absent from employee portfolio")
	}
	profile := call("GET", "customers/"+c.ID, "", employee)
	var detail struct {
		Visits []crm.Visit `json:"visits"`
		Audit  []crm.Audit `json:"audit"`
	}
	_ = json.Unmarshal(profile.Body.Bytes(), &detail)
	if len(detail.Visits) != 1 || len(detail.Visits[0].Steps) != 2 || len(detail.Audit) != 0 {
		t.Fatal("incorrect history or audit boundary")
	}
	managerLogin := call("POST", "auth/login", `{"role":"manager"}`, nil)
	manager := managerLogin.Result().Cookies()[0]
	report := call("GET", "manager/dashboard", "", manager)
	if report.Code != 200 || !strings.Contains(report.Body.String(), c.ID) {
		t.Fatal("manager cannot inspect employee activity")
	}
	profile = call("GET", "customers/"+c.ID, "", manager)
	_ = json.Unmarshal(profile.Body.Bytes(), &detail)
	if len(detail.Audit) < 3 {
		t.Fatal("manager audit missing")
	}
}
