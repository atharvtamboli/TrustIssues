package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
)

var errMissing = errors.New("record unavailable")
var errDuplicate = errors.New("document already submitted")
var errLedgerOffline = errors.New("ledger unavailable")

type Ledger interface {
	Issue(id, org string) (Block, error)
	Lookup(ref string) (Block, bool, error)
	Revoke(id, org string) error
	Backend() string
}
type localLedger struct{ s *Store }

func (l localLedger) Backend() string { return "local" }
func commitment(sub Submission, fileHash string) string {
	// v1: every UTF-8 field is prefixed with its unsigned 32-bit big-endian length.
	h := sha256.New()
	fields := []string{"fatbot/credential/v1", sub.Salt, sub.ID, sub.HolderID, sub.IssuerOrg, sub.FileName, fileHash}
	for _, v := range append(fields, sub.Skills...) {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(v)))
		h.Write(n[:])
		h.Write([]byte(v))
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (l localLedger) Read(ref string) (Block, bool) {
	st := l.s.snapshot()
	for _, b := range st.Chain {
		if b.Index == 0 || b.Status == "revoked" {
			continue
		}
		if strings.EqualFold(b.Hash, ref) || strings.EqualFold(b.FileHash, ref) || b.CredentialID == ref {
			sub := st.Submissions[b.CredentialID]
			if sub.Status == "revoked" {
				b.Status = "revoked"
				b.Receipt = sub.RevokeTx
			}
			return b, true
		}
	}
	return Block{}, false
}
func (l localLedger) Lookup(ref string) (Block, bool, error) {
	b, ok := l.Read(ref)
	if ok && (b.Backend == "fabric" || l.s.snapshot().Submissions[b.CredentialID].Status == "revoking") {
		return Block{}, false, errLedgerOffline
	}
	return b, ok, nil
}
func recordIssue(st *State, id, org, backend, receipt string) (Block, error) {
	sub, ok := st.Submissions[id]
	if !ok || sub.IssuerOrg != org {
		return Block{}, errMissing
	}
	if sub.Source != backend && !(sub.Source == "" && backend == "local") {
		return Block{}, errLedgerOffline
	}
	for _, b := range st.Chain {
		if b.CredentialID == id && b.Status != "revoked" {
			return b, nil
		}
	}
	if sub.Status != "issuing" || sub.Source != backend {
		return Block{}, errors.New("approval intent backend mismatch")
	}
	prev := st.Chain[len(st.Chain)-1]
	b := Block{Index: prev.Index + 1, Timestamp: utcNow(), PrevHash: prev.Hash, CredentialID: id, IssuerOrg: org, FileName: sub.FileName, FileHash: sub.FileHash, Skills: sub.Skills, Role: "issuer", Status: "verified", Backend: backend, Commitment: commitment(sub, sub.FileHash), Receipt: receipt}
	b.Hash = calculateHash(b)
	st.Chain = append(st.Chain, b)
	sub.Status = "committed"
	sub.Envelope = nil
	sub.PendingTx = ""
	st.Submissions[id] = sub
	return b, nil
}
func (l localLedger) Issue(id, org string) (Block, error) {
	var result Block
	e := l.s.update(func(st *State) error { var e error; result, e = recordIssue(st, id, org, "local", ""); return e })
	return result, e
}
func recordRevoke(st *State, id, org, backend, tx string) error {
	sub, ok := st.Submissions[id]
	if !ok || sub.IssuerOrg != org {
		return errMissing
	}
	if sub.Source != backend && !(sub.Source == "" && backend == "local") {
		return errLedgerOffline
	}
	if sub.Status == "revoked" {
		return nil
	}
	if sub.Status != "committed" && sub.Status != "revoking" {
		return errors.New("credential not issued")
	}
	prev := st.Chain[len(st.Chain)-1]
	b := Block{Index: prev.Index + 1, Timestamp: utcNow(), PrevHash: prev.Hash, CredentialID: id, IssuerOrg: org, Role: "issuer", Status: "revoked", Backend: backend, Receipt: tx}
	b.Hash = calculateHash(b)
	st.Chain = append(st.Chain, b)
	sub.Status = "revoked"
	sub.RevokeTx = tx
	if tx == "" {
		sub.RevokeTx = b.Hash
	}
	sub.Envelope = nil
	sub.PendingTx = ""
	st.Submissions[id] = sub
	return nil
}
func (l localLedger) Revoke(id, org string) error {
	return l.s.update(func(st *State) error { return recordRevoke(st, id, org, "local", "") })
}
