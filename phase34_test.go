package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func shared(h *harness, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.r.ServeHTTP(w, req)
	return w
}
func outcome(t *testing.T, w *httptest.ResponseRecorder, status int, want string) {
	t.Helper()
	expect(t, w, status)
	var data struct{ Outcome string }
	if json.Unmarshal(w.Body.Bytes(), &data) != nil || data.Outcome != want {
		t.Fatalf("want %s, got %s", want, w.Body)
	}
}
func TestCommitmentVector(t *testing.T) {
	s := Submission{ID: "id", Salt: "salt", HolderID: "h", IssuerOrg: "o", PendingFile: PendingFile{FileName: "f", Skills: []string{"Go"}}}
	if got := commitment(s, "digest"); got != "fff8e33ef36e4a8b97c9057c9910b95503e7cca524998896720fee0121db4490" {
		t.Fatal(got)
	}
}
func TestCredentialPresentationLifecycle(t *testing.T) { h := setup(t); credentialFlow(t, h, false) }
func credentialFlow(t *testing.T, h *harness, live bool) {
	alice, bob, dean, other, employer := h.login("alice"), h.login("bob"), h.login("dean"), h.login("other"), h.login("employer")
	id := h.upload(alice, "degree", "Go,Math", "uni-a")
	expect(t, h.approve(dean, id), 200)
	check := func(want string) {
		t.Helper()
		fn := func() error {
			w := h.request("GET", "/verify/hash/"+id, "", nil, employer)
			var data struct{ Outcome string }
			_ = json.Unmarshal(w.Body.Bytes(), &data)
			if data.Outcome != want {
				return errors.New(w.Body.String())
			}
			return nil
		}
		end := time.Now().Add(15 * time.Second)
		for {
			e := fn()
			if e == nil {
				return
			}
			if !live || time.Now().After(end) {
				t.Fatal(e)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	check("active")
	second := h.upload(alice, "other degree", "Math", "uni-a")
	expect(t, h.approve(dean, second), 200)
	expect(t, h.request("POST", "/credentials/"+id+"/share", "", nil, bob), 404)
	w := h.request("POST", "/credentials/"+id+"/share", "", nil, alice)
	expect(t, w, 200)
	var link struct{ Path string }
	json.Unmarshal(w.Body.Bytes(), &link)
	_, token, ok := strings.Cut(link.Path, "#")
	if !ok {
		t.Fatal("missing token")
	}
	outcome(t, shared(h, "/present/status?digest="+digest([]byte("degree")), token), 200, "matching")
	outcome(t, shared(h, "/present/status?digest="+digest([]byte("degreE")), token), 200, "altered")
	expect(t, shared(h, "/present/evidence", h.app.shareToken(id, time.Now().Unix()-1)), 410)
	expect(t, shared(h, "/present/evidence", token+"X"), 403)
	w = shared(h, "/present/evidence", token)
	expect(t, w, 200)
	if w.Body.String() != "degree" {
		t.Fatal("wrong document")
	}
	expect(t, h.request("POST", "/credentials/"+id+"/revoke", "", nil, other), 404)
	for i := 0; i < 2; i++ {
		expect(t, h.request("POST", "/credentials/"+id+"/revoke", "", nil, dean), 200)
	}
	check("revoked")
	outcome(t, shared(h, "/present/status", token), 200, "revoked")
	expect(t, shared(h, "/present/evidence", token), 410)
	for _, x := range []struct {
		cookie *http.Cookie
		want   int
	}{{alice, 1}, {bob, 0}} {
		w = h.request("GET", "/student/dashboard", "", nil, x.cookie)
		expect(t, w, 200)
		var d StudentDashboardData
		json.Unmarshal(w.Body.Bytes(), &d)
		if len(d.MySkills) != x.want || (x.want == 1 && d.MySkills[0] != "math") {
			t.Fatal(d.MySkills)
		}
	}
	outcome(t, h.request("GET", "/verify/hash/"+strings.Repeat("d", 64), "", nil, employer), 404, "unknown")
	if live {
		b, _ := (localLedger{h.s}).Read(id)
		st := h.s.snapshot()
		for _, issued := range st.Chain {
			if issued.CredentialID == id && issued.Status != "revoked" {
				t.Logf("credential=%s issue_tx=%s revoke_tx=%s", id, issued.Receipt, b.Receipt)
			}
		}
	}
}
func TestFabricReconcileAndFailClosed(t *testing.T) {
	h := setup(t)
	cfg := fabricConfig{Reader: "partner", Issuers: map[string]signer{"uni-a": {Profile: "university", MSP: "Org1MSP"}}}
	f := &fabricLedger{localLedger: localLedger{h.s}, cfg: cfg}
	h.app.ledger = f
	var record *credentialRecord
	var prepared, submitted int
	down := false
	f.call = func(profile, op, fn string, args []string, envelope []byte) (gatewayReply, error) {
		switch op {
		case "read":
			if profile == "partner" && down {
				return gatewayReply{}, errors.New("peer down")
			}
			return gatewayReply{Record: record}, nil
		case "prepare":
			prepared++
			return gatewayReply{Tx: "tx-id", Envelope: []byte("endorsed")}, nil
		case "submit":
			submitted++
			for _, s := range h.s.snapshot().Submissions {
				if s.PendingTx != "tx-id" || string(s.Envelope) != "endorsed" {
					t.Fatal("missing durable intent")
				}
				record = &credentialRecord{ID: s.ID, MSP: "Org1MSP", Commitment: commitment(s, s.FileHash), Status: "active", IssueTx: "tx-id"}
			}
			return gatewayReply{}, errors.New("commit reply lost")
		}
		return gatewayReply{}, errors.New("unexpected operation")
	}
	id := h.upload(h.login("alice"), "cert", "Go", "uni-a")
	dean := h.login("dean")
	expect(t, h.approve(dean, id), 503)
	expect(t, h.approve(dean, id), 200)
	if prepared != 1 || submitted != 1 || len(h.s.snapshot().Chain) != 2 {
		t.Fatal(prepared, submitted)
	}
	down = true
	outcome(t, h.request("GET", "/verify/hash/"+id, "", nil, h.login("employer")), 503, "unavailable")
}
func TestLiveFabricPortals(t *testing.T) {
	path := os.Getenv("FATBOT_LIVE_TEST_CONFIG")
	if path == "" {
		t.Skip("live two-peer configuration required")
	}
	h := setup(t)
	f, e := openFabric(h.s, path)
	if e != nil {
		t.Fatal(e)
	}
	f.cfg.Issuers = map[string]signer{"uni-a": f.cfg.Issuers["Demo University"], "uni-b": f.cfg.Issuers["Partner College"]}
	h.app.ledger = f
	credentialFlow(t, h, true)
	id := h.s.snapshot().Order[1]
	if _, e := f.call(f.cfg.Issuers["uni-b"].Profile, "prepare", "RevokeCredential", []string{id}, nil); e == nil {
		t.Fatal("contract endorsed cross-institution revocation")
	}
	if _, e := f.call(f.cfg.Reader, "prepare", "IssueCredential", []string{randomID(), randomID()}, nil); e == nil {
		t.Fatal("contract endorsed unauthorized reader")
	}
	if _, e := f.call(f.cfg.Issuers["uni-a"].Profile, "prepare", "IssueCredential", []string{id, randomID()}, nil); e == nil {
		t.Fatal("contract endorsed duplicate ID")
	}
	if _, e := f.call(f.cfg.Reader, "read", "", []string{id}, nil); e != nil {
		t.Fatalf("partner unavailable after denial tests: %v", e)
	}
}

func TestFabricRetryReusesEndorsedEnvelope(t *testing.T) {
	h := setup(t)
	f := &fabricLedger{localLedger: localLedger{h.s}, cfg: fabricConfig{Reader: "partner", Issuers: map[string]signer{"uni-a": {Profile: "university", MSP: "Org1MSP"}}}}
	h.app.ledger = f
	alice, dean := h.login("alice"), h.login("dean")
	id := h.upload(alice, "original", "Go", "uni-a")
	var record *credentialRecord
	var operations []string
	attempts := 0
	f.call = func(profile, op, fn string, args []string, envelope []byte) (gatewayReply, error) {
		operations = append(operations, op)
		switch op {
		case "read":
			return gatewayReply{Record: record}, nil
		case "prepare":
			return gatewayReply{Tx: "tx-one", Envelope: []byte("original envelope")}, nil
		case "status":
			if len(args) != 1 || args[0] != "tx-one" {
				t.Fatal(args)
			}
			return gatewayReply{}, nil
		case "submit":
			attempts++
			if string(envelope) != "original envelope" {
				t.Fatal("resubmission changed payload")
			}
			if attempts == 1 {
				return gatewayReply{}, errors.New("transport interrupted")
			}
			sub := h.s.snapshot().Submissions[id]
			record = &credentialRecord{ID: id, MSP: "Org1MSP", Commitment: commitment(sub, sub.FileHash), Status: "active", IssueTx: "tx-one"}
			return gatewayReply{Tx: "tx-one"}, nil
		}
		return gatewayReply{}, errors.New("unexpected request")
	}
	expect(t, h.approve(dean, id), 503)
	expect(t, h.approve(dean, id), 200)
	if strings.Join(operations, ",") != "read,prepare,submit,read,status,submit,read" {
		t.Fatal(operations)
	}
	if len(h.s.snapshot().Chain) != 2 {
		t.Fatal("unexpected receipt count")
	}
}
