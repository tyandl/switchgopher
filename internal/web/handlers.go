// Package web holds the HTTP handlers and route wiring for switchgopher's
// browser UI.
package web

import (
	"database/sql"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/gorilla/mux"
)

// Version is shown in the page footer and the Generator meta tag.
const Version = "0.0.1"

// Author is shown in the page footer and the Author meta tag.
const Author = "Timothy Yandl"

// type def interface{} for readability
type Any interface{}

// the template files. Loaded once by NewRouter (not eagerly at package init)
// so that importing this package - e.g. from tests - doesn't require running
// from the repo root just to find templates/*.tmpl.
var templates *template.Template

// wrap the database pointer in a closure.
func dbHandler(fn func(http.ResponseWriter, *http.Request, *sql.DB), db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fn(w, r, db)
	}
}

// homePath is the landing page. Every redirect to it goes through
// redirectHome, so the path is written in one place.
const homePath = "/campus/home/"

// internalError logs err together with the request that caused it and answers
// 500 with a generic message and the request's ID, so a user can quote it and
// it can be found in the log. The error text stays in the log: it can contain
// SQL or file paths that a visitor has no business seeing.
func internalError(w http.ResponseWriter, req *http.Request, err error) {
	id := requestIDFrom(req.Context())
	slog.Error("request failed", "method", req.Method, "path", req.URL.Path, "request_id", id, "err", err)
	msg := "internal server error"
	if id != "" {
		msg += " (reference: " + id + ")"
	}
	http.Error(w, msg, http.StatusInternalServerError)
}

// redirectHome sends the client to the home page with a 303 code.
// 303 "See Other" indicates that the correct response can be found under a
// different URI and should be retrieved using a GET method.
func redirectHome(w http.ResponseWriter, req *http.Request) {
	http.Redirect(w, req, homePath, http.StatusSeeOther)
}

// root handler simply redirects the request to the homepage.
func rootHandler(w http.ResponseWriter, req *http.Request) {
	redirectHome(w, req)
}

func helpHandler(w http.ResponseWriter, req *http.Request) {
	p := map[string]Any{
		"Title":   "Help",
		"Version": Version,
	}
	if err := templates.ExecuteTemplate(w, "help.tmpl", &p); err != nil {
		internalError(w, req, err)
		return
	}
}

// safeJoin joins base with a caller-supplied relative path, blocking
// directory traversal outside base while still allowing legitimate
// subdirectories (e.g. "Category/Device.txt", as configsHandler builds).
// filepath.Base would also block traversal, but by discarding everything
// except the leaf filename - it can't tell "Category/Device.txt" apart from
// "../../etc/passwd", so it breaks the former while blocking the latter.
// Cleaning a "/"-rooted copy of userPath first absorbs any ".." components
// (Clean never lets a rooted path climb above "/"), and then Join treats
// that as an ordinary relative path under base, not a re-rooted absolute one.
func safeJoin(base, userPath string) string {
	return filepath.Join(base, filepath.Clean("/"+userPath))
}

// handles requests for file downloads
func downloadHandler(w http.ResponseWriter, req *http.Request) {
	folderVals, ok := req.URL.Query()["folder"]
	if !ok || folderVals[0] == "" {
		http.Error(w, "missing folder", http.StatusBadRequest)
		return
	}
	fileVals, ok := req.URL.Query()["file"]
	if !ok || fileVals[0] == "" {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	folder, ok := configFolder(folderVals[0])
	if !ok {
		http.Error(w, "no such folder", http.StatusNotFound)
		return
	}
	http.ServeFile(w, req, safeJoin(folder, fileVals[0]))
}

// style handler serves the css files for all pages on the site. Input is sanitized but muxer.
func styleHandler(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "resources/styles/"+mux.Vars(req)["style"])
}

// scriptHandler serves the javascript files for all pages on the site. The
// filename is restricted by the route's regexp.
func scriptHandler(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "resources/scripts/"+mux.Vars(req)["script"])
}
