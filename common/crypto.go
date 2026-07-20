package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const encryptedValueVersion = "v1"

var ErrInvalidCiphertext = errors.New("invalid encrypted value")

type Cipher struct {
	aead  cipher.AEAD
	keyID string
}

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES-256-GCM key must contain exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &Cipher{aead: aead, keyID: KeyFingerprint(key)}, nil
}

func KeyFingerprint(key []byte) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])
}

func (c *Cipher) KeyID() string {
	return c.keyID
}

func (c *Cipher) Encrypt(plaintext []byte, aad string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate encryption nonce: %w", err)
	}
	ciphertext := c.aead.Seal(nil, nonce, plaintext, []byte(aad))
	return strings.Join([]string{
		encryptedValueVersion,
		base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(ciphertext),
	}, ":"), nil
}

func (c *Cipher) Decrypt(value, aad string) ([]byte, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 || parts[0] != encryptedValueVersion {
		return nil, ErrInvalidCiphertext
	}
	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(nonce) != c.aead.NonceSize() {
		return nil, ErrInvalidCiphertext
	}
	ciphertext, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil || len(ciphertext) < c.aead.Overhead() {
		return nil, ErrInvalidCiphertext
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}
