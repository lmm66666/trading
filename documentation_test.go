package main

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestMarkdownDestinations(t *testing.T) {
	source := []byte("" +
		"[ordinary](docs/ordinary.md)\n" +
		"[titled](docs/titled.md \"title\")\n" +
		"[angled](<docs/with space.md#section>)\n" +
		"[reference][design]\n\n" +
		"[design]: docs/reference.md\n\n" +
		"`[inline](docs/ignored-inline.md)`\n\n" +
		"```text\n[fenced](docs/ignored-fenced.md)\n```\n")
	want := []string{
		"docs/ordinary.md",
		"docs/titled.md",
		"docs/with space.md#section",
		"docs/reference.md",
	}
	if got := markdownDestinations(source); !reflect.DeepEqual(got, want) {
		t.Fatalf("markdown destinations = %#v, want %#v", got, want)
	}
}

func TestMarkdownAnchors(t *testing.T) {
	source := []byte("# 中文标题 / API\n\n## Repeat\n\n## Repeat\n")
	want := map[string]struct{}{
		"中文标题--api": {},
		"repeat":    {},
		"repeat-1":  {},
	}
	if got := markdownAnchors(source); !reflect.DeepEqual(got, want) {
		t.Fatalf("markdown anchors = %#v, want %#v", got, want)
	}
}

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
	for _, forbidden := range []string{"README.md", "docs/decisions"} {
		if _, err := os.Stat(forbidden); !os.IsNotExist(err) {
			t.Errorf("forbidden documentation path %s must not exist", forbidden)
		}
	}
	validateRequirementStatuses(t, "docs/requirements/active", false)
	validateRequirementStatuses(t, "docs/requirements/archived", true)
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
		for _, target := range markdownDestinations(body) {
			if externalDocumentationTarget(target) {
				continue
			}
			pathTarget, fragment := splitDocumentationTarget(target)
			if pathTarget == "" {
				pathTarget = name
			}
			if strings.HasPrefix(pathTarget, "/") {
				pathTarget = strings.TrimPrefix(pathTarget, "/")
			} else if pathTarget != name {
				pathTarget = filepath.Join(filepath.Dir(name), filepath.FromSlash(pathTarget))
			}
			decoded, err := url.PathUnescape(pathTarget)
			if err != nil {
				t.Errorf("%s contains invalid escaped link %q: %v", clean, pathTarget, err)
				continue
			}
			resolved := filepath.Clean(filepath.FromSlash(decoded))
			if _, err := os.Stat(resolved); err != nil {
				t.Errorf("%s links to missing target %q: %v", clean, pathTarget, err)
				continue
			}
			if fragment != "" && filepath.Ext(resolved) == ".md" {
				validateMarkdownAnchor(t, clean, resolved, fragment)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk documentation: %v", err)
	}
}

func markdownDestinations(source []byte) []string {
	document := goldmark.DefaultParser().Parse(text.NewReader(source))
	destinations := make([]string, 0)
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch link := node.(type) {
		case *ast.Link:
			destinations = append(destinations, string(link.Destination))
		case *ast.Image:
			destinations = append(destinations, string(link.Destination))
		}
		return ast.WalkContinue, nil
	})
	return destinations
}

func splitDocumentationTarget(target string) (string, string) {
	parsed, err := url.Parse(target)
	if err != nil {
		return target, ""
	}
	return parsed.Path, parsed.Fragment
}

func validateMarkdownAnchor(t *testing.T, sourceName, targetName, fragment string) {
	t.Helper()
	decoded, err := url.PathUnescape(fragment)
	if err != nil {
		t.Errorf("%s contains invalid escaped anchor %q: %v", sourceName, fragment, err)
		return
	}
	body, err := os.ReadFile(targetName)
	if err != nil {
		t.Errorf("read anchor target %s: %v", targetName, err)
		return
	}
	if _, ok := markdownAnchors(body)[decoded]; !ok {
		t.Errorf("%s links to missing anchor %q in %s", sourceName, fragment, targetName)
	}
}

func markdownAnchors(source []byte) map[string]struct{} {
	document := goldmark.DefaultParser().Parse(text.NewReader(source))
	anchors := make(map[string]struct{})
	duplicates := make(map[string]int)
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		heading, ok := node.(*ast.Heading)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		base := markdownAnchor(string(heading.Text(source)))
		anchor := base
		if count := duplicates[base]; count > 0 {
			anchor = base + "-" + strconv.Itoa(count)
		}
		duplicates[base]++
		anchors[anchor] = struct{}{}
		return ast.WalkContinue, nil
	})
	return anchors
}

func markdownAnchor(heading string) string {
	var result strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(heading)) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			result.WriteRune(r)
		case unicode.IsSpace(r):
			result.WriteByte('-')
		}
	}
	return result.String()
}

func validateRequirementStatuses(t *testing.T, directory string, archived bool) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Errorf("read requirement directory %s: %v", directory, err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "REQ-") || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Errorf("read requirement %s: %v", entry.Name(), err)
			continue
		}
		status := requirementStatus(string(body))
		valid := map[string]bool{
			"待评审": true,
			"已批准": true,
			"开发中": true,
			"待合并": true,
			"已完成": true,
			"已取消": true,
		}
		if !valid[status] {
			t.Errorf("requirement %s has unknown or missing status %q", entry.Name(), status)
			continue
		}
		completed := status == "已完成" || status == "已取消"
		if archived != completed {
			t.Errorf("requirement %s has status %q inconsistent with directory %s", entry.Name(), status, directory)
		}
	}
}

func requirementStatus(body string) string {
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Split(line, "|")
		if len(fields) >= 4 && strings.TrimSpace(fields[1]) == "状态" {
			return strings.TrimSpace(fields[2])
		}
	}
	return ""
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
	if target == "" || strings.HasPrefix(target, "#") {
		return false
	}
	parsed, err := url.Parse(target)
	return err == nil && parsed.Scheme != ""
}
