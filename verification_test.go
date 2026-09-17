package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests execute the real gate selector with isolated command substitutes.
// They verify dispatch and failure propagation, not MySQL or image correctness.
func TestVerificationSelection(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		args                      []string
		mysql, image, local, fail bool
		failTool                  string
	}{
		{name: "default_local", local: true},
		{name: "mysql", args: []string{"--mysql"}, mysql: true, local: true},
		{name: "image", args: []string{"--image"}, image: true, local: true},
		{name: "full", args: []string{"--full"}, mysql: true, image: true, local: true},
		{name: "combined", args: []string{"--mysql", "--image"}, mysql: true, image: true, local: true},
		{name: "help", args: []string{"--help"}},
		{name: "invalid_option", args: []string{"--my-sql"}, fail: true},
		{name: "local_failure", local: true, fail: true, failTool: "npm"},
		{name: "mysql_failure", args: []string{"--full"}, mysql: true, local: true, fail: true, failTool: "mysql"},
		{name: "image_failure", args: []string{"--image"}, image: true, local: true, fail: true, failTool: "docker"},
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
if [[ "$tool" == go ]]; then
 case "$*" in
  *-tags=deployment*) if [[ "$VERIFY_TEST_FAIL" == mysql ]]; then exit 23; fi ;;
  'list '*) printf 'trading\n' ;;
  'tool cover '*) printf 'total: (statements) 100.0%%\n' ;;
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
				t.Fatalf("exit mismatch: err=%v, output=%s", err, out)
			}
			data, readErr := os.ReadFile(log)
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			calls := string(data)
			for _, check := range []struct {
				needle string
				want   bool
			}{
				{"npm --prefix web ci", tc.local},
				{"-tags=deployment", tc.mysql},
				{"docker buildx build --platform linux/amd64", tc.image},
			} {
				if strings.Contains(calls, check.needle) != check.want {
					t.Errorf("dispatch %q: want %v, calls=%s", check.needle, check.want, calls)
				}
			}
			if tc.mysql && tc.failTool != "mysql" && !strings.Contains(calls, "-tags=integration") {
				t.Error("selected MySQL did not run integration gate")
			}
			if !tc.local && len(data) != 0 {
				t.Errorf("help/invalid options executed commands: %s", data)
			}
			if tc.fail && strings.Contains(string(out), "所选门禁全部通过") {
				t.Error("failed gate reported success")
			}
			if tc.local && !tc.fail {
				if !strings.Contains(calls, "go test -race ./...") || !strings.Contains(calls, "go vet ./...") {
					t.Error("local checks omitted")
				}
				if !tc.mysql && !strings.Contains(string(out), "MySQL：未选择") {
					t.Error("unselected MySQL not disclosed")
				}
				if !tc.image && !strings.Contains(string(out), "镜像：未选择") {
					t.Error("unselected image not disclosed")
				}
			}
		})
	}
}
