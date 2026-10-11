package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "switchgopher_session"
	sessionTTL    = 12 * time.Hour
)

// Auth is a single shared login (one username, one password) guarding the
// whole UI with a signed session cookie. See documentation/TODO.md for the
// planned move to per-user accounts via LDAP/SSO.
type Auth struct {
	username string
	hash     []byte
	// secret signs session cookies. Random per process, so a restart logs
	// everyone out - acceptable for a single shared login.
	secret []byte
}

// HashPassword returns a bcrypt hash suitable for -auth-password-hash.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

// NewAuth validates the bcrypt hash up front so a bad value fails at
// startup instead of locking everyone out silently.
func NewAuth(username, passwordHash string) (*Auth, error) {
	if username == "" {
		return nil, errors.New("auth username is empty")
	}
	if _, err := bcrypt.Cost([]byte(passwordHash)); err != nil {
		return nil, errors.New("auth password hash is missing or not a valid bcrypt hash")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	return &Auth{username: username, hash: []byte(passwordHash), secret: secret}, nil
}

func (a *Auth) sign(expiry int64) string {
	exp := strconv.FormatInt(expiry, 10)
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(exp))
	return exp + "." + hex.EncodeToString(mac.Sum(nil))
}

func (a *Auth) valid(value string) bool {
	exp, _, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	expiry, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() >= expiry {
		return false
	}
	return hmac.Equal([]byte(value), []byte(a.sign(expiry)))
}

func (a *Auth) loggedIn(req *http.Request) bool {
	c, err := req.Cookie(sessionCookie)
	return err == nil && a.valid(c.Value)
}

// Static assets stay public so the login page itself can be styled.
var publicAsset = regexp.MustCompile(`^/[a-z_]*\.(css|js)$`)

// Middleware redirects every unauthenticated request to /login/, except the
// login/logout pages and static assets.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		p := req.URL.Path
		if p == "/login/" || p == "/logout/" || publicAsset.MatchString(p) || a.loggedIn(req) {
			next.ServeHTTP(w, req)
			return
		}
		target := "/login/"
		if req.Method == http.MethodGet {
			target += "?next=" + url.QueryEscape(req.URL.RequestURI())
		}
		http.Redirect(w, req, target, http.StatusSeeOther)
	})
}

// safeNext only allows same-site absolute paths, so ?next= can't be used as
// an open redirect.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return homePath
	}
	return next
}

func (a *Auth) setCookie(w http.ResponseWriter, req *http.Request, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   req.TLS != nil || strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https"),
	})
}

// LoginHandler shows the login form (GET) and checks credentials (POST). On
// success it sets the session cookie and redirects to the "next" page.
func (a *Auth) LoginHandler(w http.ResponseWriter, req *http.Request) {
	p := map[string]Any{
		"Title":   "Login",
		"Version": Version,
		"Next":    safeNext(req.FormValue("next")),
	}

	if req.Method == http.MethodPost {
		user := req.PostFormValue("username")
		// Always run bcrypt, even for a wrong username, so response time
		// doesn't reveal which half was wrong.
		passErr := bcrypt.CompareHashAndPassword(a.hash, []byte(req.PostFormValue("password")))
		userOK := subtle.ConstantTimeCompare([]byte(user), []byte(a.username)) == 1
		if userOK && passErr == nil {
			expiry := time.Now().Add(sessionTTL)
			a.setCookie(w, req, a.sign(expiry.Unix()), expiry)
			slog.Info("login succeeded", "remote", req.RemoteAddr, "request_id", requestIDFrom(req.Context()))
			http.Redirect(w, req, safeNext(req.PostFormValue("next")), http.StatusSeeOther)
			return
		}
		// The username is deliberately not logged: people sometimes type
		// their password into that field.
		slog.Warn("login failed", "remote", req.RemoteAddr, "request_id", requestIDFrom(req.Context()))
		time.Sleep(500 * time.Millisecond)
		p["Error"] = "Invalid username or password."
		w.WriteHeader(http.StatusUnauthorized)
	}

	if err := templates.ExecuteTemplate(w, "login.tmpl", &p); err != nil {
		internalError(w, req, err)
	}
}

// LogoutHandler clears the session cookie and returns to the login page.
func (a *Auth) LogoutHandler(w http.ResponseWriter, req *http.Request) {
	a.setCookie(w, req, "", time.Unix(0, 0))
	slog.Debug("logged out", "remote", req.RemoteAddr)
	http.Redirect(w, req, "/login/", http.StatusSeeOther)
}
