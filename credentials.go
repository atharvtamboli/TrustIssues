package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func validDigest(s string) bool { _, e := hex.DecodeString(s); return len(s) == 64 && e == nil }
func (a *App) shareToken(id string, exp int64) string {
	body := id + "." + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, a.store.key)
	mac.Write([]byte("fatbot/share/v1/" + body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (a *App) sharedID(c *gin.Context) (string, bool) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 3 || !validDigest(parts[0]) {
		fail(c, 403, "Invalid share link")
		return "", false
	}
	exp, e := strconv.ParseInt(parts[1], 10, 64)
	if e != nil || !hmac.Equal([]byte(token), []byte(a.shareToken(parts[0], exp))) {
		fail(c, 403, "Invalid share link")
		return "", false
	}
	if time.Now().Unix() >= exp {
		fail(c, 410, "Share link expired")
		return "", false
	}
	return parts[0], true
}
func (a *App) credentialRoutes(r *gin.Engine, holder, issuer, verifier gin.HandlerFunc) {
	r.GET("/presentation", asset("presentation.html"))
	r.GET("/qr.js", asset("qr.js"))
	r.GET("/credential-ui.js", asset("credential-ui.js"))
	r.GET("/verify/hash/:hash", verifier, func(c *gin.Context) { a.verify(c, c.Param("hash"), false) })
	r.GET("/present/status", func(c *gin.Context) {
		if id, ok := a.sharedID(c); ok {
			a.verify(c, id, true)
		}
	})
	r.GET("/present/evidence", func(c *gin.Context) {
		id, ok := a.sharedID(c)
		if !ok {
			return
		}
		b, ok, e := a.ledger.Lookup(id)
		if e != nil {
			fail(c, 503, "Ledger unavailable")
			return
		}
		if !ok || b.Status == "revoked" {
			fail(c, 410, "Credential unavailable")
			return
		}
		if b.Commitment != "" {
			sub := a.store.snapshot().Submissions[id]
			if commitment(sub, sub.FileHash) != b.Commitment {
				fail(c, 409, "Evidence differs from the ledger commitment")
				return
			}
		}
		a.sendEvidence(c, id)
	})
	r.GET("/my-evidence/:id", holder, func(c *gin.Context) {
		id := c.Param("id")
		if a.store.snapshot().Submissions[id].HolderID != current(c).ID {
			fail(c, 404, "Document unavailable")
			return
		}
		a.sendEvidence(c, id)
	})
	r.POST("/credentials/:id/share", holder, func(c *gin.Context) {
		id := c.Param("id")
		sub := a.store.snapshot().Submissions[id]
		if sub.HolderID != current(c).ID || sub.Status != "committed" {
			fail(c, 404, "Active credential unavailable")
			return
		}
		b, ok, e := a.ledger.Lookup(id)
		if e != nil {
			fail(c, 503, "Ledger unavailable")
			return
		}
		if !ok || b.Status == "revoked" {
			fail(c, 409, "Credential unavailable")
			return
		}
		exp := time.Now().Add(time.Hour)
		c.JSON(200, gin.H{"path": "/presentation#" + a.shareToken(id, exp.Unix()), "expires": exp.UTC()})
	})
	r.POST("/credentials/:id/revoke", issuer, func(c *gin.Context) {
		id := c.Param("id")
		org := current(c).Org
		sub, ok := a.store.snapshot().Submissions[id]
		if !ok || sub.IssuerOrg != org {
			fail(c, 404, "Credential unavailable")
			return
		}
		if sub.Status != "committed" && sub.Status != "revoking" && sub.Status != "revoked" {
			fail(c, 409, "Credential has not been issued")
			return
		}
		if e := a.ledger.Revoke(id, org); e != nil {
			fail(c, 503, "Revocation unconfirmed. Retry with the same credential ID.")
			return
		}
		c.JSON(200, gin.H{"status": "revoked", "transaction_id": a.store.snapshot().Submissions[id].RevokeTx})
	})
}
func (a *App) sendEvidence(c *gin.Context, id string) {
	sub, ok := a.store.snapshot().Submissions[id]
	if !ok {
		fail(c, 404, "Evidence unavailable")
		return
	}
	data, e := a.readObject(sub)
	if e != nil {
		storeFailure(c, e)
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": sub.FileName}))
	c.Data(200, "application/octet-stream", data)
}
func (a *App) verify(c *gin.Context, ref string, shared bool) {
	ref = strings.ToLower(strings.TrimSpace(ref))
	provided, has := c.GetQuery("digest")
	if has && !validDigest(provided) {
		fail(c, 400, "Expected a SHA-256 document digest")
		return
	}
	checked := utcNow()
	b, ok, e := a.ledger.Lookup(ref)
	if e != nil {
		c.JSON(503, gin.H{"outcome": "unavailable", "message": "Ledger unavailable", "checked_at": checked})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"found": false, "outcome": "unknown", "message": "Credential not found", "checked_at": checked})
		return
	}
	outcome, msg := "active", "Credential is active. Select the original file to check its contents."
	if b.CredentialID == "" {
		outcome = "legacy"
		msg = "Historical record with unrecorded issuer identity."
	}
	sub, private := a.store.snapshot().Submissions[b.CredentialID]
	if !has && b.FileHash != "" && ref == b.FileHash {
		provided = ref
		has = true
	}
	if has {
		match := strings.EqualFold(provided, b.FileHash)
		if b.Commitment != "" {
			if !private {
				fail(c, 422, "A holder presentation is required to compare this evidence")
				return
			}
			match = commitment(sub, strings.ToLower(provided)) == b.Commitment
		}
		if !match {
			outcome = "altered"
			msg = "Document differs from the issued credential."
		} else if outcome != "legacy" {
			outcome = "matching"
			msg = "Document matches the active credential."
		}
	}
	if b.Status == "revoked" {
		outcome = "revoked"
		msg = "The issuing institution revoked this credential."
	}
	if b.Status == "revoking" {
		c.JSON(503, gin.H{"outcome": "unavailable", "message": "Revocation awaiting confirmation", "checked_at": checked})
		return
	}
	backend := b.Backend
	if backend == "" {
		backend = "local"
	}
	receipt := b.Receipt
	if receipt == "" {
		receipt = b.Hash
	}
	result := gin.H{"found": true, "outcome": outcome, "status": outcome, "issuer_org": b.IssuerOrg, "credential_id": b.CredentialID, "checked_at": checked, "timestamp": b.Timestamp, "message": msg, "transaction_id": receipt, "ledger": backend}
	if shared && private {
		result["credential_type"] = sub.FileName
		result["skills"] = sub.Skills
	}
	c.JSON(200, result)
}
