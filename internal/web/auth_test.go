package web

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testAuth(t *testing.T) *Auth {
	t.Helper()
	h, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewAuth("admin", h)
	if err != nil {
		t.Fatal(err)
	}
	templates = template.Must(template.New("login.tmpl").Parse(`login {{ .Error }} {{ .Next }}`))
	return a
}

func TestNewAuthRejectsBadHash(t *testing.T) {
	if _, err := NewAuth("admin", ""); err == nil {
		t.Error("empty hash should be rejected")
	}
	if _, err := NewAuth("admin", "plaintext"); err == nil {
		t.Error("non-bcrypt hash should be rejected")
	}
}

func TestMiddlewareRedirectsAnonymous(t *testing.T) {
	a := testAuth(t)
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("secret")) }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/campus/stats/?x=1", nil))
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login/?next=") {
		t.Fatalf("got %d %q, want redirect to /login/", rec.Code, rec.Header().Get("Location"))
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Error("protected body leaked")
	}

	for _, path := range []string{"/login/", "/base.css", "/_tables.css", "/base.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Errorf("%s: got %d, want public", path, rec.Code)
		}
	}
}

func login(t *testing.T, a *Auth, user, pass, next string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"username": {user}, "password": {pass}, "next": {next}}
	req := httptest.NewRequest("POST", "/login/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	a.LoginHandler(rec, req)
	return rec
}

func TestLoginGrantsAccess(t *testing.T) {
	a := testAuth(t)
	rec := login(t, a, "admin", "hunter2", "/campus/stats/")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campus/stats/" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatalf("want one HttpOnly session cookie, got %+v", cookies)
	}

	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/campus/stats/", nil)
	req.AddCookie(cookies[0])
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	if out.Code != 200 {
		t.Errorf("logged-in request got %d", out.Code)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	a := testAuth(t)
	for _, c := range [][2]string{{"admin", "wrong"}, {"nobody", "hunter2"}} {
		rec := login(t, a, c[0], c[1], "")
		if rec.Code != http.StatusUnauthorized || len(rec.Result().Cookies()) != 0 {
			t.Errorf("%v: got %d with %d cookies", c, rec.Code, len(rec.Result().Cookies()))
		}
	}
}

func TestForgedAndExpiredCookiesRejected(t *testing.T) {
	a := testAuth(t)
	if a.valid("9999999999.deadbeef") {
		t.Error("forged signature accepted")
	}
	if a.valid(a.sign(time.Now().Add(-time.Minute).Unix())) {
		t.Error("expired cookie accepted")
	}
	if !a.valid(a.sign(time.Now().Add(time.Minute).Unix())) {
		t.Error("good cookie rejected")
	}
	other := testAuth(t)
	if other.valid(a.sign(time.Now().Add(time.Minute).Unix())) {
		t.Error("cookie signed with another secret accepted")
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/campus/vlans/":  "/campus/vlans/",
		"":                "/campus/home/",
		"//evil.com":      "/campus/home/",
		"/\\evil.com":     "/campus/home/",
		"http://evil.com": "/campus/home/",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	a := testAuth(t)
	rec := httptest.NewRecorder()
	a.LogoutHandler(rec, httptest.NewRequest("GET", "/logout/", nil))
	c := rec.Result().Cookies()
	if rec.Code != http.StatusSeeOther || len(c) != 1 || c[0].Value != "" {
		t.Errorf("got %d %+v", rec.Code, c)
	}
}

func TestSessionCookieSecureBehindTLSProxy(t *testing.T) {
	a := testAuth(t)
	for header, wantSecure := range map[string]bool{"": false, "http": false, "https": true, "HTTPS": true} {
		req := httptest.NewRequest("GET", "/logout/", nil)
		if header != "" {
			req.Header.Set("X-Forwarded-Proto", header)
		}
		rec := httptest.NewRecorder()
		a.LogoutHandler(rec, req)
		c := rec.Result().Cookies()
		if len(c) != 1 || c[0].Secure != wantSecure {
			t.Errorf("X-Forwarded-Proto=%q: Secure = %v, want %v", header, c[0].Secure, wantSecure)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	for k, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "same-origin",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}
