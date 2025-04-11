package mock

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func (o FileServerOptions) Run() error {
	// Make local directory
	if err := os.MkdirAll(o.LocalDir, os.ModePerm); err != nil {
		return fmt.Errorf("create local directory failed: %v", err)
	}

	http.HandleFunc("/api/mock/file", o.uploadHandler)

	fmt.Printf("Server listening at :%d\n", o.Port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", o.Port), nil); err != nil {
		return fmt.Errorf("server listen failed: %v", err)
	}
	return nil
}

func (o FileServerOptions) uploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"code": "0", "msg": "POST method only"}`, http.StatusOK)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, o.MaxFileSize*1024*1024)

	if err := r.ParseMultipartForm(o.MaxFileSize * 1024 * 1024); err != nil {
		http.Error(w, fmt.Sprintf(`{"code": "0", "msg": "request body too large: %v"}`, err), http.StatusOK)
		return
	}

	file, header, err := r.FormFile(o.FormKey)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"code": "0", "msg": "no files in request: %v"}`, err), http.StatusOK)
		return
	}
	defer file.Close()

	if header.Filename == "" {
		http.Error(w, `{"code": "0", "msg": "invalid file name"}`, http.StatusOK)
		return
	}

	dstPath := filepath.Join(o.LocalDir, filepath.Base(header.Filename))
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

	log.Println("File uploaded: %s", dstPath)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{
        "code": "1",
        "msg": "OK"
    }`)
}
