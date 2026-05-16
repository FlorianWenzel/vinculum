package git

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseGitHubRepo(t *testing.T) {
	cases := []struct {
		url   string
		owner string
		repo  string
		ok    bool
	}{
		{"https://github.com/acme/api.git", "acme", "api", true},
		{"https://github.com/acme/api", "acme", "api", true},
		{"git@github.com:acme/api.git", "acme", "api", true},
		{"git@github.com:acme/api", "acme", "api", true},
		{"ssh://git@github.com/acme/api.git", "acme", "api", true},
		{"https://gitlab.com/acme/api.git", "", "", false},
		{"", "", "", false},
		{"not-a-url", "", "", false},
		{"https://github.com/only-one-part", "", "", false},
	}
	for _, c := range cases {
		o, r, err := ParseGitHubRepo(c.url)
		gotOK := err == nil
		if gotOK != c.ok {
			t.Errorf("%q: ok=%v want %v (err=%v)", c.url, gotOK, c.ok, err)
			continue
		}
		if c.ok && (o != c.owner || r != c.repo) {
			t.Errorf("%q: got %q/%q want %q/%q", c.url, o, r, c.owner, c.repo)
		}
	}
}

func TestCreatePR_Success(t *testing.T) {
	var got struct {
		Title, Head, Base, Body string
	}
	var authHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/api/pulls" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method=%q", r.Method)
		}
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"html_url": "https://github.com/acme/api/pull/42",
			"number":   42,
		})
	}))
	defer srv.Close()

	c := NewGitHubClient("ghp_test").WithBaseURL(srv.URL)
	out, err := c.CreatePR(context.Background(), "acme", "api", GitHubPRRequest{
		Title: "Feat: x", Head: "feat/x", Base: "main", Body: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Number != 42 || out.HTMLURL != "https://github.com/acme/api/pull/42" {
		t.Errorf("out=%+v", out)
	}
	if got.Title != "Feat: x" || got.Head != "feat/x" || got.Base != "main" || got.Body != "hello" {
		t.Errorf("payload=%+v", got)
	}
	if authHeader != "Bearer ghp_test" {
		t.Errorf("authHeader=%q", authHeader)
	}
}

func TestCreatePR_GitHubError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Validation Failed",
			"errors":  []map[string]string{{"message": "A pull request already exists"}},
		})
	}))
	defer srv.Close()

	c := NewGitHubClient("ghp_test").WithBaseURL(srv.URL)
	_, err := c.CreatePR(context.Background(), "acme", "api", GitHubPRRequest{
		Title: "x", Head: "h", Base: "b",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "Validation Failed") {
		t.Errorf("err=%v", err)
	}
}

func TestCreatePR_AlreadyExists_ReturnsExistingPR(t *testing.T) {
	var listed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/api/pulls":
			w.WriteHeader(422)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": "Validation Failed",
				"errors": []map[string]string{{
					"resource": "PullRequest",
					"code":     "custom",
					"message":  "A pull request already exists for acme:feat/x.",
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/api/pulls":
			listed = true
			if got := r.URL.Query().Get("head"); got != "acme:feat/x" {
				t.Errorf("head query = %q", got)
			}
			if got := r.URL.Query().Get("base"); got != "main" {
				t.Errorf("base query = %q", got)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"html_url": "https://github.com/acme/api/pull/99",
				"number":   99,
			}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c := NewGitHubClient("ghp_test").WithBaseURL(srv.URL)
	out, err := c.CreatePR(context.Background(), "acme", "api", GitHubPRRequest{
		Title: "Feat: x", Head: "feat/x", Base: "main",
	})
	if err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if !listed {
		t.Error("expected the client to list existing PRs after 422")
	}
	if out.Number != 99 || out.HTMLURL != "https://github.com/acme/api/pull/99" {
		t.Errorf("out=%+v", out)
	}
}

func TestCreatePR_AlreadyExists_NoMatchingPR_StillErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(422)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": "A pull request already exists",
			})
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{}) // empty
		}
	}))
	defer srv.Close()

	c := NewGitHubClient("ghp_test").WithBaseURL(srv.URL)
	_, err := c.CreatePR(context.Background(), "a", "b", GitHubPRRequest{
		Title: "x", Head: "h", Base: "b",
	})
	if err == nil {
		t.Error("expected error when API says exists but list is empty")
	}
}

func TestCreatePR_ValidatesArgs(t *testing.T) {
	c := NewGitHubClient("")
	if _, err := c.CreatePR(context.Background(), "a", "b", GitHubPRRequest{Title: "x", Head: "h", Base: "b"}); err == nil {
		t.Error("missing token should error")
	}
	c = NewGitHubClient("ghp_test")
	if _, err := c.CreatePR(context.Background(), "a", "b", GitHubPRRequest{Title: "", Head: "h", Base: "b"}); err == nil {
		t.Error("missing title should error")
	}
	if _, err := c.CreatePR(context.Background(), "a", "b", GitHubPRRequest{Title: "t", Head: "", Base: "b"}); err == nil {
		t.Error("missing head should error")
	}
	if _, err := c.CreatePR(context.Background(), "a", "b", GitHubPRRequest{Title: "t", Head: "h", Base: ""}); err == nil {
		t.Error("missing base should error")
	}
}
