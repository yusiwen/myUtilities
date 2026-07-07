package qrcode

import (
	"fmt"
	"io"
	"os"
	"strings"

	gqrcode "github.com/skip2/go-qrcode"
	"golang.org/x/term"
)

type Options struct {
	Text   string `arg:"" optional:"" name:"text" help:"Text to encode (or pipe from stdin)."`
	Output string `short:"o" help:"Save as PNG image file."`
	Level  string `enum:"low,medium,high" default:"medium" help:"Error correction level."`
}

func (o *Options) Run() error {
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

func (o *Options) resolveText() (string, error) {
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

func (o *Options) parseLevel() (gqrcode.RecoveryLevel, error) {
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
