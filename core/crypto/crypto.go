package crypto

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

type CipherMode string

const (
	ModeECB CipherMode = "ecb"
	ModeCBC CipherMode = "cbc"
)

type Cipher interface {
	Name() string
	KeySize() int
	BlockSize() int
	Encrypt(key, iv, data []byte, mode CipherMode) ([]byte, error)
	Decrypt(key, iv, data []byte, mode CipherMode) ([]byte, error)
}

type CommonOptions struct {
	Encrypt      bool   `short:"e" name:"encrypt" help:"Encrypt mode." xor:"mode"`
	Decrypt      bool   `short:"d" name:"decrypt" help:"Decrypt mode." xor:"mode"`
	Key          string `name:"key" help:"Hex-encoded key." xor:"key"`
	PlainKey     string `name:"plain-key" help:"Plain text key." xor:"key"`
	Input        string `name:"input" help:"Input data string." xor:"input"`
	InputFile    string `name:"input-file" help:"Path to input file." xor:"input"`
	OutputFile   string `name:"output-file" help:"Path to output file."`
	InputFormat  string `name:"input-format" enum:"bin,hex" default:"bin" help:"Input format."`
	OutputFormat string `name:"output-format" enum:"bin,hex" default:"bin" help:"Output format."`
	IV           string `name:"iv" help:"Hex-encoded IV (required for CBC mode)." xor:"iv_src"`
	PlainIV      string `name:"plain-iv" help:"Plain text IV." xor:"iv_src"`
}

func (c *CommonOptions) ResolveKey(keySize int) ([]byte, error) {
	if c.PlainKey != "" {
		return padOrTruncate([]byte(c.PlainKey), keySize), nil
	}
	if c.Key != "" {
		decoded, err := hex.DecodeString(c.Key)
		if err != nil {
			return nil, fmt.Errorf("invalid hex key: %w", err)
		}
		return padOrTruncate(decoded, keySize), nil
	}
	return nil, fmt.Errorf("either --key or --plain-key is required")
}

func (c *CommonOptions) ResolveIV(blockSize int) ([]byte, error) {
	if c.PlainIV != "" {
		return padOrTruncate([]byte(c.PlainIV), blockSize), nil
	}
	if c.IV != "" {
		decoded, err := hex.DecodeString(c.IV)
		if err != nil {
			return nil, fmt.Errorf("invalid hex IV: %w", err)
		}
		return padOrTruncate(decoded, blockSize), nil
	}
	return nil, fmt.Errorf("IV is required for CBC mode (--iv or --plain-iv)")
}

func (c *CommonOptions) ReadInput() ([]byte, error) {
	if c.Input != "" {
		return []byte(c.Input), nil
	}
	if c.InputFile != "" {
		return os.ReadFile(c.InputFile)
	}
	return io.ReadAll(os.Stdin)
}

func (c *CommonOptions) ParseInput(raw []byte) ([]byte, error) {
	if c.InputFormat == "hex" {
		decoded, err := hex.DecodeString(string(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid hex input: %w", err)
		}
		return decoded, nil
	}
	return raw, nil
}

func (c *CommonOptions) FormatOutput(data []byte) ([]byte, error) {
	if c.OutputFormat == "hex" {
		return []byte(hex.EncodeToString(data)), nil
	}
	return data, nil
}

func (c *CommonOptions) WriteOutput(data []byte) error {
	if c.OutputFile != "" {
		return os.WriteFile(c.OutputFile, data, 0644)
	}
	_, err := os.Stdout.Write(data)
	return err
}

func padOrTruncate(data []byte, size int) []byte {
	if len(data) == size {
		return data
	}
	if len(data) > size {
		return data[:size]
	}
	padded := make([]byte, size)
	copy(padded, data)
	return padded
}
