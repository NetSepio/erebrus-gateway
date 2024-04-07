package auth

import (
	"crypto/ed25519"

	"github.com/vk-rv/pvx"
)

func GenerateToken(claims pvx.Claims, privateKey ed25519.PrivateKey) (string, error) {
	asymSK := pvx.NewAsymmetricSecretKey(privateKey, pvx.Version4)
	ppv4 := pvx.NewPV4Public()
	tokenString, err := ppv4.Sign(asymSK, claims)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}
