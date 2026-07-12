package qrcode

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	gqrcode "github.com/skip2/go-qrcode"
	"golang.org/x/term"
)

type Options struct {
	Gen   GenOptions   `cmd:"" name:"gen" aliases:"g" help:"Generate QR code from text."`
	Serve ServeOptions `cmd:"" name:"serve" help:"Start QR code HTTP server."`
}

type GenOptions struct {
	Text   string `arg:"" optional:"" name:"text" help:"Text to encode (or pipe from stdin)."`
	Output string `short:"o" help:"Save as PNG image file."`
	Level  string `enum:"low,medium,high" default:"medium" help:"Error correction level."`
}

type ServeOptions struct {
	Port int `help:"Port to listen on." default:"8085"`
}

func (o *GenOptions) Run() error {
	text, err := o.resolveText()
	if err != nil {
		return err
	}

	level, err := o.parseLevel()
	if err != nil {
		return err
	}

	qr, err := gqrcode.New(text, level)
	if err != nil {
		return fmt.Errorf("generate qr: %w", err)
	}

	if o.Output != "" {
		if err := qr.WriteFile(256, o.Output); err != nil {
			return fmt.Errorf("write file: %w", err)
		}
		fmt.Printf("QR code saved to %s\n", o.Output)
		return nil
	}

	fmt.Println(qr.ToSmallString(false))
	return nil
}

func (o *ServeOptions) Run() error {
	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux)
	fmt.Printf("QR code server listening on :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/qrcode", handleGenerate)
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" {
		http.Error(w, "missing text parameter", http.StatusBadRequest)
		return
	}

	level := gqrcode.Medium
	switch r.URL.Query().Get("level") {
	case "low":
		level = gqrcode.Low
	case "high":
		level = gqrcode.High
	}

	qr, err := gqrcode.New(text, level)
	if err != nil {
		http.Error(w, fmt.Sprintf("generate qr: %v", err), http.StatusInternalServerError)
		return
	}

	png, err := qr.PNG(256)
	if err != nil {
		http.Error(w, fmt.Sprintf("encode png: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

func (o *GenOptions) resolveText() (string, error) {
	if o.Text != "" {
		return o.Text, nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("text is required; pipe input or provide as argument")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimRight(string(data), "\n\r"), nil
}

func (o *GenOptions) parseLevel() (gqrcode.RecoveryLevel, error) {
	switch o.Level {
	case "low":
		return gqrcode.Low, nil
	case "medium":
		return gqrcode.Medium, nil
	case "high":
		return gqrcode.High, nil
	}
	return 0, fmt.Errorf("invalid level: %s", o.Level)
}
