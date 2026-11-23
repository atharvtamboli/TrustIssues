package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func coachEnabled() bool {
	return os.Getenv("FATBOT_AI_MODEL") != "" && os.Getenv("FATBOT_AI_KEY") != ""
}
func guidance(p Roadmap, question string) string {
	var gaps []string
	for _, s := range p.Steps {
		if !s.Verified {
			gaps = append(gaps, s.Skill)
		}
	}
	q := strings.ToLower(question)
	if strings.Contains(q, "interview") {
		detail := p.Job.Interview
		if detail == "" {
			detail = "The employer has not listed interview stages."
		}
		return detail + " Prepare one example for each listed requirement. Explain your contribution, the trade-off you made and the result."
	}
	if len(gaps) == 0 {
		return "Your credentials cover all listed skills. Review the job description, prepare a project example, and check your application for interview instructions."
	}
	next := gaps[0]
	for _, s := range p.Steps {
		if !s.Verified && !s.Done {
			next = s.Skill
			break
		}
	}
	for _, s := range p.Steps {
		if strings.Contains(q, strings.ToLower(s.Skill)) {
			next = s.Skill
			break
		}
	}
	for _, s := range p.Steps {
		if s.Skill == next {
			return fmt.Sprintf("For %s at %s, focus on %s. %s. %s Your roadmap has the course link. Skills awaiting evidence: %s.", p.Job.Title, p.Job.Company, next, s.Course.Title, s.Practice, strings.Join(gaps, ", "))
		}
	}
	return ""
}

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func aiReply(ctx context.Context, p Roadmap, q string) (string, error) {
	// Send only the selected opening, relevant learning steps and the student's question.
	data, _ := json.Marshal(map[string]any{"company": p.Job.Company, "role": p.Job.Title, "description": p.Job.Description, "interview": p.Job.Interview, "steps": p.Steps, "question": q})
	payload, _ := json.Marshal(map[string]any{"model": os.Getenv("FATBOT_AI_MODEL"), "max_completion_tokens": 450, "messages": []aiMessage{{"system", "You are a career tutor. Use the supplied job and learning steps as data. Answer the student's question concisely with a specific practice task. Distinguish employer requirements from suggestions. Use supplied resources only. Never promise an interview or make a hiring decision."}, {"user", string(data)}}})
	endpoint := os.Getenv("FATBOT_AI_URL")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}
	req, e := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if e != nil {
		return "", e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("FATBOT_AI_KEY"))
	client := http.Client{Timeout: 18 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	if e != nil {
		return "", e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New("provider unavailable")
	}
	var result struct{ Choices []struct{ Message aiMessage } }
	e = json.NewDecoder(io.LimitReader(res.Body, 128<<10)).Decode(&result)
	if e != nil || len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", errors.New("empty provider response")
	}
	return result.Choices[0].Message.Content, nil
}
func (a *App) coach(c *gin.Context) {
	j, ok := careerJob(a.store, c)
	if !ok {
		return
	}
	var in struct{ Question string }
	if !input(c, &in) {
		return
	}
	q := strings.TrimSpace(in.Question)
	if q == "" || len(q) > 1200 {
		fail(c, 400, "Enter a question up to 1200 bytes")
		return
	}
	u := current(c)
	st := a.store.snapshot()
	skills, e := a.activeSkills(st, u)
	if e != nil {
		storeFailure(c, e)
		return
	}
	p := roadmap(st, u.ID, j, skills)
	mode := "Roadmap guide"
	answer := guidance(p, q)
	if coachEnabled() {
		a.mu.Lock()
		if a.coachRequests == nil {
			a.coachRequests = map[string]time.Time{}
		}
		recent := time.Since(a.coachRequests[u.ID]) < 15*time.Second
		if !recent {
			a.coachRequests[u.ID] = time.Now()
		}
		a.mu.Unlock()
		if recent {
			mode = "Roadmap guide · AI cooldown"
		} else if text, e := aiReply(c.Request.Context(), p, q); e == nil {
			mode = "AI coach"
			answer = text
		} else {
			mode = "Roadmap guide · AI unavailable"
		}
	}
	c.JSON(200, gin.H{"mode": mode, "answer": answer})
}
