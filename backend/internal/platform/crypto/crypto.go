// Package crypto implements envelope encryption for secrets at rest.
//
// A random 256-bit data key encrypts each payload with AES-GCM; the data key
// itself is wrapped (AES-GCM) by a master key derived from the
// MDCP_ENCRYPTION_KEY environment value. Postgres stores only ciphertext,
// the wrapped data key, nonces and a key id — never plaintext.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// Encryptor wraps and unwraps payloads with a single master key.
type Encryptor struct {
	master [32]byte
	keyID  string
}

// NewEncryptor derives a 256-bit master key from the given secret string.
func NewEncryptor(secret, keyID string) *Encryptor {
	return &Encryptor{master: sha256.Sum256([]byte(secret)), keyID: keyID}
}

// KeyID identifies the master key that wrapped a data key.
func (e *Encryptor) KeyID() string { return e.keyID }

// Envelope is the encrypted form of a payload.
type Envelope struct {
	Ciphertext       []byte // payload sealed by the data key
	EncryptedDataKey []byte // data-key nonce || wrapped data key
	Nonce            []byte // payload nonce
	KeyID            string
}

// Encrypt seals plaintext under a fresh data key.
func (e *Encryptor) Encrypt(plaintext []byte) (Envelope, error) {
	var dataKey [32]byte
	if _, err := rand.Read(dataKey[:]); err != nil {
		return Envelope{}, fmt.Errorf("crypto: generate data key: %w", err)
	}

	ciphertext, nonce, err := seal(dataKey[:], plaintext)
	if err != nil {
		return Envelope{}, err
	}
	wrapped, keyNonce, err := seal(e.master[:], dataKey[:])
	if err != nil {
		return Envelope{}, err
	}

	return Envelope{
		Ciphertext:       ciphertext,
		EncryptedDataKey: append(keyNonce, wrapped...),
		Nonce:            nonce,
		KeyID:            e.keyID,
	}, nil
}

// Decrypt unwraps the data key and opens the payload.
func (e *Encryptor) Decrypt(env Envelope) ([]byte, error) {
	gcm, err := newGCM(e.master[:])
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(env.EncryptedDataKey) < ns {
		return nil, fmt.Errorf("crypto: malformed encrypted data key")
	}
	dataKey, err := gcm.Open(nil, env.EncryptedDataKey[:ns], env.EncryptedDataKey[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: unwrap data key: %w", err)
	}

	pgcm, err := newGCM(dataKey)
	if err != nil {
		return nil, err
	}
	plaintext, err := pgcm.Open(nil, env.Nonce, env.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: open payload: %w", err)
	}
	return plaintext, nil
}

func seal(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}
	return gcm.Seal(nil, nonce, plaintext, nil), nonce, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
