package pkey

import (
	"fmt"
	"os"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// GenerateIdentity writes a new random private key to the given path.
func GenerateIdentity(path string) (crypto.PrivKey, error) {
	privk, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 0)
	if err != nil {
		return nil, err
	}

	bytes, err := crypto.MarshalPrivateKey(privk)
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(path, bytes, 0400)

	return privk, err
}

// ReadIdentity reads a private key from the given path.
func ReadIdentity(path string) (crypto.PrivKey, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return crypto.UnmarshalPrivateKey(bytes)
}

// LoadIdentity reads a private key from the given path and, if it does not
// exist, generates a new one.
func LoadIdentity(path string) (crypto.PrivKey, error) {
	if _, err := os.Stat(path); err == nil {
		return ReadIdentity(path)
	} else if os.IsNotExist(err) {
		fmt.Printf("Generating peer identity in %s\n", path)
		return GenerateIdentity(path)
	} else {
		return nil, err
	}
}
