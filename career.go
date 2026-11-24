package main

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type CareerProfile struct {
	Target   string
	Progress map[string]map[string]bool
}
type Application struct{ Student, Status, Applied, Note string }
type Course struct{ Title, URL string }
type LearningStep struct {
	Skill          string
	Verified, Done bool
	Course         Course
	Practice       string
}
type Roadmap struct {
	Job         Job
	Steps       []LearningStep
	Application *Application
	Matched     int
}
type CredentialReference struct{ ID, Issuer string }

type Candidate struct {
	Application
	Verified, Missing []string
	Evidence          []CredentialReference
}

func uniqueRequirements(raw []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range raw {
		s = strings.TrimSpace(s)
		k := strings.ToLower(s)
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

var courses = map[string]Course{"go": {"A Tour of Go", "https://go.dev/tour/"}, "golang": {"A Tour of Go", "https://go.dev/tour/"}, "python": {"CS50’s Introduction to Programming with Python", "https://cs50.harvard.edu/python/"}, "sql": {"SQLBolt interactive lessons", "https://sqlbolt.com/"}, "javascript": {"MDN · Learn web development", "https://developer.mozilla.org/en-US/docs/Learn_web_development"}, "css": {"MDN · Learn web development", "https://developer.mozilla.org/en-US/docs/Learn_web_development"}, "html": {"MDN · Learn web development", "https://developer.mozilla.org/en-US/docs/Learn_web_development"}}

func roadmap(st *State, u string, j Job, skills []string) Roadmap {
	plan := Roadmap{Job: j, Steps: []LearningStep{}}
	verified := map[string]bool{}
	for _, s := range skills {
		verified[s] = true
	}
	for _, req := range uniqueRequirements(j.Requirements) {
		k := strings.ToLower(req)
		course, ok := courses[k]
		if !ok {
			course = Course{"Find a course on " + req, "https://www.coursera.org/search?query=" + url.QueryEscape(req)}
		}
		step := LearningStep{req, verified[k], st.Careers[u].Progress[j.ID][k], course, "Build a small example using " + req + " for the " + j.Title + " role. Prepare to explain one decision and how you checked the result."}
		if step.Verified {
			plan.Matched++
		}
		plan.Steps = append(plan.Steps, step)
	}
	if app, ok := st.Applications[j.ID][u]; ok {
		plan.Application = &app
	}
	return plan
}
func findJob(st *State, id string) (Job, bool) {
	for _, j := range st.Jobs {
		if j.ID == id {
			return j, true
		}
	}
	return Job{}, false
}
func careerJob(s *Store, c *gin.Context) (Job, bool) {
	j, ok := findJob(s.snapshot(), c.Param("id"))
	if !ok {
		fail(c, 404, "Opening unavailable")
	}
	return j, ok
}
func input(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	if c.ShouldBindJSON(v) != nil {
		fail(c, 400, "Check the submitted fields")
		return false
	}
	return true
}
func careerSaved(c *gin.Context, e error) {
	if errors.Is(e, errMissing) {
		fail(c, 404, "Application unavailable")
		return
	}
	if e != nil {
		storeFailure(c, e)
		return
	}
	c.Status(204)
}
func (a *App) careerRoutes(r *gin.Engine, holder, employer gin.HandlerFunc) {
	both := a.require("holder", "verifier")
	r.GET("/career", both, func(c *gin.Context) {
		if current(c).Role == "holder" {
			c.Redirect(303, "/holder#jobs")
			return
		}
		c.Redirect(303, "/verifier#hiring")
	})
	r.GET("/workspace-nav.js", asset("workspace-nav.js"))
	r.GET("/student.css", asset("student.css"))
	r.GET("/student.js", asset("student.js"))
	r.GET("/skill-lab.js", asset("skill-lab.js"))

	r.GET("/hiring.js", asset("hiring.js"))
	r.GET("/career/data", both, func(c *gin.Context) {
		st := a.store.snapshot()
		u := current(c)
		jobs := []Job{}
		statuses := map[string]string{}
		for _, j := range st.Jobs {
			if u.Role == "holder" || j.Owner == u.ID {
				jobs = append(jobs, j)
				if app, ok := st.Applications[j.ID][u.ID]; ok {
					statuses[j.ID] = app.Status
				}
			}
		}
		c.JSON(200, gin.H{"user": u.ID, "role": u.Role, "org": u.Org, "jobs": jobs, "target": st.Careers[u.ID].Target, "ai": coachEnabled(), "statuses": statuses})
	})
	r.GET("/career/jobs/:id/plan", holder, func(c *gin.Context) {
		j, ok := careerJob(a.store, c)
		if !ok {
			return
		}
		u := current(c)
		st := a.store.snapshot()
		skills, e := a.activeSkills(st, u)
		if e != nil {
			storeFailure(c, e)
			return
		}
		c.JSON(200, roadmap(st, u.ID, j, skills))
	})
	r.POST("/career/jobs/:id/plan", holder, func(c *gin.Context) {
		j, ok := careerJob(a.store, c)
		if !ok {
			return
		}
		var in struct {
			Skill string
			Done  bool
		}
		if !input(c, &in) {
			return
		}
		key := strings.ToLower(in.Skill)
		if key != "" {
			found := false
			for _, s := range j.Requirements {
				if strings.EqualFold(s, key) {
					found = true
				}
			}
			if !found {
				fail(c, 400, "Unknown requirement")
				return
			}
		}
		u := current(c).ID
		careerSaved(c, a.store.update(func(st *State) error {
			if st.Careers == nil {
				st.Careers = map[string]CareerProfile{}
			}
			p := st.Careers[u]
			p.Target = j.ID
			if key != "" {
				if p.Progress == nil {
					p.Progress = map[string]map[string]bool{}
				}
				if p.Progress[j.ID] == nil {
					p.Progress[j.ID] = map[string]bool{}
				}
				p.Progress[j.ID][key] = in.Done
			}
			st.Careers[u] = p
			return nil
		}))
	})
	r.POST("/career/jobs/:id/apply", holder, func(c *gin.Context) {
		j, ok := careerJob(a.store, c)
		if !ok {
			return
		}
		if j.Owner == "" {
			fail(c, 409, "Archived opening")
			return
		}
		u := current(c).ID
		careerSaved(c, a.store.update(func(st *State) error {
			if st.Applications == nil {
				st.Applications = map[string]map[string]Application{}
			}
			if st.Applications[j.ID] == nil {
				st.Applications[j.ID] = map[string]Application{}
			}
			if _, ok := st.Applications[j.ID][u]; !ok {
				st.Applications[j.ID][u] = Application{Student: u, Status: "Applied", Applied: utcNow()}
			}
			return nil
		}))
	})
	r.GET("/career/jobs/:id/applicants", employer, func(c *gin.Context) {
		j, ok := careerJob(a.store, c)
		if !ok {
			return
		}
		if j.Owner != current(c).ID {
			fail(c, 404, "Opening unavailable")
			return
		}
		st := a.store.snapshot()
		list := []Candidate{}
		for _, app := range st.Applications[j.ID] {
			blocks, e := a.skillCredentials(st, st.Accounts[app.Student])
			if e != nil {
				storeFailure(c, e)
				return
			}
			candidate := Candidate{Application: app, Verified: []string{}, Missing: []string{}}
			skills := getStudentSkills(blocks)
			required := map[string]bool{}
			for _, req := range j.Requirements {
				required[strings.ToLower(req)] = true
			}
			for _, block := range blocks {
				for _, skill := range getStudentSkills([]Block{block}) {
					if required[skill] {
						candidate.Evidence = append(candidate.Evidence, CredentialReference{block.CredentialID, block.IssuerOrg})
						break
					}
				}
			}
			for _, s := range roadmap(st, app.Student, j, skills).Steps {
				if s.Verified {
					candidate.Verified = append(candidate.Verified, s.Skill)
				} else {
					candidate.Missing = append(candidate.Missing, s.Skill)
				}
			}
			list = append(list, candidate)
		}
		sort.Slice(list, func(i, k int) bool {
			if len(list[i].Verified) == len(list[k].Verified) {
				return list[i].Applied < list[k].Applied
			}
			return len(list[i].Verified) > len(list[k].Verified)
		})
		c.JSON(200, list)
	})
	r.POST("/career/jobs/:id/invite", employer, func(c *gin.Context) {
		j, ok := careerJob(a.store, c)
		if !ok {
			return
		}
		if j.Owner != current(c).ID {
			fail(c, 404, "Opening unavailable")
			return
		}
		var in struct{ Student, Note string }
		if !input(c, &in) {
			return
		}
		in.Note = strings.TrimSpace(in.Note)
		if in.Note == "" || len(in.Note) > 1000 {
			fail(c, 400, "Enter interview instructions (up to 1000 bytes)")
			return
		}
		careerSaved(c, a.store.update(func(st *State) error {
			v, ok := st.Applications[j.ID][in.Student]
			if !ok {
				return errMissing
			}
			v.Status = "Interview invited"
			v.Note = in.Note
			st.Applications[j.ID][in.Student] = v
			return nil
		}))
	})
	r.POST("/career/jobs/:id/coach", holder, a.coach)
}
