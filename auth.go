package main

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type session struct {
	user    string
	expires time.Time
}
type App struct {
	store         *Store
	ledger        Ledger
	mu            sync.Mutex
	sessions      map[[32]byte]session
	loginSlots    chan struct{}
	coachRequests map[string]time.Time
	dummyHash     []byte
}

func newApp(s *Store) *App {
	dummy, _ := bcrypt.GenerateFromPassword([]byte(randomID()), bcrypt.DefaultCost)
	return &App{store: s, ledger: localLedger{s}, sessions: make(map[[32]byte]session), loginSlots: make(chan struct{}, 4), dummyHash: dummy}
}
func current(c *gin.Context) Account              { return c.MustGet("account").(Account) }
func fail(c *gin.Context, status int, msg string) { c.AbortWithStatusJSON(status, gin.H{"error": msg}) }
func (a *App) require(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, _ := c.Cookie("fatbot_session")
		a.mu.Lock()
		sess, ok := a.sessions[sha256.Sum256([]byte(token))]
		a.mu.Unlock()
		user, exists := a.store.snapshot().Accounts[sess.user]
		if !ok || !exists || !time.Now().Before(sess.expires) {
			if c.Request.Method == "GET" && (c.FullPath() == "/career" || c.FullPath() == "/holder" || c.FullPath() == "/issuer" || c.FullPath() == "/verifier") {
				c.Redirect(303, "/login")
				c.Abort()
				return
			}
			fail(c, 401, "Sign in to continue")
			return
		}
		for _, r := range roles {
			if user.Role == r {
				c.Set("account", user)
				c.Next()
				return
			}
		}
		fail(c, 403, "This account cannot perform that action")
	}
}
func sessionCookie(c *gin.Context, value string, age int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "fatbot_session", Value: value, Path: "/", MaxAge: age, HttpOnly: true, Secure: c.Request.TLS != nil, SameSite: http.SameSiteStrictMode})
}
func (a *App) login(c *gin.Context) {
	select {
	case a.loginSlots <- struct{}{}:
		defer func() { <-a.loginSlots }()
	default:
		fail(c, 429, "Please try again shortly")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	u, ok := a.store.snapshot().Accounts[strings.ToLower(strings.TrimSpace(c.PostForm("username")))]
	hash := a.dummyHash
	if ok {
		hash = u.PasswordHash
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(c.PostForm("password"))) != nil || !ok {
		c.Redirect(303, "/login?failed=1")
		return
	}
	token := randomID()
	old, _ := c.Cookie("fatbot_session")
	a.mu.Lock()
	delete(a.sessions, sha256.Sum256([]byte(old)))
	for k, s := range a.sessions {
		if !time.Now().Before(s.expires) {
			delete(a.sessions, k)
		}
	}
	a.sessions[sha256.Sum256([]byte(token))] = session{u.ID, time.Now().Add(8 * time.Hour)}
	a.mu.Unlock()
	sessionCookie(c, token, 8*3600)
	c.Redirect(303, "/"+u.Role)
}
func (a *App) logout(c *gin.Context) {
	token, _ := c.Cookie("fatbot_session")
	a.mu.Lock()
	delete(a.sessions, sha256.Sum256([]byte(token)))
	a.mu.Unlock()
	sessionCookie(c, "", -1)
	c.Redirect(303, "/login")
}
func sameOrigin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	if c.Request.Method == "POST" {
		origin := c.GetHeader("Origin")
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		if origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Scheme != scheme || !strings.EqualFold(u.Host, c.Request.Host) {
				fail(c, 403, "Cross-origin request denied")
				return
			}
		}
		if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			fail(c, 403, "Cross-origin request denied")
			return
		}
	}
	c.Next()
}
func addUser(s *Store, id, role, org, password string) error {
	id = strings.ToLower(strings.TrimSpace(id))
	org = strings.TrimSpace(org)
	if id == "" || len(id) > 128 || len(org) > 128 || (role != "issuer" && role != "holder" && role != "verifier") || (role == "issuer" && org == "") {
		return errors.New("supply a username, valid role and organization for issuers")
	}
	if len(password) < 12 || len(password) > 72 {
		return errors.New("password must be 12 to 72 bytes")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	return s.update(func(st *State) error {
		if _, ok := st.Accounts[id]; ok {
			return errors.New("account already exists")
		}
		st.Accounts[id] = Account{id, role, org, hash}
		return nil
	})
}
