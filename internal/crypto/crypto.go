package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"encoding/base64"
	"errors"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	keySize      uint32 = 32
	nonceSize    int    = 12
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 2
)

type Crypto struct{}

func NewCrypto() *Crypto {
	return &Crypto{}
}

func (c *Crypto) DeriveKey(
	password string,
	salt []byte,
) []byte {
	return argon2.IDKey(
		[]byte(password),
		salt,
		argonTime,
		argonMemory,
		argonThreads,
		keySize,
	)
}

func (c *Crypto) Encrypt(
	data []byte,
	key []byte,
) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, nonceSize)

	if _, err := io.ReadFull(
		crand.Reader,
		nonce,
	); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(
		nil,
		nonce,
		data,
		nil,
	)

	raw := append(
		nonce,
		ciphertext...,
	)

	return base64.StdEncoding.EncodeToString(raw), nil
}

func (c *Crypto) Decrypt(
	encoded string,
	key []byte,
) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(
		encoded,
	)
	if err != nil {
		return nil, err
	}

	if len(raw) < nonceSize {
		return nil, errors.New(
			"invalid encrypted data",
		)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := raw[:nonceSize]
	ciphertext := raw[nonceSize:]

	return gcm.Open(
		nil,
		nonce,
		ciphertext,
		nil,
	)
}
