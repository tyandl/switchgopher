package web

import (
	"database/sql"
	"html/template"
	"net/http"

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

	return securityHeaders(auth.Middleware(router))
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
