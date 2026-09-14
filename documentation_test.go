package main

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var markdownLinkPattern = regexp.MustCompile(`\[[^]]+\]\(([^)[:space:]]+)\)`)

func TestDocumentationContract(t *testing.T) {
	required := []string{
		"AGENTS.md",
		"docs/roadmap.md",
		"docs/operations.md",
		"docs/architecture/system-design.md",
		"docs/architecture/domain-map.md",
		"docs/templates/module-design.md",
		"docs/templates/requirement.md",
		"api/DESIGN.md",
		"api/api.md",
		"business/DESIGN.md",
		"data/DESIGN.md",
		"internal/market/DESIGN.md",
		"internal/indicator/DESIGN.md",
		"internal/strategy/DESIGN.md",
		"internal/backtest/DESIGN.md",
		"internal/application/DESIGN.md",
		"internal/financialscreen/DESIGN.md",
		"internal/port/DESIGN.md",
		"internal/infrastructure/mysql/DESIGN.md",
		"pkg/broker/DESIGN.md",
		"web/DESIGN.md",
		"cmd/migrate-strategy-kernel/DESIGN.md",
	}
	for _, name := range required {
		info, err := os.Stat(name)
		if err != nil {
			t.Errorf("required documentation %s: %v", name, err)
			continue
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			t.Errorf("required documentation %s must be a non-empty file", name)
		}
	}
}

func TestDocumentationLinks(t *testing.T) {
	err := filepath.WalkDir(".", func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		clean := filepath.ToSlash(strings.TrimPrefix(name, "./"))
		if entry.IsDir() && ignoredDocumentationDirectory(clean) {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(name) != ".md" {
			return nil
		}
		body, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		for _, match := range markdownLinkPattern.FindAllSubmatch(body, -1) {
			target := string(match[1])
			if externalDocumentationTarget(target) {
				continue
			}
			if index := strings.IndexAny(target, "#?"); index >= 0 {
				target = target[:index]
			}
			decoded, err := url.PathUnescape(target)
			if err != nil {
				t.Errorf("%s contains invalid escaped link %q: %v", clean, target, err)
				continue
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(name), filepath.FromSlash(decoded)))
			if _, err := os.Stat(resolved); err != nil {
				t.Errorf("%s links to missing target %q: %v", clean, target, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk documentation: %v", err)
	}
}

func ignoredDocumentationDirectory(name string) bool {
	switch name {
	case ".git", ".idea", ".worktrees", ".superpowers", "docs/analysis", "docs/superpowers", "web/node_modules", "web/dist", "web/coverage":
		return true
	default:
		return false
	}
}

func externalDocumentationTarget(target string) bool {
	return target == "" || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "/") ||
		strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
		strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "codex://")
}
