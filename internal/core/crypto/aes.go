package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

type AESCipher struct{}

func (c *AESCipher) Name() string   { return "aes" }
func (c *AESCipher) KeySize() int   { return 16 }
func (c *AESCipher) BlockSize() int { return 16 }

func (c *AESCipher) Encrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return blockEncrypt(block, iv, data, mode)
}

func (c *AESCipher) Decrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return blockDecrypt(block, iv, data, mode)
}

func blockEncrypt(block cipher.Block, iv, data []byte, mode CipherMode) ([]byte, error) {
	padded := pkcs7Pad(data, block.BlockSize())
	switch mode {
	case ModeECB:
		return ecbEncrypt(block, padded)
	case ModeCBC:
		enc := make([]byte, len(padded))
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(enc, padded)
		return enc, nil
	default:
		return nil, fmt.Errorf("unsupported mode: %s", mode)
	}
}

func blockDecrypt(block cipher.Block, iv, data []byte, mode CipherMode) ([]byte, error) {
	if len(data)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of block size")
	}
	switch mode {
	case ModeECB:
		dec := make([]byte, len(data))
		if err := ecbDecrypt(block, dec, data); err != nil {
			return nil, err
		}
		return pkcs7Unpad(dec, block.BlockSize())
	case ModeCBC:
		dec := make([]byte, len(data))
		cipher.NewCBCDecrypter(block, iv).CryptBlocks(dec, data)
		return pkcs7Unpad(dec, block.BlockSize())
	default:
		return nil, fmt.Errorf("unsupported mode: %s", mode)
	}
}

func ecbEncrypt(block cipher.Block, data []byte) ([]byte, error) {
	bs := block.BlockSize()
	if len(data)%bs != 0 {
		return nil, fmt.Errorf("data length (%d) not multiple of block size (%d)", len(data), bs)
	}
	result := make([]byte, len(data))
	for i := 0; i < len(data); i += bs {
		block.Encrypt(result[i:i+bs], data[i:i+bs])
	}
	return result, nil
}

func ecbDecrypt(block cipher.Block, dst, src []byte) error {
	bs := block.BlockSize()
	if len(src)%bs != 0 {
		return fmt.Errorf("data length (%d) not multiple of block size (%d)", len(src), bs)
	}
	for i := 0; i < len(src); i += bs {
		block.Decrypt(dst[i:i+bs], src[i:i+bs])
	}
	return nil
}
