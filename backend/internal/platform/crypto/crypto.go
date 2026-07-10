// Package crypto implements envelope encryption for secrets at rest.
//
// A random 256-bit data key encrypts each payload with AES-GCM; the data key
// itself is wrapped (AES-GCM) by a master key derived from the
// FLEETDOCK_ENCRYPTION_KEY environment value. Postgres stores only ciphertext,
// the wrapped data key, nonces and a key id — never plaintext.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// Encryptor wraps and unwraps payloads with a keyring of master keys. New
// payloads are sealed under the primary key; decryption selects the master key
// named by the envelope's KeyID, so old keys can still be read during a
// rotation window.
type Encryptor struct {
	keys    map[string][32]byte
	primary string
}

// NewEncryptor derives a single 256-bit master key from the given secret.
func NewEncryptor(secret, keyID string) *Encryptor {
	return &Encryptor{
		keys:    map[string][32]byte{keyID: sha256.Sum256([]byte(secret))},
		primary: keyID,
	}
}

// NewKeyring builds an encryptor from several named secrets. New writes use the
// primary key; every listed key can still decrypt. primaryID must be present.
func NewKeyring(primaryID string, secrets map[string]string) (*Encryptor, error) {
	keys := make(map[string][32]byte, len(secrets))
	for id, secret := range secrets {
		keys[id] = sha256.Sum256([]byte(secret))
	}
	if _, ok := keys[primaryID]; !ok {
		return nil, fmt.Errorf("crypto: primary key %q missing from keyring", primaryID)
	}
	return &Encryptor{keys: keys, primary: primaryID}, nil
}

// KeyID identifies the primary master key that wraps new data keys.
func (e *Encryptor) KeyID() string { return e.primary }

func (e *Encryptor) masterFor(keyID string) ([32]byte, error) {
	k, ok := e.keys[keyID]
	if !ok {
		return [32]byte{}, fmt.Errorf("crypto: no master key %q in keyring", keyID)
	}
	return k, nil
}

// Envelope is the encrypted form of a payload.
type Envelope struct {
	Ciphertext       []byte // payload sealed by the data key
	EncryptedDataKey []byte // data-key nonce || wrapped data key
	Nonce            []byte // payload nonce
	KeyID            string
}

// Encrypt seals plaintext under a fresh data key wrapped by the primary key.
func (e *Encryptor) Encrypt(plaintext []byte) (Envelope, error) {
	var dataKey [32]byte
	if _, err := rand.Read(dataKey[:]); err != nil {
		return Envelope{}, fmt.Errorf("crypto: generate data key: %w", err)
	}

	ciphertext, nonce, err := seal(dataKey[:], plaintext)
	if err != nil {
		return Envelope{}, err
	}
	master := e.keys[e.primary]
	wrapped, keyNonce, err := seal(master[:], dataKey[:])
	if err != nil {
		return Envelope{}, err
	}

	return Envelope{
		Ciphertext:       ciphertext,
		EncryptedDataKey: append(keyNonce, wrapped...),
		Nonce:            nonce,
		KeyID:            e.primary,
	}, nil
}

// unwrapDataKey opens the wrapped data key using the master named by env.KeyID.
func (e *Encryptor) unwrapDataKey(env Envelope) ([]byte, error) {
	master, err := e.masterFor(env.KeyID)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(master[:])
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
	return dataKey, nil
}

// Decrypt unwraps the data key and opens the payload.
func (e *Encryptor) Decrypt(env Envelope) ([]byte, error) {
	dataKey, err := e.unwrapDataKey(env)
	if err != nil {
		return nil, err
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

// Rewrap re-encrypts a payload's data key under the primary key without
// touching the payload ciphertext. It returns the new wrapped data key and the
// primary key id. Rewrapping a payload already at the primary key is a no-op
// re-wrap (still valid). The payload plaintext is never exposed.
func (e *Encryptor) Rewrap(env Envelope) (encryptedDataKey []byte, keyID string, err error) {
	dataKey, err := e.unwrapDataKey(env)
	if err != nil {
		return nil, "", err
	}
	master := e.keys[e.primary]
	wrapped, keyNonce, err := seal(master[:], dataKey)
	if err != nil {
		return nil, "", err
	}
	return append(keyNonce, wrapped...), e.primary, nil
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
