package crypto

import (
	"crypto/cipher"
	"fmt"

	"github.com/tjfoc/gmsm/sm4"
)

type SM4Cipher struct{}

func (c *SM4Cipher) Name() string   { return "sm4" }
func (c *SM4Cipher) KeySize() int   { return 16 }
func (c *SM4Cipher) BlockSize() int { return 16 }

func (c *SM4Cipher) Encrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	switch mode {
	case ModeECB:
		return sm4.Sm4Ecb(key, data, true)
	case ModeCBC:
		block, err := sm4.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("create cipher: %w", err)
		}
		padded := pkcs7Pad(data, block.BlockSize())
		encrypted := make([]byte, len(padded))
		stream := cipher.NewCBCEncrypter(block, iv)
		stream.CryptBlocks(encrypted, padded)
		return encrypted, nil
	default:
		return nil, fmt.Errorf("unsupported mode: %s", mode)
	}
}

func (c *SM4Cipher) Decrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	switch mode {
	case ModeECB:
		return sm4.Sm4Ecb(key, data, false)
	case ModeCBC:
		block, err := sm4.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("create cipher: %w", err)
		}
		if len(data)%block.BlockSize() != 0 {
			return nil, fmt.Errorf("ciphertext is not a multiple of block size")
		}
		decrypted := make([]byte, len(data))
		stream := cipher.NewCBCDecrypter(block, iv)
		stream.CryptBlocks(decrypted, data)
		return pkcs7Unpad(decrypted, block.BlockSize())
	default:
		return nil, fmt.Errorf("unsupported mode: %s", mode)
	}
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	padded := make([]byte, len(data)+padLen)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}
	return padded
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	if len(data)%blockSize != 0 {
		return nil, fmt.Errorf("data is not a multiple of block size")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize {
		return nil, fmt.Errorf("invalid padding")
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:len(data)-padLen], nil
}
