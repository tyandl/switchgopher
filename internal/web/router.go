package web

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/gorilla/mux"
)

// NewRouter registers every route and returns the http.Handler for it,
// wrapped in auth's login middleware.
func NewRouter(db *sql.DB, auth *Auth) http.Handler {
	templates = template.Must(template.ParseGlob("resources/templates/*.tmpl"))

	router := mux.NewRouter().StrictSlash(true)

	// /
	router.HandleFunc("/", rootHandler)
	router.HandleFunc("/{style:[a-z_]*\\.css$}", styleHandler)
	router.HandleFunc("/{script:[a-z]*\\.js$}", scriptHandler)
	router.HandleFunc("/download/", downloadHandler)

	router.HandleFunc("/logout/", auth.LogoutHandler)
	router.HandleFunc("/login/", auth.LoginHandler)

	// /campus/
	router.HandleFunc(homePath, homeHandler)
	router.HandleFunc("/campus/help/", helpHandler)
	router.HandleFunc("/campus/switches/", dbHandler(switchesHandler, db))
	router.HandleFunc("/campus/vlans/", dbHandler(vlansHandler, db))
	router.HandleFunc("/campus/stats/", dbHandler(statsHandler, db))
	router.HandleFunc("/campus/search/", dbHandler(searchHandler, db))
	router.HandleFunc("/campus/unused/", dbHandler(unusedHandler, db))

	// /switches/
	router.HandleFunc("/campus/switches/ports/", dbHandler(portsHandler, db))
	router.HandleFunc("/campus/switches/info/", dbHandler(infoHandler, db))
	router.HandleFunc("/campus/switches/configs/", dbHandler(configsHandler, db))
	router.HandleFunc("/campus/switches/diffs/", dbHandler(diffsHandler, db))

	// /configs/
	router.HandleFunc("/campus/switches/configs/config/", dbHandler(configHandler, db))
	router.HandleFunc("/campus/switches/configs/diff/", dbHandler(diffHandler, db))

	// /ports/
	router.HandleFunc("/campus/ports/port/", dbHandler(portHandler, db))
	router.HandleFunc("/campus/ports/node/", dbHandler(nodeHandler, db))

	// /vlans/
	router.HandleFunc("/campus/vlans/vlan/", dbHandler(vlanHandler, db))

	return requestID(requestLogger(securityHeaders(auth.Middleware(router))))
}

// securityHeaders adds baseline browser protections to every response:
// no content-type sniffing, no framing (clickjacking), and no Referer sent
// to other sites.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, req)
	})
}

// statusRecorder remembers the status code a handler wrote, for the request log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// requestLogger logs every request at Debug level: method, path, status and
// how long it took. It logs the path but not the query string, which can carry
// MAC addresses and hostnames, and never the request body (the login form).
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, req)
		slog.Debug("request", "method", req.Method, "path", req.URL.Path, "status", rec.status,
			"took", time.Since(start).Round(time.Microsecond), "remote", req.RemoteAddr, "request_id", requestIDFrom(req.Context()))
	})
}

type requestIDKey struct{}

// validRequestID restricts an ID supplied by a client or proxy to a short,
// harmless token, so it cannot be used to inject text into the logs.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// requestID gives every request an ID, stores it in the request context (see
// requestIDFrom) and returns it in the X-Request-ID response header. A valid
// X-Request-ID sent by a reverse proxy is reused, so one request can be followed
// from the proxy's log into this one; anything else is replaced.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		id := req.Header.Get("X-Request-ID")
		if !validRequestID.MatchString(id) {
			b := make([]byte, 6)
			rand.Read(b) // never fails on supported platforms
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), requestIDKey{}, id)))
	})
}

// requestIDFrom returns the ID requestID stored in ctx, or "" if there is none.
func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
