package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const localFileRoute = "/local-file"

// LocalFileHandler serves local image files for the AssetServer fallback.
type LocalFileHandler struct{}

func NewLocalFileHandler() *LocalFileHandler {
	return &LocalFileHandler{}
}

func (h *LocalFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasPrefix(r.URL.Path, localFileRoute) {
		http.NotFound(w, r)
		return
	}

	raw := r.URL.Query().Get("path")
	if raw == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	path, err := url.QueryUnescape(raw)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	path = filepath.Clean(path)
	if !isImageExt(filepath.Ext(path)) {
		http.Error(w, "unsupported file type", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

// LocalImageURL returns an AssetServer URL that streams the original image file.
func (a *App) LocalImageURL(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("路径为空")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("不能预览文件夹")
	}
	if !isImageExt(filepath.Ext(path)) {
		return "", fmt.Errorf("不是支持的图片格式")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	q := url.Values{}
	q.Set("path", abs)
	q.Set("t", strconv.FormatInt(info.ModTime().Unix(), 10))
	return localFileRoute + "?" + q.Encode(), nil
}
