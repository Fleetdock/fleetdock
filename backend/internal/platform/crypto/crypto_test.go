package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	enc := NewEncryptor("test-master-key", "master-1")
	plaintext := []byte("s3cr3t-password")

	env, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Contains(env.Ciphertext, plaintext) {
		t.Fatal("ciphertext leaks plaintext")
	}

	out, err := enc.Decrypt(env)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(out, plaintext) {
		t.Fatalf("round trip mismatch: got %q", out)
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	env, err := NewEncryptor("key-a", "master-1").Encrypt([]byte("payload"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := NewEncryptor("key-b", "master-1").Decrypt(env); err == nil {
		t.Fatal("expected decryption with wrong master key to fail")
	}
}

func TestKeyringRewrapRotation(t *testing.T) {
	plaintext := []byte("rotate-me")

	// Seal under the old key.
	old := NewEncryptor("old-secret", "master-1")
	env, err := old.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// A keyring holding both keys, primary = master-2, can read the old envelope
	// and re-wrap its data key under the new key.
	ring, err := NewKeyring("master-2", map[string]string{
		"master-1": "old-secret",
		"master-2": "new-secret",
	})
	if err != nil {
		t.Fatalf("keyring: %v", err)
	}
	if _, err := ring.Decrypt(env); err != nil {
		t.Fatalf("keyring decrypt of old envelope: %v", err)
	}
	edk, keyID, err := ring.Rewrap(env)
	if err != nil {
		t.Fatalf("rewrap: %v", err)
	}
	if keyID != "master-2" {
		t.Fatalf("expected rewrap under master-2, got %q", keyID)
	}
	env.EncryptedDataKey = edk
	env.KeyID = keyID

	// The new key alone must now decrypt the rewrapped envelope; the old key must not.
	newOnly, err := NewKeyring("master-2", map[string]string{"master-2": "new-secret"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := newOnly.Decrypt(env)
	if err != nil {
		t.Fatalf("decrypt after rewrap: %v", err)
	}
	if !bytes.Equal(out, plaintext) {
		t.Fatalf("round trip mismatch after rewrap: got %q", out)
	}
	if _, err := old.Decrypt(env); err == nil {
		t.Fatal("expected the retired key to fail after rewrap")
	}
}

func TestNewKeyringRequiresPrimary(t *testing.T) {
	if _, err := NewKeyring("missing", map[string]string{"master-1": "s"}); err == nil {
		t.Fatal("expected error when primary key is absent from the keyring")
	}
}
