package forgejo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florian/vinculum/apps/orchestrator/internal/config"
)

func TestEnsurePullRequestWithTokenUsesDroneToken(t *testing.T) {
	var authHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/vinculum/demo/pulls":
			_, _ = fmt.Fprint(w, `[]`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/vinculum/demo/pulls":
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"number":1,"html_url":"http://example/pr/1","state":"open","head":{"ref":"req/demo"},"base":{"ref":"main"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(config.Config{ForgejoBaseURL: server.URL, ForgejoAdminUser: "admin", ForgejoAdminPass: "admin"})
	if _, err := client.EnsurePullRequestWithToken(context.Background(), "drone-token", "vinculum", "demo", "req/demo", "main", "Demo PR", "body"); err != nil {
		t.Fatalf("ensure pull request with token: %v", err)
	}
	if len(authHeaders) != 2 {
		t.Fatalf("expected 2 authenticated requests, got %d", len(authHeaders))
	}
	for _, header := range authHeaders {
		if header != "token drone-token" {
			t.Fatalf("expected token auth header, got %q", header)
		}
	}
}
