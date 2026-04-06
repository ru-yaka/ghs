package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"io"
)

// encrypt encrypts plaintext with AES-256-GCM using a password-derived key.
// Output: base64(salt[32] + nonce[12] + ciphertext + tag[16])
func encrypt(plaintext []byte, password string) (string, error) {
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	buf := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	buf = append(buf, salt...)
	buf = append(buf, nonce...)
	buf = append(buf, ciphertext...)

	return base64.StdEncoding.EncodeToString(buf), nil
}

// decrypt decodes base64 data and decrypts with AES-256-GCM.
func decrypt(encoded string, password string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid data: not base64")
	}

	if len(data) < 32+12+16 {
		return nil, fmt.Errorf("invalid data: too short")
	}

	salt := data[:32]
	nonce := data[32 : 32+12]
	ciphertext := data[32+12:]

	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("wrong key or corrupted data")
	}

	return plaintext, nil
}

// deriveKey derives a 256-bit key from password + salt using iterated SHA-256.
func deriveKey(password string, salt []byte) []byte {
	h := sha256.New()
	h.Write([]byte(password))
	h.Write(salt)
	key := h.Sum(nil)
	for i := 0; i < 10000; i++ {
		h.Reset()
		h.Write(key)
		key = h.Sum(nil)
	}
	return key
}

// generateKey creates a random human-readable key (base32, 32 chars, 160 bits).
func generateKey() (string, error) {
	b := make([]byte, 20)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}
