package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// encryptBlob encrypts with a random AES key and embeds it in the output.
// Format: base64(key[32] + nonce[12] + sealedData)
// Self-contained — no separate key needed to decrypt.
func encryptBlob(plaintext []byte) (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}

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

	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	buf := make([]byte, 0, len(key)+len(nonce)+len(sealed))
	buf = append(buf, key...)
	buf = append(buf, nonce...)
	buf = append(buf, sealed...)

	return base64.StdEncoding.EncodeToString(buf), nil
}

// decryptBlob extracts the embedded key and decrypts.
func decryptBlob(encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid data: not base64")
	}

	if len(data) < 32+12+16 {
		return nil, fmt.Errorf("invalid data: too short")
	}

	key := data[:32]
	nonce := data[32:44]
	sealed := data[44:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return gcm.Open(nil, nonce, sealed, nil)
}
