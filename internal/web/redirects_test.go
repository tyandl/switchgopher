package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A request missing its required query parameter must redirect to the real
// home route (/campus/home/), not a path that 404s. None of these handlers
// touch the database before checking their parameters, so nil is fine.
func TestMissingParamRedirectsToCampusHome(t *testing.T) {
	handlers := map[string]func(http.ResponseWriter, *http.Request, *sql.DB){
		"portHandler":    portHandler,
		"nodeHandler":    nodeHandler,
		"vlanHandler":    vlanHandler,
		"portsHandler":   portsHandler,
		"infoHandler":    infoHandler,
		"configsHandler": configsHandler,
		"diffsHandler":   diffsHandler,
		"configHandler":  configHandler,
		"diffHandler":    diffHandler,
	}
	for name, h := range handlers {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest("GET", "/x/", nil), nil)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campus/home/" {
			t.Errorf("%s: got %d %q, want 303 /campus/home/", name, rec.Code, rec.Header().Get("Location"))
		}
	}
}
