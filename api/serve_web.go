package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// AttachWebUI 将已构建的单页应用挂载到现有 API 路由。
func AttachWebUI(router *gin.Engine, webDir string) error {
	if router == nil || strings.TrimSpace(webDir) == "" {
		return fmt.Errorf("attach web ui: router and directory are required")
	}
	root, err := filepath.Abs(webDir)
	if err != nil {
		return fmt.Errorf("attach web ui: resolve directory: %w", err)
	}
	indexPath := filepath.Join(root, "index.html")
	info, err := os.Stat(indexPath)
	if err != nil {
		return fmt.Errorf("attach web ui: inspect index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("attach web ui: index is not a regular file")
	}
	assetsPath := filepath.Join(root, "assets")
	if assets, statErr := os.Stat(assetsPath); statErr == nil && assets.IsDir() {
		router.Use(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			}
			c.Next()
		})
		router.StaticFS("/assets", http.Dir(assetsPath))
	}
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/api" || strings.HasPrefix(path, "/api/") || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
			c.JSON(http.StatusNotFound, gin.H{"code": "NOT_FOUND", "message": "resource not found"})
			return
		}
		c.File(indexPath)
	})
	return nil
}
