package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"sync"
)

const encryptedPrefix = "enc:v1:"

var (
	gcmCipher cipher.AEAD
	cryptoMu  sync.RWMutex
)

func InitCrypto() {
	cryptoMu.Lock()
	gcmCipher = nil
	cryptoMu.Unlock()
	log.Println("账号凭证加密密钥等待从本地 KV 加载")
}

func SetCryptoKey(raw string) error {
	if raw == "" {
		return ErrCryptoKeyMissing
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return err
	}
	if len(key) != 32 {
		return errors.New("解码后长度必须为 32 字节")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	cryptoMu.Lock()
	gcmCipher = aead
	cryptoMu.Unlock()
	log.Println("账号凭证加密已启用 (AES-256-GCM)")
	return nil
}

func GenerateBase64Secret(size int) (string, error) {
	if size <= 0 {
		return "", errors.New("size must be positive")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func CryptoEnabled() bool {
	cryptoMu.RLock()
	defer cryptoMu.RUnlock()
	return gcmCipher != nil
}

func Encrypt(plain string) string {
	if plain == "" {
		return ""
	}
	if IsEncrypted(plain) {
		return plain
	}
	cryptoMu.RLock()
	aead := gcmCipher
	cryptoMu.RUnlock()
	if aead == nil {
		return plain
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return plain
	}
	sealed := aead.Seal(nonce, nonce, []byte(plain), nil)
	return encryptedPrefix + base64.StdEncoding.EncodeToString(sealed)
}

func Decrypt(cipherText string) string {
	if !IsEncrypted(cipherText) {
		return cipherText
	}
	cryptoMu.RLock()
	aead := gcmCipher
	cryptoMu.RUnlock()
	if aead == nil {
		return cipherText
	}
	raw, err := base64.StdEncoding.DecodeString(cipherText[len(encryptedPrefix):])
	if err != nil {
		return cipherText
	}
	nonceSize := aead.NonceSize()
	if len(raw) < nonceSize {
		return cipherText
	}
	nonce, ct := raw[:nonceSize], raw[nonceSize:]
	plain, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return cipherText
	}
	return string(plain)
}

func IsEncrypted(s string) bool {
	return len(s) > len(encryptedPrefix) && s[:len(encryptedPrefix)] == encryptedPrefix
}

var ErrCryptoKeyMissing = errors.New("账号凭证加密密钥未初始化")
