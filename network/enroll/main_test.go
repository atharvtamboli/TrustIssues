package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnrollment(t *testing.T) {
	dir := t.TempDir()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	keybytes, e := x509.MarshalECPrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	for name, p := range map[string]*pem.Block{"ca": {Type: "CERTIFICATE", Bytes: der}, "key": {Type: "EC PRIVATE KEY", Bytes: keybytes}} {
		if e = os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(p), 0600); e != nil {
			t.Fatal(e)
		}
	}
	if e = enroll(filepath.Join(dir, "ca"), filepath.Join(dir, "key"), dir, "issuer"); e != nil {
		t.Fatal(e)
	}
	b, e := loadPEM(filepath.Join(dir, "signer.pem"))
	if e != nil {
		t.Fatal(e)
	}
	cert, e := x509.ParseCertificate(b)
	if e != nil {
		t.Fatal(e)
	}
	if cert.IsCA || cert.Subject.OrganizationalUnit[0] != "client" {
		t.Fatal("incorrect identity")
	}
	for _, ext := range cert.Extensions {
		if ext.Id.String() == "1.2.3.4.5.6.7.8.1" && string(ext.Value) == `{"attrs":{"fatbot.issuer":"true"}}` {
			return
		}
	}
	t.Fatal("issuer attribute missing")
}
