package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCareerWorkflow(t *testing.T) {
	h := setup(t)
	t.Setenv("FATBOT_AI_KEY", "")
	student, company, dean := h.login("alice"), h.login("employer"), h.login("dean")
	post := func(path, body string, user *http.Cookie, want int) string {
		t.Helper()
		w := h.request("POST", path, "application/json", strings.NewReader(body), user)
		expect(t, w, want)
		return w.Body.String()
	}
	var created struct{ Job Job }
	raw := post("/jobs", `{"company":"Acme","title":"Developer","requirements":["Go","SQL","go"],"interview":"Pair programming"}`, company, 200)
	if err := json.Unmarshal([]byte(raw), &created); err != nil || len(created.Job.Requirements) != 2 || created.Job.ID == "" {
		t.Fatal(raw)
	}
	path := "/career/jobs/" + created.Job.ID
	id := h.upload(student, "Go project", "Go", "uni-a")
	expect(t, h.approve(dean, id), 200)
	post(path+"/plan", `{"Skill":"SQL","Done":true}`, student, 204)
	post(path+"/apply", "{}", student, 204)
	post(path+"/apply", "{}", student, 204)
	applicants := func() []Candidate {
		t.Helper()
		w := h.request("GET", path+"/applicants", "", nil, company)
		expect(t, w, 200)
		var list []Candidate
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list) != 1 {
			t.Fatal(w.Body.String())
		}
		return list
	}
	c := applicants()[0]
	if len(c.Evidence) != 1 || c.Evidence[0].ID != id || len(c.Verified) != 1 || c.Verified[0] != "Go" || len(c.Missing) != 1 || c.Missing[0] != "SQL" {
		t.Fatal(c)
	}
	expect(t, h.request("GET", path+"/applicants", "", nil, student), 403)
	if err := addUser(h.s, "second", "verifier", "Elsewhere", "synthetic-password"); err != nil {
		t.Fatal(err)
	}
	expect(t, h.request("GET", path+"/applicants", "", nil, h.login("second")), 404)
	post(path+"/invite", `{"Student":"alice","Note":"Join Tuesday"}`, company, 204)
	plan := func(user *http.Cookie) Roadmap {
		t.Helper()
		w := h.request("GET", path+"/plan", "", nil, user)
		expect(t, w, 200)
		var p Roadmap
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := plan(student)
	if p.Application == nil || p.Application.Note != "Join Tuesday" || p.Matched != 1 || !p.Steps[1].Done {
		t.Fatal(p)
	}
	if p := plan(h.login("bob")); p.Application != nil || p.Steps[1].Done || p.Matched != 0 {
		t.Fatal(p)
	}
	q := `{"Question":"How do I prepare for this interview?"}`
	if guide := post(path+"/coach", q, student, 200); !strings.Contains(guide, "Pair programming") {
		t.Fatal(guide)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Error("missing provider authentication")
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"Practice a SQL join."}}]}`))
	}))
	defer upstream.Close()
	t.Setenv("FATBOT_AI_MODEL", "test")
	t.Setenv("FATBOT_AI_KEY", "test")
	t.Setenv("FATBOT_AI_URL", upstream.URL)
	if reply := post(path+"/coach", q, student, 200); !strings.Contains(reply, "Practice a SQL join.") {
		t.Fatal(reply)
	}
	post("/credentials/"+id+"/revoke", "", dean, 200)
	if len(applicants()[0].Verified) != 0 {
		t.Fatal("revoked credential counted")
	}
}

func TestPlannerDashboardEvidence(t *testing.T) {
	h := setup(t)
	alice, employer, dean := h.login("alice"), h.login("employer"), h.login("dean")
	w := h.request("POST", "/jobs", "application/json", strings.NewReader(`{"title":"Planner fixture","requirements":["Go","SQL"]}`), employer)
	expect(t, w, 200)
	check := func(user *http.Cookie, want int) {
		t.Helper()
		w := h.request("GET", "/student/dashboard", "", nil, user)
		expect(t, w, 200)
		var dashboard StudentDashboardData
		if err := json.Unmarshal(w.Body.Bytes(), &dashboard); err != nil {
			t.Fatal(err)
		}
		for _, match := range dashboard.JobMatches {
			if match.Job.Title == "Planner fixture" {
				if len(match.MissingSkills) != want || match.Job.Owner != "employer" {
					t.Fatal(match)
				}
				return
			}
		}
		t.Fatal("missing opening")
	}
	id := h.upload(alice, "planner evidence", "Go", "uni-a")
	check(alice, 2)
	expect(t, h.approve(dean, id), 200)
	check(alice, 1)
	check(h.login("bob"), 2)
	expect(t, h.request("POST", "/credentials/"+id+"/revoke", "application/json", nil, dean), 200)
	check(alice, 2)
	expect(t, h.request("GET", "/student/dashboard", "", nil, employer), 403)
	expect(t, h.request("GET", "/skill-lab.js", "", nil, nil), 200)
}
