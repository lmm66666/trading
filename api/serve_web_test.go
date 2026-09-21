package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAttachWebUI(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<main>workbench</main>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("app"), 0o600); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(KernelServices{})
	if err := AttachWebUI(router, dir); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/", "/watch/SZSE:002415"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.String() != "<main>workbench</main>" {
			t.Fatalf("path %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
	asset := httptest.NewRecorder()
	router.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if asset.Code != http.StatusOK || asset.Body.String() != "app" {
		t.Fatalf("asset: status=%d body=%q", asset.Code, asset.Body.String())
	}
	missingAPI := httptest.NewRecorder()
	router.ServeHTTP(missingAPI, httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil))
	if missingAPI.Code != http.StatusNotFound {
		t.Fatalf("missing api: status=%d body=%q", missingAPI.Code, missingAPI.Body.String())
	}
	// 契约（docs/standards/http-api.md）：错误时 code 为 HTTP 状态整数，message 为稳定标识。
	var missingBody struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data"`
	}
	if err := json.Unmarshal(missingAPI.Body.Bytes(), &missingBody); err != nil {
		t.Fatalf("missing api body is not JSON: %q", missingAPI.Body.String())
	}
	if missingBody.Code != http.StatusNotFound || missingBody.Message != "NOT_FOUND" || missingBody.Data != nil {
		t.Fatalf("missing api envelope: %+v", missingBody)
	}
	missingMethod := httptest.NewRecorder()
	router.ServeHTTP(missingMethod, httptest.NewRequest(http.MethodPost, "/missing", nil))
	if missingMethod.Code != http.StatusNotFound || missingMethod.Body.String() == "<main>workbench</main>" {
		t.Fatalf("missing method: status=%d body=%q", missingMethod.Code, missingMethod.Body.String())
	}
}

func TestAttachWebUIRequiresBuildOutput(t *testing.T) {
	if err := AttachWebUI(NewRouter(KernelServices{}), t.TempDir()); err == nil {
		t.Fatal("expected missing index error")
	}
}
