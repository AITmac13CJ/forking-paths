package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"forking-paths/internal/store"
)

func (s *Server) uploadCover(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, &store.ValidationError{Msg: "图片过大或格式不正确"})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, &store.ValidationError{Msg: "请选择一张图片"})
		return
	}
	defer file.Close()

	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		writeErr(w, err)
		return
	}
	ext := imageExt(head[:n])
	if ext == "" {
		writeErr(w, &store.ValidationError{Msg: "只接受 JPG、PNG、WebP 或 GIF"})
		return
	}

	dir, err := CoversDir()
	if err != nil {
		writeErr(w, err)
		return
	}
	name, err := randomName(ext)
	if err != nil {
		writeErr(w, err)
		return
	}
	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer dst.Close()
	if _, err := dst.Write(head[:n]); err != nil {
		_ = os.Remove(dst.Name())
		writeErr(w, err)
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		_ = os.Remove(dst.Name())
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"url": "/covers/" + name})
}

func CoversDir() (string, error) {
	for _, p := range []string{"frontend/public/covers", "../frontend/public/covers"} {
		info, err := os.Stat(p)
		if err == nil && info.IsDir() {
			return filepath.Abs(p)
		}
	}
	return "", &store.ValidationError{Msg: "封面目录不存在"}
}

func imageExt(head []byte) string {
	switch {
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return ".jpg"
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		return ".png"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return ".webp"
	case len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return ".gif"
	default:
		return ""
	}
}

func randomName(ext string) (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf) + ext, nil
}

func ServeCovers(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/covers/")
		if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(name)))
	})
}
