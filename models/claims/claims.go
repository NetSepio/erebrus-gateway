package claims

import (
	"fmt"
	"time"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/config/envconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/vk-rv/pvx"
)

type CustomClaims struct {
	WalletAddress *string `json:"walletAddress,omitempty"`
	UserId        string  `json:"userId"`
	SignedBy      string  `json:"signedBy"`
	Email         *string `json:"email,omitempty"`
	pvx.RegisteredClaims
}

func (c CustomClaims) Valid() error {
	db := dbconfig.GetDb()
	if err := c.RegisteredClaims.Valid(); err != nil {
		return err
	}
	fmt.Printf("c.UserId: %s\n", c.UserId)
	err := db.Model(&models.User{}).Where("user_id = ?", c.UserId).First(&models.User{}).Error
	if err != nil {
		return err
	}
	return nil
}

func NewWithWallet(userId string, walletAddr *string) CustomClaims {
	pasetoExpirationInHours := envconfig.EnvVars.PASETO_EXPIRATION
	expiration := time.Now().Add(pasetoExpirationInHours)
	signedBy := envconfig.EnvVars.PASETO_SIGNED_BY
	return CustomClaims{
		walletAddr,
		userId,
		signedBy,
		nil,
		pvx.RegisteredClaims{
			Expiration: &expiration,
		},
	}
}

func NewWithEmail(userId string, email *string) CustomClaims {
	pasetoExpirationInHours := envconfig.EnvVars.PASETO_EXPIRATION
	expiration := time.Now().Add(pasetoExpirationInHours)
	signedBy := envconfig.EnvVars.PASETO_SIGNED_BY
	return CustomClaims{
		nil,
		userId,
		signedBy,
		email,
		pvx.RegisteredClaims{
			Expiration: &expiration,
		},
	}
}
