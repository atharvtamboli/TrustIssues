package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

type signer struct{ Profile, MSP string }
type fabricConfig struct {
	Executable, Reader string
	Issuers            map[string]signer
}
type credentialRecord struct {
	ID         string `json:"id"`
	MSP        string `json:"issuer_msp"`
	Commitment string `json:"commitment"`
	Status     string `json:"status"`
	IssueTx    string `json:"issue_tx"`
	RevokeTx   string `json:"revoke_tx"`
}
type gatewayReply struct {
	Record   *credentialRecord
	Tx       string
	Envelope []byte
	Invalid  bool
}
type gatewayCall func(profile, op, fn string, args []string, envelope []byte) (gatewayReply, error)
type fabricLedger struct {
	localLedger
	cfg    fabricConfig
	call   gatewayCall
	writes sync.Mutex
}

func (f *fabricLedger) Backend() string { return "fabric" }
func openFabric(s *Store, path string) (*fabricLedger, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	f := &fabricLedger{localLedger: localLedger{s}}
	if e = json.Unmarshal(b, &f.cfg); e != nil {
		return nil, e
	}
	if f.cfg.Reader == "" || len(f.cfg.Issuers) == 0 {
		return nil, errors.New("missing Fabric profiles")
	}
	if _, e = exec.LookPath(f.cfg.Executable); e != nil {
		return nil, e
	}
	seen := map[string]bool{}
	for _, sig := range f.cfg.Issuers {
		if sig.Profile == "" || sig.MSP == "" || seen[sig.MSP] {
			return nil, errors.New("each institution needs a distinct signing MSP")
		}
		seen[sig.MSP] = true
	}
	f.call = func(profile, op, fn string, args []string, envelope []byte) (gatewayReply, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, f.cfg.Executable, profile)
		payload, _ := json.Marshal(struct {
			Operation, Function string
			Args                []string
			Envelope            []byte
		}{op, fn, args, envelope})
		cmd.Stdin = bytes.NewReader(payload)
		var reply gatewayReply
		out, e := cmd.Output()
		if e != nil {
			return reply, fmt.Errorf("gateway %s: %w", op, e)
		}
		e = json.Unmarshal(out, &reply)
		return reply, e
	}
	return f, nil
}
func (f *fabricLedger) Lookup(ref string) (Block, bool, error) {
	b, known := f.Read(ref)
	if known && b.Backend != "fabric" {
		return b, true, nil
	}
	if !known && !validDigest(ref) {
		return Block{}, false, nil
	}
	id := ref
	if known {
		id = b.CredentialID
	}
	res, e := f.call(f.cfg.Reader, "read", "", []string{id}, nil)
	if e != nil {
		return Block{}, false, e
	}
	r := res.Record
	if r == nil {
		if known {
			return Block{}, false, errLedgerOffline
		}
		return Block{}, false, nil
	}
	if r.ID != id || (r.Status != "active" && r.Status != "revoked") {
		return Block{}, false, errLedgerOffline
	}
	if known {
		sub := f.s.snapshot().Submissions[id]
		if f.cfg.Issuers[b.IssuerOrg].MSP != r.MSP || b.Commitment != r.Commitment || ((sub.Status == "revoking" || sub.Status == "revoked") && r.Status != "revoked") {
			return Block{}, false, errLedgerOffline
		}
	} else {
		b = Block{CredentialID: id, IssuerOrg: r.MSP, Backend: "fabric", Commitment: r.Commitment}
	}
	b.Status = r.Status
	b.Receipt = r.IssueTx
	if r.Status == "revoked" {
		b.Receipt = r.RevokeTx
	}
	return b, true, nil
}
func (f *fabricLedger) Issue(id, org string) (Block, error) {
	sub := f.s.snapshot().Submissions[id]
	if sub.Source != "fabric" {
		return f.localLedger.Issue(id, org)
	}
	return f.mutate(id, org, false)
}
func (f *fabricLedger) Revoke(id, org string) error {
	if f.s.snapshot().Submissions[id].Source != "fabric" {
		return f.localLedger.Revoke(id, org)
	}
	_, e := f.mutate(id, org, true)
	return e
}
func (f *fabricLedger) mutate(id, org string, revoke bool) (Block, error) {
	f.writes.Lock()
	defer f.writes.Unlock()
	sub, ok := f.s.snapshot().Submissions[id]
	sig, allowed := f.cfg.Issuers[org]
	if !ok || !allowed || sub.IssuerOrg != org {
		return Block{}, errMissing
	}
	expected := commitment(sub, sub.FileHash)
	fn := "IssueCredential"
	args := []string{id, expected}
	if revoke {
		fn = "RevokeCredential"
		args = []string{id}
		if sub.Status != "committed" && sub.Status != "revoking" && sub.Status != "revoked" {
			return Block{}, errMissing
		}
	}
	prior, e := f.call(sig.Profile, "read", "", []string{id}, nil)
	if e != nil {
		return Block{}, e
	}
	var result Block
	finish := func(r *credentialRecord) error {
		if r.ID != id || r.MSP != sig.MSP || r.Commitment != expected || r.IssueTx == "" {
			return errors.New("credential does not match approval intent")
		}
		return f.s.update(func(st *State) error {
			var err error
			result, err = recordIssue(st, id, org, "fabric", r.IssueTx)
			if err != nil {
				return err
			}
			if r.Status == "revoked" {
				return recordRevoke(st, id, org, "fabric", r.RevokeTx)
			}
			return nil
		})
	}
	if r := prior.Record; r != nil {
		if r.IssuerMismatch(sig.MSP, expected) {
			return Block{}, errors.New("conflicting credential")
		}
		if !revoke || r.Status == "revoked" {
			e = finish(r)
			return result, e
		}
	} else if revoke {
		return Block{}, errLedgerOffline
	}
	if sub.PendingTx != "" {
		status, err := f.call(sig.Profile, "status", "", []string{sub.PendingTx}, nil)
		if err != nil {
			return Block{}, err
		}
		if status.Tx != "" {
			if status.Tx != sub.PendingTx {
				return Block{}, errLedgerOffline
			}
			if status.Invalid {
				return Block{}, f.clearInvalid(id)
			}
			// A valid transaction must become visible on the issuer before reconciliation.
			return Block{}, errors.New("committed transaction awaiting state visibility")
		}
	}
	if len(sub.Envelope) == 0 {
		prepared, e := f.call(sig.Profile, "prepare", fn, args, nil)
		if e != nil {
			return Block{}, e
		}
		if len(prepared.Envelope) == 0 || prepared.Tx == "" {
			return Block{}, errLedgerOffline
		}
		sub.Envelope = prepared.Envelope
		sub.PendingTx = prepared.Tx
		if revoke {
			sub.Status = "revoking"
		}
		if e = f.s.update(func(st *State) error { st.Submissions[id] = sub; return nil }); e != nil {
			return Block{}, e
		}
	}
	receipt, e := f.call(sig.Profile, "submit", "", nil, sub.Envelope)
	if e != nil {
		return Block{}, e
	}
	if receipt.Tx != sub.PendingTx {
		return Block{}, errLedgerOffline
	}
	if receipt.Invalid {
		return Block{}, f.clearInvalid(id)
	}
	final, e := f.call(sig.Profile, "read", "", []string{id}, nil)
	if e != nil {
		return Block{}, e
	}
	if final.Record == nil || (revoke && final.Record.Status != "revoked") {
		return Block{}, errLedgerOffline
	}
	e = finish(final.Record)
	return result, e
}
func (r *credentialRecord) IssuerMismatch(msp, digest string) bool {
	return r.MSP != msp || r.Commitment != digest
}

func (f *fabricLedger) clearInvalid(id string) error {
	err := f.s.update(func(st *State) error {
		v := st.Submissions[id]
		v.Envelope = nil
		v.PendingTx = ""
		st.Submissions[id] = v
		return nil
	})
	if err != nil {
		return err
	}
	return errors.New("transaction invalid; retry with same credential ID")
}
