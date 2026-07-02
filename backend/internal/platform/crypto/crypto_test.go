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
