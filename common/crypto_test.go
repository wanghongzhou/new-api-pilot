package common

import (
	"errors"
	"testing"
)

func TestCipherRoundTripAndAADBinding(t *testing.T) {
	cipher, err := NewCipher([]byte("abcdefghijklmnopqrstuvwxyz123456"))
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}
	encrypted, err := cipher.Encrypt([]byte("secret-value"), "site:1:access_token")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	plaintext, err := cipher.Decrypt(encrypted, "site:1:access_token")
	if err != nil || string(plaintext) != "secret-value" {
		t.Fatalf("Decrypt() = %q, %v", plaintext, err)
	}
	if _, err := cipher.Decrypt(encrypted, "site:2:access_token"); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("wrong AAD error = %v", err)
	}
	if len(cipher.KeyID()) != 64 {
		t.Fatalf("key fingerprint length = %d", len(cipher.KeyID()))
	}
}

func TestNewCipherRejectsWrongKeyLength(t *testing.T) {
	if _, err := NewCipher(make([]byte, 31)); err == nil {
		t.Fatal("NewCipher() accepted a 31-byte key")
	}
}
