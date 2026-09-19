package main

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
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
		"docs/design/README.md",
		"docs/standards/engineering.md",
		"docs/design/api.md",
		"docs/standards/http-api.md",
		"docs/design/data.md",
		"docs/design/internal/market.md",
		"docs/design/internal/indicator.md",
		"docs/design/internal/strategy.md",
		"docs/design/workflows/strategy-scan.md",
		"docs/design/workflows/market-data.md",
		"docs/design/workflows/backtesting.md",
		"docs/design/workflows/chart-query.md",
		"docs/design/strategies/daily-b1.md",
		"docs/design/strategies/weekly-b1.md",
		"docs/design/strategies/bottom-surge-pullback.md",
		"docs/design/internal/backtest.md",
		"docs/design/internal/application.md",
		"docs/design/internal/port.md",
		"docs/design/internal/infrastructure/mysql.md",
		"docs/design/pkg/broker.md",
		"docs/design/web.md",
		"docs/design/cmd/migrate-strategy-kernel.md",
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
	for _, forbidden := range []string{"README.md", "docs/decisions", "docs/templates", "docs/requirements"} {
		if _, err := os.Stat(forbidden); !os.IsNotExist(err) {
			t.Errorf("forbidden documentation path %s must not exist", forbidden)
		}
	}
	validateChangeLayout(t)
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
			sameDocument := pathTarget == ""
			if sameDocument {
				pathTarget = name
			} else if strings.HasPrefix(pathTarget, "/") {
				pathTarget = strings.TrimPrefix(pathTarget, "/")
			} else {
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

func validateChangeLayout(t *testing.T) {
	t.Helper()
	seen := map[string]string{}
	for _, root := range []string{"docs/changes/active", "docs/changes/archive"} {
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			directory := filepath.Join(root, entry.Name())
			if root == "docs/changes/archive" && entry.Name() == "legacy" {
				validateLegacyChanges(t, directory)
				continue
			}
			if !entry.IsDir() {
				t.Errorf("change must be a directory: %s", directory)
				continue
			}
			requirements := readDocumentMetadata(t, filepath.Join(directory, "requirements.md"))
			design := readDocumentMetadata(t, filepath.Join(directory, "design.md"))
			verification := readDocumentMetadata(t, filepath.Join(directory, "verification.md"))
			id := requirements.ID
			if id == "" {
				t.Errorf("missing change id: %s", directory)
			}
			if previous, ok := seen[id]; ok {
				t.Errorf("duplicate change id %s: %s and %s", id, previous, directory)
			}
			seen[id] = directory
			valid := map[string]bool{"draft": true, "reviewed": true, "approved": true, "implementing": true, "verifying": true, "implemented": true, "rejected": true, "superseded": true}
			if !valid[requirements.Status] {
				t.Errorf("invalid change status in %s: %s", directory, requirements.Status)
			}
			if (root == "docs/changes/archive") != (requirements.Status == "implemented") {
				t.Errorf("change status/location mismatch: %s", directory)
			}
			if design.Status != "" || verification.Status != "" {
				t.Errorf("only requirements owns lifecycle: %s", directory)
			}
			results := map[string]bool{"pending": true, "passed": true, "failed": true, "blocked": true}
			if !results[verification.Result] {
				t.Errorf("invalid verification result: %s", directory)
			}
			if requirements.Status == "implemented" && verification.Result != "passed" {
				t.Errorf("implemented change without passing evidence: %s", directory)
			}
			for _, doc := range []documentMetadata{requirements, design} {
				if !map[string]bool{"draft": true, "in-review": true, "approved": true, "changes-requested": true}[doc.ApprovalStatus] {
					t.Errorf("invalid approval status: %s", directory)
				}
				if doc.ApprovalStatus == "approved" && (doc.ApprovedBy == "" || doc.ApprovedAt == "" || doc.ApprovedRevision == "" || len(doc.ApprovedScope) == 0) {
					t.Errorf("approved document lacks approval evidence: %s", directory)
				}
				if map[string]bool{"approved": true, "implementing": true, "verifying": true, "implemented": true}[requirements.Status] && doc.ApprovalStatus != "approved" {
					t.Errorf("implementation without approved documents: %s", directory)
				}
			}
		}
	}
}

