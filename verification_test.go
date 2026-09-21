package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Command substitutes verify selection and failure propagation, not external services.
func TestVerificationSelection(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		args                     []string
		local, full, mysql, fail bool
		images                   []string
		failTool                 string
	}{
		{name: "quick", local: true},
		{name: "mysql_only", args: []string{"--mysql"}, mysql: true},
		{name: "images_only", args: []string{"--image"}, images: []string{"updater", "workbench", "updater-configured"}},
		{name: "updater_only", args: []string{"--image=updater"}, images: []string{"updater"}},
		{name: "configured_only", args: []string{"--image=updater-configured"}, images: []string{"updater-configured"}},
		{name: "combined", args: []string{"--mysql", "--image=workbench"}, mysql: true, images: []string{"workbench"}},
		{name: "full", args: []string{"--full"}, local: true, full: true, mysql: true, images: []string{"updater", "workbench", "updater-configured"}},
		{name: "help", args: []string{"--help"}},
		{name: "invalid", args: []string{"--my-sql"}, fail: true},
		{name: "invalid_target", args: []string{"--image=unknown"}, fail: true},
		{name: "coverage_failure", args: []string{"--full"}, local: true, full: true, fail: true, failTool: "coverage"},
		{name: "local_failure", local: true, fail: true, failTool: "npm"},
		{name: "mysql_failure", args: []string{"--mysql"}, mysql: true, fail: true, failTool: "mysql"},
		{name: "image_failure", args: []string{"--image=updater"}, images: []string{"updater"}, fail: true, failTool: "docker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			stub := `#!/bin/bash
set -eu
tool="${0##*/}"
printf '%s %s\n' "$tool" "$*" >> "$VERIFY_TEST_LOG"
if [[ "$tool" == "$VERIFY_TEST_FAIL" ]]; then exit 19; fi
if [[ "$tool" == docker && "$*" == 'image inspect '* ]]; then
 case "$*" in
  *trading-workbench:verify*) printf '%s\n' 'linux/amd64 app ["/app/trading","-service","workbench"] ["-config","/app/config.yaml"]' ;;
  *) printf '%s\n' 'linux/amd64 app ["/app/trading","-service","updater"] ["-config","/app/config.yaml"]' ;;
 esac
fi
if [[ "$tool" == go ]]; then
 case "$*" in
  *-tags=deployment*) if [[ "$VERIFY_TEST_FAIL" == mysql ]]; then exit 23; fi ;;
  'list '*) printf 'trading\n' ;;
  'test '*) for arg in "$@"; do case "$arg" in -coverprofile=*) printf 'mode: set\n' > "${arg#-coverprofile=}";; esac; done ;;
  'tool cover '*) if [[ "$VERIFY_TEST_FAIL" == coverage ]]; then printf 'total: (statements) 0.0%%\n'; else printf 'total: (statements) 100.0%%\n'; fi ;;
 esac
fi
`
			for _, tool := range []string{"go", "npm", "docker"} {
				if err := os.WriteFile(filepath.Join(bin, tool), []byte(stub), 0700); err != nil {
					t.Fatal(err)
				}
			}
			log := filepath.Join(dir, "commands")
			cmd := exec.Command("bash", append([]string{"scripts/verify.sh"}, tc.args...)...)
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "VERIFY_TEST_LOG="+log, "VERIFY_TEST_FAIL="+tc.failTool)
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.fail {
				t.Fatalf("exit mismatch: %v, %s", err, out)
			}
			data, err := os.ReadFile(log)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			calls := string(data)
			for _, check := range []struct {
				needle string
				want   bool
			}{
				{"npm --prefix web", tc.local}, {"-tags=deployment", tc.mysql}, {"docker buildx build", len(tc.images) > 0},
			} {
				if strings.Contains(calls, check.needle) != check.want {
					t.Errorf("dispatch %q want %v: %s", check.needle, check.want, calls)
				}
			}
			if !tc.fail {
				for _, role := range []string{"updater", "workbench", "updater-configured"} {
					want := false
					for _, selected := range tc.images {
						want = want || selected == role
					}
					if strings.Contains(calls, "--target "+role+" ") != want {
						t.Errorf("target %s: %s", role, calls)
					}
				}
				if strings.Contains(calls, "go test -race ./...") != tc.full {
					t.Errorf("unexpected race selection: %s", calls)
				}
				if strings.Contains(calls, "-coverprofile=") != tc.full {
					t.Errorf("unexpected coverage selection: %s", calls)
				}
				if tc.local && !strings.Contains(calls, "go vet ./...") {
					t.Error("missing vet")
				}
				if tc.full && strings.Count(calls, "-coverprofile=") != 1 {
					t.Error("coverage must run once, derive core profiles from it")
				}
				if tc.mysql && !strings.Contains(calls, "-tags=integration") {
					t.Error("missing integration tests")
				}
				if strings.Contains(calls, "--target updater-configured ") && !strings.Contains(calls, "--no-cache-filter updater-configured") {
					t.Error("configured image can reuse old configuration")
				}
				if strings.Contains(calls, "--no-cache ") {
					t.Error("dependency caches must be reused")
				}
			}
			if tc.fail && strings.Contains(string(out), "所选检查通过") {
				t.Error("failure reported success")
			}
		})
	}
}
