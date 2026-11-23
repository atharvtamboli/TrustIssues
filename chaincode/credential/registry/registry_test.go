package registry

import (
	"strings"
	"testing"
)

type memory struct {
	keys   map[string][]byte
	events []string
	tx     string
}

func (s *memory) GetState(k string) ([]byte, error) { return s.keys[k], nil }
func (s *memory) PutState(k string, b []byte) error { s.keys[k] = b; return nil }
func (s *memory) SetEvent(k string, b []byte) error { s.events = append(s.events, k); return nil }
func (s *memory) GetTxID() string                   { return s.tx }
func TestPermissionsAndLifecycle(t *testing.T) {
	s := &memory{keys: map[string][]byte{}, tx: "issue-tx"}
	id := strings.Repeat("a", 64)
	digest := strings.Repeat("b", 64)
	for _, x := range []struct {
		org string
		ok  bool
	}{{"Org1MSP", false}, {"StrangerMSP", true}} {
		if _, e := Write(s, x.org, x.ok, "issue", id, digest); e == nil {
			t.Fatal("unauthorized")
		}
	}
	r, e := Write(s, "Org1MSP", true, "issue", id, digest)
	if e != nil || r.Status != "active" {
		t.Fatal(r, e)
	}
	if _, e = Write(s, "Org1MSP", true, "issue", id, digest); e == nil {
		t.Fatal("duplicate")
	}
	if _, e = Write(s, "Org2MSP", true, "revoke", id, ""); e == nil {
		t.Fatal("cross-org revoke")
	}
	s.tx = "revoke-tx"
	for i := 0; i < 2; i++ {
		r, e = Write(s, "Org1MSP", true, "revoke", id, "")
		if e != nil || r.RevokeTx != "revoke-tx" {
			t.Fatal(r, e)
		}
	}
	r, e = Read(s, id)
	if e != nil || r.IssueTx != "issue-tx" || r.Status != "revoked" || len(s.events) != 2 {
		t.Fatal(r, e, s.events)
	}
}
