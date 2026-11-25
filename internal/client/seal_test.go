package client

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

// open is what the controllers do: the other end of SealPrivateKey.
func open(t *testing.T, private *ecdh.PrivateKey, blobB64, projectID string) ([]byte, error) {
	t.Helper()
	blob, err := base64.StdEncoding.DecodeString(blobB64)
	if err != nil {
		return nil, err
	}
	if string(blob[:5]) != "TBOX1" {
		t.Fatalf("magic = %q", blob[:5])
	}
	ephemeralPublic, nonce, sealed := blob[5:37], blob[37:49], blob[49:]
	peer, err := ecdh.X25519().NewPublicKey(ephemeralPublic)
	if err != nil {
		return nil, err
	}
	shared, err := private.ECDH(peer)
	if err != nil {
		return nil, err
	}
	salt := append(append([]byte{}, ephemeralPublic...), private.PublicKey().Bytes()...)
	key, err := hkdf.Key(sha256.New, shared, salt, "tebib-cert-import-v1", 32)
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	return gcm.Open(nil, nonce, sealed, []byte("tebib-cert-import:"+projectID))
}

func TestSealPrivateKeyRoundTripsAndHidesTheKey(t *testing.T) {
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public := base64.StdEncoding.EncodeToString(private.PublicKey().Bytes())
	secret := []byte("-----BEGIN PRIVATE KEY-----\nSECRETBODY\n-----END PRIVATE KEY-----\n")
	sealed, err := SealPrivateKey(public, secret, "p-one")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(sealed)
	if strings.Contains(string(raw), "SECRETBODY") {
		t.Fatal("the sealed blob carries the key in clear")
	}
	got, err := open(t, private, sealed, "p-one")
	if err != nil || string(got) != string(secret) {
		t.Fatalf("open = %q, %v", got, err)
	}
	again, _ := SealPrivateKey(public, secret, "p-one")
	if again == sealed {
		t.Fatal("two seals of the same key are identical: the ephemeral key is reused")
	}
}

func TestSealPrivateKeyRefusesAMalformedPublicKey(t *testing.T) {
	for _, bad := range []string{"", "not base64!!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := SealPrivateKey(bad, []byte("k"), "p-one"); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestOnlyTheRecipientCanOpenIt(t *testing.T) {
	right, _ := ecdh.X25519().GenerateKey(rand.Reader)
	wrong, _ := ecdh.X25519().GenerateKey(rand.Reader)
	sealed, err := SealPrivateKey(base64.StdEncoding.EncodeToString(right.PublicKey().Bytes()), []byte("k"), "p-one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := open(t, wrong, sealed, "p-one"); err == nil {
		t.Fatal("another key opened the box")
	}
}

func TestTheBoxIsBoundToTheProject(t *testing.T) {
	private, _ := ecdh.X25519().GenerateKey(rand.Reader)
	public := base64.StdEncoding.EncodeToString(private.PublicKey().Bytes())
	sealed, err := SealPrivateKey(public, []byte("k"), "p-one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := open(t, private, sealed, "p-two"); err == nil {
		t.Fatal("a box made for one project opened as another's")
	}
}
