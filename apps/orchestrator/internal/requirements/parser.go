package requirements

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Document struct {
	Title     string   `yaml:"title"`
	Slug      string   `yaml:"slug"`
	Status    string   `yaml:"status"`
	DependsOn []string `yaml:"depends_on"`
	Branch    string   `yaml:"branch"`
	Owner     string   `yaml:"owner,omitempty"`
	Body      string   `yaml:"-"`
}

func ParseDocument(path string, content []byte) (Document, error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Document{}, fmt.Errorf("requirement %s is missing YAML frontmatter", path)
	}
	rest := strings.TrimPrefix(text, "---\n")
	parts := strings.SplitN(rest, "\n---\n", 2)
	if len(parts) != 2 {
		return Document{}, fmt.Errorf("requirement %s is missing YAML frontmatter", path)
	}
	var doc Document
	if err := yaml.Unmarshal([]byte(parts[0]), &doc); err != nil {
		return Document{}, fmt.Errorf("parse requirement frontmatter: %w", err)
	}
	doc.Title = strings.TrimSpace(doc.Title)
	doc.Slug = slugOrDefault(doc.Slug, path, doc.Title)
	doc.Status = normalizeStatus(doc.Status)
	doc.Branch = strings.TrimSpace(doc.Branch)
	if doc.Branch == "" {
		doc.Branch = "req/" + doc.Slug
	}
	for i := range doc.DependsOn {
		doc.DependsOn[i] = strings.TrimSpace(doc.DependsOn[i])
	}
	doc.Body = strings.TrimSpace(parts[1])
	if doc.Title == "" {
		return Document{}, fmt.Errorf("requirement %s is missing title", path)
	}
	if doc.Status == "" {
		return Document{}, fmt.Errorf("requirement %s is missing status", path)
	}
	return doc, nil
}

func RenderDocument(doc Document) (string, error) {
	frontmatter := map[string]any{
		"title":  strings.TrimSpace(doc.Title),
		"slug":   slugOrDefault(doc.Slug, "", doc.Title),
		"status": normalizeStatus(doc.Status),
		"branch": strings.TrimSpace(doc.Branch),
	}
	if frontmatter["branch"] == "" {
		frontmatter["branch"] = "req/" + frontmatter["slug"].(string)
	}
	if len(doc.DependsOn) > 0 {
		frontmatter["depends_on"] = doc.DependsOn
	}
	if strings.TrimSpace(doc.Owner) != "" {
		frontmatter["owner"] = strings.TrimSpace(doc.Owner)
	}
	data, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", err
	}
	body := strings.TrimSpace(doc.Body)
	if body == "" {
		body = "# Context\n\nDescribe the requirement here.\n\n# Acceptance Criteria\n\n- Add acceptance criteria"
	}
	return "---\n" + string(data) + "---\n\n" + body + "\n", nil
}

func Checksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func NormalizeDependencyPath(requirementsPath, value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(trimmed, ".md") && strings.Contains(trimmed, "/") {
		return filepath.Clean(trimmed)
	}
	if strings.HasSuffix(trimmed, ".md") {
		return filepath.ToSlash(filepath.Join(requirementsPath, trimmed))
	}
	return filepath.ToSlash(filepath.Join(requirementsPath, trimmed+".md"))
}

func slugOrDefault(value, path, title string) string {
	base := strings.TrimSpace(value)
	if base == "" {
		base = strings.TrimSpace(title)
	}
	if base == "" && path != "" {
		base = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	base = strings.ToLower(base)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	base = re.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		return "requirement"
	}
	return base
}

func normalizeStatus(value string) string {
	status := strings.ToUpper(strings.TrimSpace(value))
	if status == "" {
		return ""
	}
	return status
}
