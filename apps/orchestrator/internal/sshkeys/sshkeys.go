package sshkeys

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

type KeyPair struct {
	PrivatePEM  string
	PublicKey   string
	Fingerprint string
}

func Generate(comment string) (KeyPair, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return KeyPair{}, err
	}
	pubKey, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		return KeyPair{}, err
	}
	pubAuthorized := string(ssh.MarshalAuthorizedKey(pubKey))
	if comment != "" {
		pubAuthorized = fmt.Sprintf("%s %s\n", string(pubAuthorized[:len(pubAuthorized)-1]), comment)
	}
	fingerprint := ssh.FingerprintSHA256(pubKey)
	block := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if block == nil {
		return KeyPair{}, fmt.Errorf("failed to encode private key")
	}
	return KeyPair{PrivatePEM: string(block), PublicKey: pubAuthorized, Fingerprint: fingerprint}, nil
}
