package client

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const sealInfo = "tebib-cert-import-v1"

// SealPrivateKey seals secret to the platform's X25519 public key (base64), as the API's
// GET .../certificates/import-key describes, so the API can't read it:
//
//	"TBOX1" || ephemeral_public(32) || nonce(12) || AES-256-GCM(key, nonce, secret, aad)
//	key = HKDF-SHA256(ECDH, salt = ephemeral_public || recipient_public, info = sealInfo)
//	aad = "tebib-cert-import:" + project_id (the project the certificate is imported into)
func SealPrivateKey(publicKeyB64 string, secret []byte, projectID string) (string, error) {
	recipientRaw, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(recipientRaw) != 32 {
		return "", fmt.Errorf("the platform's public key is malformed")
	}
	curve := ecdh.X25519()
	recipient, err := curve.NewPublicKey(recipientRaw)
	if err != nil {
		return "", err
	}
	ephemeral, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	shared, err := ephemeral.ECDH(recipient)
	if err != nil {
		return "", err
	}
	ephemeralPublic := ephemeral.PublicKey().Bytes()
	salt := append(append([]byte{}, ephemeralPublic...), recipientRaw...)
	key, err := hkdf.Key(sha256.New, shared, salt, sealInfo, 32)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	blob := append([]byte("TBOX1"), ephemeralPublic...)
	blob = append(blob, nonce...)
	blob = gcm.Seal(blob, nonce, secret, []byte("tebib-cert-import:"+projectID))
	return base64.StdEncoding.EncodeToString(blob), nil
}
