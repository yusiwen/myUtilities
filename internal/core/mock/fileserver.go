package mock

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

type FileServer struct {
	LocalDir    string
	FormKey     string
	MaxFileSize int64
}

func NewFileServer(localDir, formKey string, maxFileSize int64) *FileServer {
	return &FileServer{LocalDir: localDir, FormKey: formKey, MaxFileSize: maxFileSize}
}

// Handler returns the HTTP mux for the file upload server.
func (f *FileServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/mock/file", f.uploadHandler)
	mux.HandleFunc("/api/mock/file-error/unknown-fields", f.uploadUnknownHandler)
	mux.HandleFunc("/api/mock/file-error/missing-fields", f.uploadMissingHandler)
	return mux
}

func (f *FileServer) uploadUnknownHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{
        "result": 0,
        "code": "0",
        "msg": "Mocking Error"
    }`)
}

func (f *FileServer) uploadMissingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{
        "msg": "OK"
    }`)
}

func (f *FileServer) uploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"code": "0", "msg": "POST method only"}`, http.StatusOK)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, f.MaxFileSize*1024*1024)

	if err := r.ParseMultipartForm(f.MaxFileSize * 1024 * 1024); err != nil {
		http.Error(w, fmt.Sprintf(`{"code": "0", "msg": "request body too large: %v"}`, err), http.StatusOK)
		return
	}

	file, header, err := r.FormFile(f.FormKey)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"code": "0", "msg": "no files in request: %v"}`, err), http.StatusOK)
		return
	}
	defer file.Close()

	if header.Filename == "" {
		http.Error(w, `{"code": "0", "msg": "invalid file name"}`, http.StatusOK)
		return
	}

	dstPath := filepath.Join(f.LocalDir, filepath.Base(header.Filename))
	dstFile, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"code": "0", "msg": "create file failed: %v"}`, err), http.StatusOK)
		return
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, file); err != nil {
		http.Error(w, fmt.Sprintf(`{"code": "0", "msg": "store file failed: %v"}`, err), http.StatusOK)
		return
	}

	log.Printf("File uploaded: %s", dstPath)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{
        "code": "1",
        "msg": "OK"
    }`)
}
