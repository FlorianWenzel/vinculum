package requirements

import "testing"

func TestParseDocument(t *testing.T) {
	content := []byte("---\ntitle: User authentication\nslug: user-auth\nstatus: TODO\ndepends_on:\n  - foundation-api.md\nbranch: req/user-auth\n---\n\n# Context\n\nShip login.")
	doc, err := ParseDocument("requirements/user-auth.md", content)
	if err != nil {
		t.Fatalf("parse document: %v", err)
	}
	if doc.Title != "User authentication" {
		t.Fatalf("unexpected title %q", doc.Title)
	}
	if doc.Branch != "req/user-auth" {
		t.Fatalf("unexpected branch %q", doc.Branch)
	}
	if len(doc.DependsOn) != 1 || doc.DependsOn[0] != "foundation-api.md" {
		t.Fatalf("unexpected dependencies %#v", doc.DependsOn)
	}
}

func TestRenderDocumentProvidesDefaults(t *testing.T) {
	content, err := RenderDocument(Document{Title: "API foundation", Status: "todo"})
	if err != nil {
		t.Fatalf("render document: %v", err)
	}
	doc, err := ParseDocument("requirements/api-foundation.md", []byte(content))
	if err != nil {
		t.Fatalf("parse rendered document: %v", err)
	}
	if doc.Slug != "api-foundation" {
		t.Fatalf("unexpected slug %q", doc.Slug)
	}
	if doc.Branch != "req/api-foundation" {
		t.Fatalf("unexpected branch %q", doc.Branch)
	}
	if doc.Status != "TODO" {
		t.Fatalf("unexpected status %q", doc.Status)
	}
}
