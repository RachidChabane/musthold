package mutantvault

import (
	"bytes"
	"testing"

	"filippo.io/age"
	"pgregory.net/rapid"
)

func newIdentity(t testing.TB) *age.X25519Identity {
	t.Helper()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"source text [verifies SP-47~1]", []byte("def f():\n    return 1\n")},
		{"empty [verifies SP-47~1]", []byte{}},
		{"binary [verifies SP-47~1]", []byte{0x00, 0xff, 0x10, 0x80}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity := newIdentity(t)
			sealed, err := Encrypt(test.plaintext, identity.Recipient().String())
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			opened, err := Decrypt(sealed, identity.String())
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(opened, test.plaintext) {
				t.Fatalf("decrypted %q, want %q", opened, test.plaintext)
			}
		})
	}
}

func TestDecryptRefusesOtherIdentity(t *testing.T) {
	t.Run("another holder cannot read [verifies SP-47~1]", func(t *testing.T) {
		owner, other := newIdentity(t), newIdentity(t)
		sealed, err := Encrypt([]byte("secret mutant"), owner.Recipient().String())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decrypt(sealed, other.String()); err == nil {
			t.Fatal("expected a decryption error")
		}
	})
}

func TestInvalidKeyStrings(t *testing.T) {
	t.Run("invalid recipient [verifies SP-47~1]", func(t *testing.T) {
		for _, recipient := range []string{"", "not-a-recipient", "age1qqqq"} {
			if _, err := Encrypt([]byte("x"), recipient); err == nil {
				t.Errorf("recipient %q accepted", recipient)
			}
		}
	})
	t.Run("invalid identity [verifies SP-47~1]", func(t *testing.T) {
		identity := newIdentity(t)
		sealed, err := Encrypt([]byte("x"), identity.Recipient().String())
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"", "AGE-SECRET-KEY-1INVALID", identity.Recipient().String()} {
			if _, err := Decrypt(sealed, bad); err == nil {
				t.Errorf("identity %q accepted", bad)
			}
		}
	})
	t.Run("corrupted ciphertext [verifies SP-47~1]", func(t *testing.T) {
		identity := newIdentity(t)
		if _, err := Decrypt([]byte("not an age file"), identity.String()); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestEncryptThenDecryptReturnsOriginalBytes(t *testing.T) {
	identity := newIdentity(t)
	recipient := identity.Recipient().String()
	rapid.Check(t, func(t *rapid.T) {
		plaintext := rapid.SliceOf(rapid.Byte()).Draw(t, "plaintext")
		sealed, err := Encrypt(plaintext, recipient)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		if bytes.Equal(sealed, plaintext) {
			t.Fatalf("ciphertext equals plaintext")
		}
		opened, err := Decrypt(sealed, identity.String())
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if !bytes.Equal(opened, plaintext) {
			t.Fatalf("round trip changed the bytes")
		}
	})
}
