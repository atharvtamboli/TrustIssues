package main

import (
	"encoding/json"
	"log"

	"fatbot/credential/registry"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type CredentialContract struct{ contractapi.Contract }

func response(r *registry.Record, err error) (string, error) {
	if err != nil {
		return "", err
	}
	b, e := json.Marshal(r)
	return string(b), e
}
func (c *CredentialContract) ReadCredential(ctx contractapi.TransactionContextInterface, id string) (string, error) {
	return response(registry.Read(ctx.GetStub(), id))
}
func transition(ctx contractapi.TransactionContextInterface, action, id, commitment string) (string, error) {
	identity := ctx.GetClientIdentity()
	org, err := identity.GetMSPID()
	if err != nil {
		return "", err
	}
	val, ok, err := identity.GetAttributeValue("fatbot.issuer")
	if err != nil {
		return "", err
	}
	return response(registry.Write(ctx.GetStub(), org, ok && val == "true", action, id, commitment))
}
func (c *CredentialContract) IssueCredential(ctx contractapi.TransactionContextInterface, id, commitment string) (string, error) {
	return transition(ctx, "issue", id, commitment)
}
func (c *CredentialContract) RevokeCredential(ctx contractapi.TransactionContextInterface, id string) (string, error) {
	return transition(ctx, "revoke", id, "")
}
func main() {
	cc, e := contractapi.NewChaincode(&CredentialContract{})
	if e != nil {
		log.Fatal(e)
	}
	if e = cc.Start(); e != nil {
		log.Fatal(e)
	}
}
