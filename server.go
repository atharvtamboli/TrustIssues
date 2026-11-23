package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

func (a *App) router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), sameOrigin)
	r.GET("/", func(c *gin.Context) { c.Redirect(303, "/login") })
	r.GET("/login", asset("login.html"))
	r.GET("/portal.js", asset("portal.js"))
	r.POST("/login", a.login)
	r.POST("/logout", a.logout)
	for _, role := range []string{"holder", "issuer", "verifier"} {
		r.GET("/"+role, a.require(role), asset(role+".html"))
	}
	holder, issuer, verifier := a.require("holder"), a.require("issuer"), a.require("verifier")
	r.GET("/issuers", holder, func(c *gin.Context) { c.JSON(200, a.issuerOrgs(a.store.snapshot())) })
	r.POST("/upload", holder, a.upload)
	r.POST("/approve", issuer, a.approve)
	r.GET("/documents/:id", issuer, a.document)
	r.GET("/pending-files", issuer, func(c *gin.Context) { _, p := visible(a.store.snapshot(), current(c)); c.JSON(200, p) })
	r.GET("/blocks", issuer, func(c *gin.Context) { b, _ := visible(a.store.snapshot(), current(c)); c.JSON(200, b) })
	r.GET("/all-files", holder, func(c *gin.Context) {
		b, p := visible(a.store.snapshot(), current(c))
		c.JSON(200, gin.H{"blockchain": b, "pending": p})
	})
	r.GET("/student/dashboard", holder, func(c *gin.Context) {
		st := a.store.snapshot()
		skills, err := a.activeSkills(st, current(c))
		if err != nil {
			fail(c, 503, "Credential status unavailable")
			return
		}
		c.JSON(200, StudentDashboardData{skills, calculateJobMatches(skills, st.Jobs)})
	})
	a.credentialRoutes(r, holder, issuer, verifier)
	a.careerRoutes(r, holder, verifier)
	r.GET("/jobs", verifier, func(c *gin.Context) { c.JSON(200, a.store.snapshot().Jobs) })
	r.POST("/jobs", verifier, func(c *gin.Context) {
		var job Job
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 65536)
		if c.ShouldBindJSON(&job) != nil || strings.TrimSpace(job.Title) == "" {
			fail(c, 400, "A job title is required")
			return
		}
		u := current(c)
		job.ID = randomID()
		job.Owner = u.ID
		if u.Org != "" && u.Org != "-" {
			job.Company = u.Org
		}
		job.Company = strings.TrimSpace(job.Company)
		if job.Company == "" {
			job.Company = u.ID
		}
		job.Title = strings.TrimSpace(job.Title)
		job.Requirements = uniqueRequirements(job.Requirements)
		if len(job.Requirements) > 50 {
			fail(c, 400, "Use up to 50 distinct requirements")
			return
		}
		job.PostedDate = utcNow()
		err := a.store.update(func(st *State) error { st.Jobs = append(st.Jobs, job); return nil })
		if err != nil {
			storeFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"message": "Job posted successfully", "job": job})
	})
	return r
}
func (a *App) issuerOrgs(st *State) []string {
	seen := map[string]bool{}
	orgs := []string{}
	for _, u := range st.Accounts {
		if u.Role == "issuer" && !seen[u.Org] {
			if f, ok := a.ledger.(*fabricLedger); ok {
				if _, configured := f.cfg.Issuers[u.Org]; !configured {
					continue
				}
			}
			orgs = append(orgs, u.Org)
			seen[u.Org] = true
		}
	}
	sort.Strings(orgs)
	return orgs
}
func visible(st *State, u Account) ([]Block, []Submission) {
	blocks := []Block{}
	pending := []Submission{}
	owns := func(sub Submission) bool {
		return (u.Role == "issuer" && sub.IssuerOrg == u.Org) || (u.Role == "holder" && sub.HolderID == u.ID)
	}
	for _, id := range st.Order {
		sub := st.Submissions[id]
		if owns(sub) && (sub.Status == "pending" || sub.Status == "issuing") {
			// Only the submission summary belongs in portal responses.
			sub.Envelope, sub.Salt, sub.PendingTx = nil, "", ""
			pending = append(pending, sub)
		}
	}
	for _, b := range st.Chain {
		if sub, ok := st.Submissions[b.CredentialID]; ok && owns(sub) && b.Status != "revoked" {
			if sub.Status == "revoked" || sub.Status == "revoking" {
				b.Status = sub.Status
			}
			blocks = append(blocks, b)
		}
	}
	return blocks, pending
}
func storeFailure(c *gin.Context, err error) {
	log.Printf("storage operation: %v", err)
	fail(c, 503, "Storage unavailable. Retry the operation.")
}
func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func (a *App) upload(c *gin.Context) {
	const limit = 16 << 20
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit+(1<<20))
	header, err := c.FormFile("file")
	if err != nil {
		fail(c, 400, "Select a file up to 16 MiB")
		return
	}
	org := strings.TrimSpace(c.PostForm("issuer_org"))
	valid := false
	for _, name := range a.issuerOrgs(a.store.snapshot()) {
		if org == name {
			valid = true
			break
		}
	}
	if !valid {
		fail(c, 400, "Select an issuing organization")
		return
	}
	f, err := header.Open()
	if err != nil {
		storeFailure(c, err)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		storeFailure(c, err)
		return
	}
	if len(data) == 0 || len(data) > limit {
		fail(c, 400, "Select a nonempty file up to 16 MiB")
		return
	}
	skills := []string{}
	for _, skill := range strings.Split(c.PostForm("skills"), ",") {
		if v := strings.TrimSpace(skill); v != "" {
			skills = append(skills, v)
		}
	}
	sub := Submission{ID: randomID(), HolderID: current(c).ID, IssuerOrg: org, Status: "pending", PendingFile: PendingFile{FileName: header.Filename, FileHash: digest(data), Timestamp: utcNow(), Skills: skills}}
	path := filepath.Join(a.store.dir, "objects", sub.ID)
	g, err := gcmFor(a.store.key)
	if err != nil {
		storeFailure(c, err)
		return
	}
	nonce := make([]byte, g.NonceSize())
	rand.Read(nonce)
	if err = atomicWrite(path, g.Seal(nonce, nonce, data, nil)); err != nil {
		storeFailure(c, err)
		return
	}
	err = a.store.update(func(st *State) error {
		for _, existing := range st.Submissions {
			if existing.FileHash == sub.FileHash {
				return errDuplicate
			}
		}
		for _, b := range st.Chain {
			if b.FileHash == sub.FileHash {
				return errDuplicate
			}
		}
		st.Submissions[sub.ID] = sub
		st.Order = append(st.Order, sub.ID)
		return nil
	})
	if err != nil {
		os.Remove(path)
		if errors.Is(err, errDuplicate) {
			fail(c, 409, err.Error())
			return
		}
		storeFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"message": "Submitted for approval", "id": sub.ID, "file_name": sub.FileName, "status": sub.Status})
}
func (a *App) readObject(sub Submission) ([]byte, error) {
	data, e := os.ReadFile(filepath.Join(a.store.dir, "objects", sub.ID))
	if e != nil {
		return nil, e
	}
	data, e = unseal(data, a.store.key)
	if e != nil {
		return nil, e
	}
	if digest(data) != sub.FileHash {
		return nil, errors.New("document digest mismatch")
	}
	return data, nil
}
func (a *App) document(c *gin.Context) {
	sub, ok := a.store.snapshot().Submissions[c.Param("id")]
	if !ok || sub.IssuerOrg != current(c).Org {
		fail(c, 404, "Document unavailable")
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
func (a *App) approve(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	if c.ShouldBindJSON(&req) != nil || req.ID == "" {
		fail(c, 400, "Submission ID required")
		return
	}
	u := current(c)
	sub, ok := a.store.snapshot().Submissions[req.ID]
	if !ok || sub.IssuerOrg != u.Org {
		fail(c, 404, "Submission unavailable")
		return
	}
	if sub.Status == "revoked" || sub.Status == "revoking" {
		fail(c, 409, "Credential is revoked or awaiting revocation")
		return
	}
	if sub.Status != "committed" {
		if _, e := a.readObject(sub); e != nil {
			log.Printf("approval evidence: %v", e)
			c.JSON(503, gin.H{"status": "failed", "error": "Evidence unavailable. Approval was not submitted."})
			return
		}
		e := a.store.update(func(st *State) error {
			s := st.Submissions[req.ID]
			if s.Status == "pending" {
				s.Status = "issuing"
				s.Salt = randomID()
				s.Source = a.ledger.Backend()
				s.ApprovedBy = u.ID
				st.Submissions[s.ID] = s
			}
			return nil
		})
		if e != nil {
			storeFailure(c, e)
			return
		}
	}
	if sub.Status == "issuing" && sub.Salt == "" {
		if e := a.store.update(func(st *State) error {
			v := st.Submissions[sub.ID]
			if v.Salt == "" {
				v.Salt = randomID()
				v.Source = a.ledger.Backend()
				st.Submissions[v.ID] = v
			}
			return nil
		}); e != nil {
			storeFailure(c, e)
			return
		}
	}
	b, e := a.ledger.Issue(sub.ID, u.Org)
	if e != nil {
		log.Printf("approval %s: %v", sub.ID, e)
		c.JSON(503, gin.H{"id": sub.ID, "status": "unknown", "error": "Approval outcome unknown. Retry with the same submission ID."})
		return
	}
	c.JSON(200, gin.H{"id": sub.ID, "status": "committed", "message": "Credential issued", "block": b})
}

func (a *App) activeSkills(st *State, u Account) ([]string, error) {
	blocks, err := a.skillCredentials(st, u)
	return getStudentSkills(blocks), err
}

func (a *App) skillCredentials(st *State, u Account) ([]Block, error) {
	b, _ := visible(st, u)
	for i, block := range b {
		if block.Backend == "fabric" {
			v, ok, e := a.ledger.Lookup(block.CredentialID)
			if e != nil || !ok {
				return nil, errLedgerOffline
			}
			if v.Status == "revoked" {
				b[i].Status = "revoked"
			}
		}
	}
	return b, nil
}