func validateLegacyChanges(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !regexp.MustCompile(`^REQ-\d{4}-\d{3}-[a-z0-9-]+\.md$`).MatchString(entry.Name()) {
			t.Errorf("invalid historical record: %s", entry.Name())
			continue
		}
		body, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		status := requirementStatus(string(body))
		if status != "已完成" && status != "已取消" {
			t.Errorf("nonterminal historical record: %s", entry.Name())
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
	case ".git", ".idea", ".worktrees", ".superpowers", "docs/superpowers", "web/node_modules", "web/dist", "web/coverage":
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

// documentMetadata contains only fields checked by the repository gates.
type documentMetadata struct {
	Kind             string   `yaml:"kind"`
	BaselineRevision string   `yaml:"baseline_revision"`
	ID               string   `yaml:"id"`
	Status           string   `yaml:"status"`
	Authority        string   `yaml:"authority"`
	Owns             []string `yaml:"owns"`
	Result           string   `yaml:"result"`
	ApprovalStatus   string   `yaml:"approval_status"`
	ApprovedBy       string   `yaml:"approved_by"`
	ApprovedAt       string   `yaml:"approved_at"`
	ApprovedRevision string   `yaml:"approved_revision"`
	ApprovedScope    []string `yaml:"approved_scope"`
}

func readDocumentMetadata(t *testing.T, name string) documentMetadata {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Errorf("read document %s: %v", name, err)
		return documentMetadata{}
	}
	parts := strings.SplitN(string(body), "---", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
		t.Errorf("missing frontmatter: %s", name)
		return documentMetadata{}
	}
	var metadata documentMetadata
	if err := yaml.Unmarshal([]byte(parts[1]), &metadata); err != nil {
		t.Errorf("invalid frontmatter %s: %v", name, err)
	}
	return metadata
}

func TestDocumentationOwnership(t *testing.T) {
	owners := map[string][]string{}
	for _, root := range []string{"docs/design", "docs/architecture", "docs/standards"} {
		err := filepath.WalkDir(root, func(name string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(name) != ".md" {
				return nil
			}
			metadata := readDocumentMetadata(t, name)
			if !validDesignMetadata(filepath.ToSlash(name), metadata) {
				t.Errorf("invalid design authority or explanation metadata: %s", name)
			}
			for _, claim := range metadata.Owns {
				clean := filepath.ToSlash(filepath.Clean(claim))
				if filepath.IsAbs(claim) || clean == ".." || strings.HasPrefix(clean, "../") || strings.ContainsAny(claim, "*?[]") || strings.TrimSuffix(claim, "/") != clean {
					t.Errorf("invalid owns %q in %s", claim, name)
					continue
				}
				info, err := os.Stat(clean)
				if err != nil {
					t.Errorf("missing owns %q in %s", claim, name)
					continue
				}
				if info.IsDir() {
					if !strings.HasSuffix(claim, "/") {
						t.Errorf("directory owns needs trailing slash: %s", claim)
						continue
					}
					entries, err := os.ReadDir(clean)
					if err != nil {
						return err
					}
					for _, item := range entries {
						if !item.IsDir() {
							file := filepath.ToSlash(filepath.Join(clean, item.Name()))
							owners[file] = append(owners[file], name)
						}
					}
				} else {
					if strings.HasSuffix(claim, "/") {
						t.Errorf("file owns cannot end in slash: %s", claim)
					}
					owners[clean] = append(owners[clean], name)
				}
			}
			return nil
		})
		if err != nil {
			t.Error(err)
		}
	}
	for name, documents := range owners {
		if len(documents) > 1 {
			t.Errorf("overlapping owners for %s: %v", name, documents)
		}
	}
	err := filepath.WalkDir(".", func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		clean := filepath.ToSlash(name)
		if entry.IsDir() && ignoredDocumentationDirectory(clean) {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		switch filepath.Ext(clean) {
		case ".go", ".ts", ".tsx", ".js", ".jsx", ".sh", ".css", ".sql":
			if len(owners[clean]) != 1 {
				t.Errorf("source %s must have exactly one design owner, got %v", clean, owners[clean])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func validDesignMetadata(name string, metadata documentMetadata) bool {
	explanationPath := strings.HasPrefix(name, "docs/design/workflows/") || strings.HasPrefix(name, "docs/design/strategies/")
	if explanationPath {
		return metadata.Kind == "explanation" && metadata.Status == "baseline-review" &&
			metadata.Authority == "code-derived" && len(metadata.Owns) == 0 &&
			regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(metadata.BaselineRevision)
	}
	return metadata.Kind == "" && metadata.Status == "approved" && metadata.Authority == "normative"
}

func TestDocumentationAuthorityBoundaries(t *testing.T) {
	baseline := "0aadb823d45701cd3c1d702f510bf97420bb4fc1"
	explanation := documentMetadata{Kind: "explanation", Status: "baseline-review", Authority: "code-derived", BaselineRevision: baseline}
	normative := documentMetadata{Status: "approved", Authority: "normative"}
	cases := []struct {
		name     string
		path     string
		metadata documentMetadata
		want     bool
	}{
		{"existing module", "docs/design/internal/strategy.md", normative, true},
		{"workflow explanation", "docs/design/workflows/strategy-scan.md", explanation, true},
		{"strategy explanation", "docs/design/strategies/daily-b1.md", explanation, true},
		{"module cannot downgrade", "docs/design/internal/strategy.md", explanation, false},
		{"standard cannot downgrade", "docs/standards/http-api.md", explanation, false},
		{"draft module", "docs/design/internal/strategy.md", documentMetadata{Status: "draft", Authority: "proposed"}, false},
		{"missing source revision", "docs/design/strategies/daily-b1.md", documentMetadata{Kind: "explanation", Status: "baseline-review", Authority: "code-derived"}, false},
		{"explanation cannot own source", "docs/design/strategies/daily-b1.md", documentMetadata{Kind: "explanation", Status: "baseline-review", Authority: "code-derived", BaselineRevision: baseline, Owns: []string{"internal/strategy/"}}, false},
		{"explanation cannot claim approval", "docs/design/strategies/daily-b1.md", documentMetadata{Kind: "explanation", Status: "approved", Authority: "normative", BaselineRevision: baseline}, false},
		{"explanation kind required", "docs/design/strategies/daily-b1.md", normative, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validDesignMetadata(tc.path, tc.metadata); got != tc.want {
				t.Fatalf("validDesignMetadata(%s) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
