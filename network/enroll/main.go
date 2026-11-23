// enroll provisions a development issuer certificate from the Phase 2 cryptogen CA.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

func loadPEM(path string) ([]byte, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	p, _ := pem.Decode(b)
	if p == nil {
		return nil, errors.New("invalid PEM")
	}
	return p.Bytes, nil
}
func enroll(certPath, keyPath, dir, name string) error {
	data, e := loadPEM(certPath)
	if e != nil {
		return e
	}
	ca, e := x509.ParseCertificate(data)
	if e != nil {
		return e
	}
	data, e = loadPEM(keyPath)
	if e != nil {
		return e
	}
	key, e := x509.ParseECPrivateKey(data)
	if e != nil {
		v, e := x509.ParsePKCS8PrivateKey(data)
		if e != nil {
			return e
		}
		var ok bool
		key, ok = v.(*ecdsa.PrivateKey)
		if !ok {
			return errors.New("expected EC issuer CA key")
		}
	}
	if !ca.IsCA || !key.PublicKey.Equal(ca.PublicKey) {
		return errors.New("incorrect enrollment CA")
	}
	leaf, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return e
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if e != nil {
		return e
	}
	serial.Add(serial, big.NewInt(1))
	now := time.Now()
	expiry := now.Add(180 * 24 * time.Hour)
	if ca.NotAfter.Before(expiry) {
		expiry = ca.NotAfter
	}
	tpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: name, OrganizationalUnit: []string{"client"}}, NotBefore: now.Add(-time.Minute), NotAfter: expiry, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 3, 4, 5, 6, 7, 8, 1}, Value: []byte(`{"attrs":{"fatbot.issuer":"true"}}`)}}}
	der, e := x509.CreateCertificate(rand.Reader, tpl, ca, &leaf.PublicKey, key)
	if e != nil {
		return e
	}
	parsed, e := x509.ParseCertificate(der)
	if e != nil {
		return e
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if _, e = parsed.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); e != nil {
		return e
	}
	secret, e := x509.MarshalPKCS8PrivateKey(leaf)
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "signer.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: secret}), 0600); e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(dir, "signer.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
}
func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "expected CA certificate, CA key, private output directory and common name")
		os.Exit(2)
	}
	if e := enroll(os.Args[1], os.Args[2], os.Args[3], os.Args[4]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
