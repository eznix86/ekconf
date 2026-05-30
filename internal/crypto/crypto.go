package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	SaltLen  = 16
	NonceLen = 12

	ArgonMemory  = 64 * 1024
	ArgonTime    = 3
	ArgonThreads = 4
	ArgonKeyLen  = 32
)

type EncryptedFile struct {
	Salt       []byte
	Nonce      []byte
	Ciphertext []byte
}

func deriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, ArgonTime, ArgonMemory, ArgonThreads, ArgonKeyLen)
}

func Encrypt(plaintext []byte, password string) (*EncryptedFile, error) {
	salt := make([]byte, SaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	nonce := make([]byte, NonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	ciphertext := aesGCM.Seal(nil, nonce, plaintext, nil)

	return &EncryptedFile{
		Salt:       salt,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}

func Decrypt(ef *EncryptedFile, password string) ([]byte, error) {
	key := deriveKey(password, ef.Salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	plaintext, err := aesGCM.Open(nil, ef.Nonce, ef.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}

func Marshal(ef *EncryptedFile) []byte {
	buf := make([]byte, 0, SaltLen+NonceLen+len(ef.Ciphertext))
	buf = append(buf, ef.Salt...)
	buf = append(buf, ef.Nonce...)
	buf = append(buf, ef.Ciphertext...)
	return buf
}

func Unmarshal(data []byte) (*EncryptedFile, error) {
	if len(data) < SaltLen+NonceLen {
		return nil, fmt.Errorf("data too short: %d bytes", len(data))
	}

	return &EncryptedFile{
		Salt:       data[:SaltLen],
		Nonce:      data[SaltLen : SaltLen+NonceLen],
		Ciphertext: data[SaltLen+NonceLen:],
	}, nil
}
