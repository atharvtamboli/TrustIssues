package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

type harness struct {
	t   *testing.T
	s   *Store
	app *App
	r   http.Handler
}

func setup(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	s, e := openStore(filepath.Join(dir, "state"), dir)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.lock.Close() })
	for _, u := range [][]string{{"alice", "holder", ""}, {"bob", "holder", ""}, {"dean", "issuer", "uni-a"}, {"other", "issuer", "uni-b"}, {"employer", "verifier", ""}} {
		if e = addUser(s, u[0], u[1], u[2], "synthetic-password"); e != nil {
			t.Fatal(e)
		}
	}
	a := newApp(s)
	return &harness{t, s, a, a.router()}
}
func (h *harness) request(method, path, ct string, body io.Reader, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", ct)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	return w
}
func expect(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("want %d, got %d: %s", status, w.Code, w.Body)
	}
}
func (h *harness) login(id string) *http.Cookie {
	v := url.Values{"username": {id}, "password": {"synthetic-password"}}
	w := h.request("POST", "/login", "application/x-www-form-urlencoded", strings.NewReader(v.Encode()), nil)
	expect(h.t, w, 303)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		h.t.Fatal("invalid session cookie")
	}
	return cookies[0]
}
func (h *harness) upload(user *http.Cookie, data, skill, org string) string {
	var body bytes.Buffer
	m := multipart.NewWriter(&body)
	w, e := m.CreateFormFile("file", "certificate.pdf")
	if e != nil {
		h.t.Fatal(e)
	}
	io.WriteString(w, data)
	m.WriteField("issuer_org", org)
	m.WriteField("skills", skill)
	m.Close()
	res := h.request("POST", "/upload", m.FormDataContentType(), &body, user)
	expect(h.t, res, 200)
	var sub Submission
	if e = json.Unmarshal(res.Body.Bytes(), &sub); e != nil || sub.ID == "" {
		h.t.Fatal("missing submission ID")
	}
	return sub.ID
}
func (h *harness) approve(cookie *http.Cookie, id string) *httptest.ResponseRecorder {
	return h.request("POST", "/approve", "application/json", strings.NewReader(fmt.Sprintf("{\"id\":%q}", id)), cookie)
}

func TestPhase1OwnershipAndConcurrentApproval(t *testing.T) {
	h := setup(t)
	alice, bob, dean, other, employer := h.login("alice"), h.login("bob"), h.login("dean"), h.login("other"), h.login("employer")
	id := h.upload(alice, "alice evidence", "Go", "uni-a")
	otherID := h.upload(bob, "bob evidence", "Music", "uni-b")
	for _, x := range []struct {
		path   string
		cookie *http.Cookie
		code   int
	}{{"/documents/" + id, dean, 200}, {"/documents/" + id, other, 404}, {"/documents/" + id, bob, 403}, {"/documents/" + otherID, dean, 404}, {"/uploads/certificate.pdf", dean, 404}, {"/pending-files", bob, 403}} {
		w := h.request("GET", x.path, "", nil, x.cookie)
		expect(t, w, x.code)
		if x.code == 200 && w.Body.String() != "alice evidence" {
			t.Fatal("incorrect evidence")
		}
	}
	for _, route := range []string{"/all-files", "/pending-files"} {
		c := bob
		if route == "/pending-files" {
			c = other
		}
		w := h.request("GET", route, "", nil, c)
		expect(t, w, 200)
		if strings.Contains(w.Body.String(), id) || !strings.Contains(w.Body.String(), otherID) {
			t.Fatal("incorrect scope", route)
		}
	}
	expect(t, h.approve(other, id), 404)
	var wg sync.WaitGroup
	result := make(chan int, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); result <- h.approve(dean, id).Code }()
	}
	wg.Wait()
	close(result)
	for status := range result {
		if status != 200 {
			t.Fatalf("retry returned %d", status)
		}
	}
	st := h.s.snapshot()
	if len(st.Chain) != 2 || st.Submissions[id].ApprovedBy != "dean" || st.Submissions[id].Status != "committed" {
		t.Fatal("incorrect commit")
	}
	expect(t, h.approve(other, otherID), 200)
	for _, x := range []struct {
		c             *http.Cookie
		want, exclude string
	}{{alice, "go", "music"}, {bob, "music", "go"}} {
		w := h.request("GET", "/student/dashboard", "", nil, x.c)
		expect(t, w, 200)
		var d StudentDashboardData
		json.Unmarshal(w.Body.Bytes(), &d)
		if len(d.MySkills) != 1 || d.MySkills[0] != x.want {
			t.Fatalf("dashboard skills %v", d.MySkills)
		}
	}
	expect(t, h.request("GET", "/verify/hash/"+digest([]byte("alice evidence")), "", nil, employer), 200)
	expect(t, h.request("GET", "/verify/hash/"+digest([]byte("different")), "", nil, employer), 404)
	expect(t, h.request("POST", "/logout", "", nil, alice), 303)
	expect(t, h.request("GET", "/all-files", "", nil, alice), 401)
	if _, e := openStore(h.s.dir, t.TempDir()); e == nil {
		t.Fatal("second writer allowed")
	}
	h.s.lock.Close()
	reopened, e := openStore(h.s.dir, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.lock.Close()
	if b, ok := (localLedger{reopened}).Read(id); !ok || b.IssuerOrg != "uni-a" {
		t.Fatal("lost commit across restart")
	}
}

type unavailableLedger struct{ Ledger }

func (unavailableLedger) Issue(id, org string) (Block, error) {
	return Block{}, errors.New("simulated interruption")
}
func TestPhase1WriteFailureAndIntentRecovery(t *testing.T) {
	h := setup(t)
	issuer := h.login("dean")
	id := h.upload(h.login("alice"), "evidence", "Go", "uni-a")
	// An unavailable snapshot destination must never result in a successful approval.
	path := filepath.Join(h.s.dir, "state.enc")
	if e := os.Rename(path, path+".backup"); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	expect(t, h.approve(issuer, id), 503)
	if h.s.snapshot().Submissions[id].Status != "pending" {
		t.Fatal("failed write published")
	}
	if e := os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(path+".backup", path); e != nil {
		t.Fatal(e)
	}
	h.app.ledger = unavailableLedger{h.app.ledger}
	expect(t, h.approve(issuer, id), 503)
	if h.s.snapshot().Submissions[id].Status != "issuing" {
		t.Fatal("intent lost")
	}
	h.s.lock.Close()
	s, e := openStore(h.s.dir, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.lock.Close()
	h.s = s
	h.app = newApp(s)
	h.r = h.app.router()
	expect(t, h.approve(h.login("dean"), id), 200)
	if len(s.snapshot().Chain) != 2 {
		t.Fatal("unexpected ledger length")
	}
}
func TestPhase1SessionBoundaries(t *testing.T) {
	h := setup(t)
	c := h.login("dean")
	req := httptest.NewRequest("GET", "/pending-files", nil)
	req.Header.Set("Role", "issuer")
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	expect(t, w, 401)
	req = httptest.NewRequest("POST", "/approve", nil)
	req.Header.Set("Origin", "https://other.example")
	req.AddCookie(c)
	w = httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	expect(t, w, 403)
	expect(t, h.request("GET", "/issuer", "", nil, c), 200)
	expect(t, h.request("GET", "/holder", "", nil, c), 403)
	expect(t, h.request("GET", "/issuer", "", nil, nil), 303)
}
