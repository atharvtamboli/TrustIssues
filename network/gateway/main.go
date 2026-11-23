// gateway executes one request against an explicitly configured organization.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	protoGateway "github.com/hyperledger/fabric-protos-go-apiv2/gateway"
	msp "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	fabpeer "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Profile struct{ MSP, Endpoint, ServerName, CA, Cert, Key string }
type Request struct {
	Operation, Function string
	Args                []string
	Envelope            []byte
}
type Reply struct {
	Record   json.RawMessage `json:"record,omitempty"`
	Envelope []byte          `json:"envelope,omitempty"`
	Tx       string          `json:"tx,omitempty"`
	Invalid  bool            `json:"invalid,omitempty"`
}

func execute() (Reply, error) {
	var result Reply
	if len(os.Args) != 2 {
		return result, errors.New("expected signer profile path")
	}
	raw, e := os.ReadFile(os.Args[1])
	if e != nil {
		return result, e
	}
	var p Profile
	if e = json.Unmarshal(raw, &p); e != nil {
		return result, e
	}
	ca, e := os.ReadFile(p.CA)
	if e != nil {
		return result, e
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return result, errors.New("invalid CA")
	}
	conn, e := grpc.NewClient(p.Endpoint, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: pool, ServerName: p.ServerName, MinVersion: tls.VersionTLS12})))
	if e != nil {
		return result, e
	}
	defer conn.Close()
	pem, e := os.ReadFile(p.Cert)
	if e != nil {
		return result, e
	}
	cert, e := identity.CertificateFromPEM(pem)
	if e != nil {
		return result, e
	}
	id, e := identity.NewX509Identity(p.MSP, cert)
	if e != nil {
		return result, e
	}
	pem, e = os.ReadFile(p.Key)
	if e != nil {
		return result, e
	}
	key, e := identity.PrivateKeyFromPEM(pem)
	if e != nil {
		return result, e
	}
	sign, e := identity.NewPrivateKeySign(key)
	if e != nil {
		return result, e
	}
	gw, e := client.Connect(id, client.WithSign(sign), client.WithClientConnection(conn), client.WithEvaluateTimeout(10*time.Second), client.WithEndorseTimeout(20*time.Second), client.WithSubmitTimeout(15*time.Second), client.WithCommitStatusTimeout(30*time.Second))
	if e != nil {
		return result, e
	}
	defer gw.Close()
	var req Request
	if e = json.NewDecoder(os.Stdin).Decode(&req); e != nil {
		return result, e
	}
	contract := gw.GetNetwork("credentials").GetContract("credential")
	switch req.Operation {
	case "read":
		data, e := contract.Evaluate("ReadCredential", client.WithArguments(req.Args...), client.WithEndorsingOrganizations(p.MSP))
		if e != nil {
			return result, e
		}
		result.Record = data
	case "status":
		if len(req.Args) != 1 {
			return result, errors.New("missing transaction ID")
		}
		serialized, e := proto.Marshal(&msp.SerializedIdentity{Mspid: p.MSP, IdBytes: id.Credentials()})
		if e != nil {
			return result, e
		}
		payload, e := proto.Marshal(&protoGateway.CommitStatusRequest{ChannelId: "credentials", TransactionId: req.Args[0], Identity: serialized})
		if e != nil {
			return result, e
		}
		digest := sha256.Sum256(payload)
		signature, e := sign(digest[:])
		if e != nil {
			return result, e
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		reply, e := protoGateway.NewGatewayClient(conn).CommitStatus(ctx, &protoGateway.SignedCommitStatusRequest{Request: payload, Signature: signature})
		if status.Code(e) == codes.DeadlineExceeded {
			return result, nil
		}
		if e != nil {
			return result, e
		}
		result.Tx = req.Args[0]
		result.Invalid = reply.Result != fabpeer.TxValidationCode_VALID
	case "prepare":
		proposal, e := contract.NewProposal(req.Function, client.WithArguments(req.Args...))
		if e != nil {
			return result, e
		}
		tx, e := proposal.Endorse()
		if e != nil {
			return result, e
		}
		result.Tx = tx.TransactionID()
		result.Envelope, e = tx.Bytes()
		if e != nil {
			return result, e
		}
	case "submit":
		tx, e := gw.NewTransaction(req.Envelope)
		if e != nil {
			return result, e
		}
		commit, e := tx.Submit()
		if e != nil {
			return result, e
		}
		status, e := commit.Status()
		if e != nil {
			return result, e
		}
		result.Invalid = !status.Successful
		result.Tx = commit.TransactionID()
	default:
		return result, errors.New("unsupported gateway operation")
	}
	return result, nil
}
func main() {
	result, e := execute()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	if e = json.NewEncoder(os.Stdout).Encode(result); e != nil {
		os.Exit(1)
	}
}
