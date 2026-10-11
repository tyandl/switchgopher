package web

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// captureLogs sends slog output to a buffer, at Debug level, until the test ends.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func TestInternalErrorHidesDetailsFromTheClientButLogsThem(t *testing.T) {
	logs := captureLogs(t)
	rec := httptest.NewRecorder()
	internalError(rec, httptest.NewRequest("GET", "/campus/stats/?secret=1", nil), errors.New("SELECT * FROM nodes: disk I/O error"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "SELECT") || strings.Contains(rec.Body.String(), "disk") {
		t.Errorf("error details leaked to the client: %q", rec.Body.String())
	}
	out := logs.String()
	for _, want := range []string{"level=ERROR", "path=/campus/stats/", "disk I/O error"} {
		if !strings.Contains(out, want) {
			t.Errorf("log is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret=1") {
		t.Errorf("the query string should not be logged:\n%s", out)
	}
}

func TestRequestLoggerRecordsStatusAtDebug(t *testing.T) {
	logs := captureLogs(t)
	h := requestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x/?mac=AA:BB", nil))

	out := logs.String()
	for _, want := range []string{"level=DEBUG", "msg=request", "method=GET", "path=/x/", "status=418"} {
		if !strings.Contains(out, want) {
			t.Errorf("log is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "mac=") {
		t.Errorf("the query string should not be logged:\n%s", out)
	}
}

func TestLoginEventsAreLoggedWithoutTheUsernameOrPassword(t *testing.T) {
	a := testAuth(t)
	logs := captureLogs(t)

	post := func(user, pass string) {
		form := url.Values{"username": {user}, "password": {pass}}
		req := httptest.NewRequest("POST", "/login/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		a.LoginHandler(httptest.NewRecorder(), req)
	}
	post("hunter2-typed-as-username", "wrong")
	post("admin", "hunter2")

	out := logs.String()
	if !strings.Contains(out, "level=WARN msg=\"login failed\"") || !strings.Contains(out, "level=INFO msg=\"login succeeded\"") {
		t.Errorf("expected a warning for the failure and an info for the success:\n%s", out)
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("a username or password was logged:\n%s", out)
	}
}

func TestRequestIDIsGeneratedReusedAndSanitized(t *testing.T) {
	var seen string
	h := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = requestIDFrom(r.Context())
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if len(seen) != 12 || rec.Header().Get("X-Request-ID") != seen {
		t.Errorf("generated id %q, header %q", seen, rec.Header().Get("X-Request-ID"))
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "proxy-abc.123")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "proxy-abc.123" {
		t.Errorf("a valid id from a proxy should be reused, got %q", seen)
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "bad id\nlevel=ERROR msg=forged")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if strings.ContainsAny(seen, " \n=") || len(seen) != 12 {
		t.Errorf("an unsafe id must be replaced, got %q", seen)
	}
}

func TestRequestIDAppearsInTheLogAndTheErrorPage(t *testing.T) {
	logs := captureLogs(t)
	h := requestID(requestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalError(w, r, errors.New("boom"))
	})))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x/", nil)
	req.Header.Set("X-Request-ID", "ref123")
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "(reference: ref123)") {
		t.Errorf("the error page should show the reference: %q", rec.Body.String())
	}
	if n := strings.Count(logs.String(), "request_id=ref123"); n != 2 {
		t.Errorf("expected the id on both the error line and the request line, found %d:\n%s", n, logs.String())
	}
}
