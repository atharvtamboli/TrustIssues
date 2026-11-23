// Package registry contains the deterministic state transitions used by chaincode.
package registry

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

type State interface {
	GetState(string) ([]byte, error)
	PutState(string, []byte) error
	SetEvent(string, []byte) error
	GetTxID() string
}
type Record struct {
	ID         string `json:"id"`
	Issuer     string `json:"issuer_msp"`
	Commitment string `json:"commitment"`
	Status     string `json:"status"`
	IssueTx    string `json:"issue_tx"`
	RevokeTx   string `json:"revoke_tx,omitempty"`
}

func valid(s string) bool {
	_, err := hex.DecodeString(s)
	return len(s) == 64 && s == strings.ToLower(s) && err == nil
}
func Read(s State, id string) (*Record, error) {
	if !valid(id) {
		return nil, errors.New("invalid credential ID")
	}
	b, err := s.GetState("credential:v1:" + id)
	if err != nil || len(b) == 0 {
		return nil, err
	}
	var r Record
	err = json.Unmarshal(b, &r)
	return &r, err
}
func Write(s State, org string, authorized bool, action, id, commitment string) (*Record, error) {
	if !authorized || (org != "Org1MSP" && org != "Org2MSP") {
		return nil, errors.New("issuer enrollment required")
	}
	r, err := Read(s, id)
	if err != nil {
		return nil, err
	}
	event := ""
	switch action {
	case "issue":
		if r != nil {
			return nil, errors.New("credential already exists")
		}
		if !valid(commitment) {
			return nil, errors.New("invalid v1 commitment")
		}
		r = &Record{ID: id, Issuer: org, Commitment: commitment, Status: "active", IssueTx: s.GetTxID()}
		event = "CredentialIssued"
	case "revoke":
		if r == nil || r.Issuer != org {
			return nil, errors.New("only original issuing MSP can revoke")
		}
		if r.Status == "revoked" {
			return r, nil
		}
		r.Status = "revoked"
		r.RevokeTx = s.GetTxID()
		event = "CredentialRevoked"
	default:
		return nil, errors.New("unknown transition")
	}
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	if err = s.PutState("credential:v1:"+id, b); err != nil {
		return nil, err
	}
	if err = s.SetEvent(event, b); err != nil {
		return nil, err
	}
	return r, nil
}
