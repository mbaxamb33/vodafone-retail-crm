package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"vodafone/store/internal/apperr"
	"vodafone/store/internal/crm"
)

type ctxKey int

const (
	requestInfoKey ctxKey = iota
	userKey
)

// requestInfo is filled in as the request passes through middleware and read by the access log.
type requestInfo struct {
	ID      string
	UserID  string
	StoreID string
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// observe assigns a request ID, recovers panics and writes one structured log line per request.
// Customer data, bodies, cookies and query strings are never logged.
func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		info := &requestInfo{ID: crm.NewID()}
		w.Header().Set("X-Request-ID", info.ID)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		// The mux records the matched route pattern on the request it receives.
		req := r.WithContext(context.WithValue(r.Context(), requestInfoKey, info))
		defer func() {
			if p := recover(); p != nil {
				s.log.Error("panic", "request_id", info.ID, "panic", p, "stack", string(debug.Stack()))
				writeError(rec, req, apperr.ErrInternal)
			}
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			}
			s.log.Log(r.Context(), level, "request", "request_id", info.ID, "method", r.Method, "route", req.Pattern,
				"status", rec.status, "duration_ms", time.Since(start).Milliseconds(), "user_id", info.UserID, "store_id", info.StoreID)
		}()
		next.ServeHTTP(rec, req)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/health" || r.URL.Path == "/ready" {
			h.Set("Cache-Control", "no-store")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		} else {
			h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		}
		next.ServeHTTP(w, r)
	})
}

// staticFiles serves the built frontend. Unknown paths get index.html so client-side routes
// such as /customers/{id} load directly; hashed assets are cached long-term.
func staticFiles(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := path.Clean("/" + r.URL.Path)
		if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil || info.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		if strings.HasPrefix(p, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

// sameOrigin rejects state-changing requests that do not come from the configured app origin.
// Together with SameSite=Strict cookies this prevents cross-site request forgery.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Origin") != s.cfg.AppOrigin {
			writeError(w, r, apperr.ErrOriginRejected)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authenticated resolves the session cookie and stores the user in the request context.
func (s *Server) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, r, apperr.ErrUnauthorized)
			return
		}
		u, err := s.auth.Authenticate(r.Context(), c.Value)
		if err != nil {
			if errors.Is(err, apperr.ErrUnauthorized) {
				s.clearSessionCookie(w)
			}
			writeError(w, r, err)
			return
		}
		if info, ok := r.Context().Value(requestInfoKey).(*requestInfo); ok {
			info.UserID, info.StoreID = u.ID, u.StoreID
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}

func currentUser(r *http.Request) crm.User {
	u, _ := r.Context().Value(userKey).(crm.User)
	return u
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type errorBody struct {
	Error struct {
		Code      string            `json:"code"`
		Message   string            `json:"message"`
		Fields    map[string]string `json:"fields,omitempty"`
		RequestID string            `json:"requestId,omitempty"`
	} `json:"error"`
}

// writeError sends a classified error. Unclassified errors are logged by the caller's request
// log as a 500 and never exposed to the client.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	e := apperr.From(err)
	var body errorBody
	body.Error.Code, body.Error.Message, body.Error.Fields = e.Code, e.Message, e.Fields
	if info, ok := r.Context().Value(requestInfoKey).(*requestInfo); ok {
		body.Error.RequestID = info.ID
	}
	if e == apperr.ErrInternal && err != apperr.ErrInternal {
		slog.Default().Error("internal error", "request_id", body.Error.RequestID, "error", err.Error())
	}
	writeJSON(w, e.Status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

const maxBody = 32 << 10

// decode reads a JSON body strictly: unknown fields and trailing data are rejected.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(v)
	if err == nil && d.More() {
		err = errors.New("trailing data")
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, apperr.ErrPayloadTooLarge)
		} else {
			writeError(w, r, apperr.Validation(map[string]string{"body": "Cererea nu are un format valid."}))
		}
		return false
	}
	return true
}
