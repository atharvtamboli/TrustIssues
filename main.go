package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

type Block struct {
	Commitment   string   `json:"commitment,omitempty"`
	Receipt      string   `json:"transaction_id,omitempty"`
	Backend      string   `json:"backend,omitempty"`
	CredentialID string   `json:"credential_id,omitempty"`
	IssuerOrg    string   `json:"issuer_org,omitempty"`
	Index        int      `json:"index"`
	Timestamp    string   `json:"timestamp"`
	FileName     string   `json:"file_name"`
	FileHash     string   `json:"file_hash"`
	PrevHash     string   `json:"prev_hash"`
	Hash         string   `json:"hash"`
	Status       string   `json:"status"` // pending/verified
	Role         string   `json:"role"`   // issuer/holder/verifier
	Skills       []string `json:"skills"` // NEW
}

type PendingFile struct {
	FileName  string   `json:"file_name"`
	FileHash  string   `json:"file_hash"`
	Timestamp string   `json:"timestamp"`
	Skills    []string `json:"skills"` // NEW
}

type Job struct {
	ID           string   `json:"id,omitempty"`
	Company      string   `json:"company,omitempty"`
	Owner        string   `json:"owner,omitempty"`
	Interview    string   `json:"interview,omitempty"`
	Title        string   `json:"title"`
	Location     string   `json:"location"`
	Type         string   `json:"type"`
	Description  string   `json:"description"`
	Requirements []string `json:"requirements"` // This is the list of skills
	PostedDate   string   `json:"posted_date"`
}

// For the new student dashboard endpoint
type StudentDashboardData struct {
	MySkills   []string   `json:"my_skills"`
	JobMatches []JobMatch `json:"job_matches"`
}

type JobMatch struct {
	Job           Job      `json:"job"`
	MatchPercent  int      `json:"match_percent"`
	MissingSkills []string `json:"missing_skills"`
}

func calculateHash(block Block) string {
	// Skills are now part of the hash
	skillsStr := strings.Join(block.Skills, ",")
	record := fmt.Sprintf("%d%s%s%s%s%s", block.Index, block.Timestamp, block.FileName, block.FileHash, block.PrevHash, skillsStr)
	if block.CredentialID != "" {
		block.Hash = ""
		encoded, err := json.Marshal(block)
		if err != nil {
			panic(err)
		}
		record = string(encoded)
	}
	hash := sha256.Sum256([]byte(record))
	return hex.EncodeToString(hash[:])
}

func getStudentSkills(blockchain []Block) []string {
	skillSet := make(map[string]bool)
	// Iterate backwards to get skills from most recent certs first
	for i := len(blockchain) - 1; i >= 0; i-- {
		if blockchain[i].Index == 0 || (blockchain[i].Status == "revoked" || blockchain[i].Status == "revoking") { // Skip Genesis block
			continue
		}
		for _, skill := range blockchain[i].Skills {
			lowercaseSkill := strings.TrimSpace(strings.ToLower(skill))
			if lowercaseSkill != "" {
				skillSet[lowercaseSkill] = true
			}
		}
	}

	// Convert set to list
	uniqueSkills := []string{}
	for skill := range skillSet {
		uniqueSkills = append(uniqueSkills, skill)
	}
	sort.Strings(uniqueSkills)
	return uniqueSkills
}

func calculateJobMatches(mySkills []string, allJobs []Job) []JobMatch {
	matches := []JobMatch{}
	skillMap := make(map[string]bool)
	for _, s := range mySkills {
		skillMap[s] = true
	}

	for _, job := range allJobs {
		var missingSkills []string
		matchCount := 0
		totalRequirements := len(job.Requirements)

		if totalRequirements == 0 {
			matches = append(matches, JobMatch{
				Job:           job,
				MatchPercent:  100, // Or 0, depending on business logic
				MissingSkills: []string{},
			})
			continue
		}

		for _, req := range job.Requirements {
			reqLower := strings.TrimSpace(strings.ToLower(req))
			if _, has := skillMap[reqLower]; has {
				matchCount++
			} else {
				missingSkills = append(missingSkills, req)
			}
		}

		matchPercent := (matchCount * 100) / totalRequirements
		matches = append(matches, JobMatch{
			Job:           job,
			MatchPercent:  matchPercent,
			MissingSkills: missingSkills,
		})
	}
	return matches
}

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("TrustIssues %s\n", strings.TrimSpace(appVersion))
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "help") {
		fmt.Println("TrustIssues — credential and career workspace\n\nUsage:\n  trustissues                 Start the web portal\n  trustissues --version       Print the version\n  trustissues user-add USER ROLE ORG\n                            Create an account (password on stdin)\n\nRoles: holder, issuer, verifier\nDefault address: http://127.0.0.1:5500\nConfiguration: FATBOT_DATA, FATBOT_ADDR, FATBOT_FABRIC_CONFIG")
		return
	}
	dir := os.Getenv("FATBOT_DATA")
	if dir == "" {
		dir = ".fatbot"
	}
	s, err := openStore(dir, ".")
	if err != nil {
		log.Fatal(err)
	}
	defer s.lock.Close()
	if len(os.Args) > 1 {
		if len(os.Args) != 5 || os.Args[1] != "user-add" {
			log.Fatal("usage: go run . user-add USER ROLE ORG (password on stdin)")
		}
		password, e := bufio.NewReader(os.Stdin).ReadString('\n')
		if e != nil && len(password) == 0 {
			log.Fatal("password required on stdin")
		}
		if e = addUser(s, os.Args[2], os.Args[3], os.Args[4], strings.TrimRight(password, "\r\n")); e != nil {
			log.Fatal(e)
		}
		fmt.Println("Account created.")
		return
	}
	listen := os.Getenv("FATBOT_ADDR")
	if listen == "" {
		listen = "127.0.0.1:5500"
	}
	app := newApp(s)
	if path := os.Getenv("FATBOT_FABRIC_CONFIG"); path != "" {
		ledger, e := openFabric(s, path)
		if e != nil {
			log.Fatal(e)
		}
		app.ledger = ledger
	}
	r := app.router()
	cert, key := os.Getenv("FATBOT_TLS_CERT"), os.Getenv("FATBOT_TLS_KEY")
	if cert != "" || key != "" {
		err = r.RunTLS(listen, cert, key)
	} else {
		err = r.Run(listen)
	}
	if err != nil {
		log.Fatal(err)
	}
}
