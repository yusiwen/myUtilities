package crypto

import (
	"fmt"

	corecrypto "github.com/yusiwen/myUtilities/core/crypto"
)

type Options struct {
	Sm4    Sm4Options    `cmd:"" name:"sm4" help:"SM4 encrypt/decrypt."`
	Passwd PasswdOptions `cmd:"" name:"passwd" help:"Generate random password."`
}

type Sm4Options struct {
	corecrypto.CommonOptions `embed:""`
	Mode                     string `name:"mode" enum:"ecb,cbc" default:"ecb" help:"Cipher mode."`
}

type PasswdOptions struct {
	Length int `short:"l" name:"length" default:"32" help:"Password length (min 8)."`
}

func (o *PasswdOptions) Run() error {
	pw, err := corecrypto.GeneratePassword(o.Length)
	if err != nil {
		return err
	}
	fmt.Print(pw)
	return nil
}

func (o *Sm4Options) Run() error {
	cipher := &corecrypto.SM4Cipher{}
	mode := corecrypto.CipherMode(o.Mode)

	key, err := o.ResolveKey(cipher.KeySize())
	if err != nil {
		return err
	}

	var iv []byte
	if mode == corecrypto.ModeCBC {
		iv, err = o.ResolveIV(cipher.BlockSize())
		if err != nil {
			return err
		}
	}

	raw, err := o.ReadInput()
	if err != nil {
		return err
	}

	data, err := o.ParseInput(raw)
	if err != nil {
		return err
	}

	var result []byte
	if o.Encrypt {
		result, err = cipher.Encrypt(key, iv, data, mode)
	} else if o.Decrypt {
		result, err = cipher.Decrypt(key, iv, data, mode)
	} else {
		return fmt.Errorf("either --encrypt or --decrypt is required")
	}
	if err != nil {
		return err
	}

	formatted, err := o.FormatOutput(result)
	if err != nil {
		return err
	}

	return o.WriteOutput(formatted)
}
