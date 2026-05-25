package crypto

import (
	"crypto/des"
)

type DESCipher struct{}
type TripleDESCipher struct{}

func (c *DESCipher) Name() string   { return "des" }
func (c *DESCipher) KeySize() int   { return 8 }
func (c *DESCipher) BlockSize() int { return 8 }

func (c *TripleDESCipher) Name() string   { return "3des" }
func (c *TripleDESCipher) KeySize() int   { return 24 }
func (c *TripleDESCipher) BlockSize() int { return 8 }

func (c *DESCipher) Encrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	block, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return blockEncrypt(block, iv, data, mode)
}

func (c *DESCipher) Decrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	block, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return blockDecrypt(block, iv, data, mode)
}

func (c *TripleDESCipher) Encrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return nil, err
	}
	return blockEncrypt(block, iv, data, mode)
}

func (c *TripleDESCipher) Decrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return nil, err
	}
	return blockDecrypt(block, iv, data, mode)
}
