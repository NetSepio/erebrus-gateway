package paseto

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/config/envconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/models/claims"
	"github.com/vk-rv/pvx"
	"gorm.io/gorm"

	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"
	"github.com/TheLazarusNetwork/go-helpers/httpo"

	"github.com/gin-gonic/gin"
)

var CTX_WALLET_ADDRES = "WALLET_ADDRESS"
var CTX_USER_ID = "USER_ID"

var (
	ErrAuthHeaderMissing = errors.New("authorization header is required")
)

func PASETO(authOptional bool) func(*gin.Context) {
	return func(c *gin.Context) {
		var headers GenericAuthHeaders
		err := c.BindHeader(&headers)
		if err != nil {
			err = fmt.Errorf("failed to bind header, %s", err)
			logValidationFailed(headers.Authorization, err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		if headers.Authorization == "" {
			if authOptional {
				c.Next()
				return
			}
			logValidationFailed(headers.Authorization, ErrAuthHeaderMissing)
			httpo.NewErrorResponse(http.StatusBadRequest, ErrAuthHeaderMissing.Error()).SendD(c)
			c.Abort()
			return
		} else if !strings.HasPrefix(headers.Authorization, "Bearer ") {
			err := errors.New("authorization header must have Bearer prefix")
			logValidationFailed(headers.Authorization, err)
			httpo.NewErrorResponse(http.StatusBadRequest, err.Error()).SendD(c)
			c.Abort()
			return
		}

		pasetoToken := strings.TrimPrefix(headers.Authorization, "Bearer ")
		ppv4 := pvx.NewPV4Public()
		k, err := hex.DecodeString(envconfig.EnvVars.PASETO_PRIVATE_KEY[2:])
		if err != nil {
			err = fmt.Errorf("failed to decode priv key, %s", err)
			logValidationFailed(headers.Authorization, err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		pubKey := ed25519.PrivateKey(k).Public().(ed25519.PublicKey)
		asymPK := pvx.NewAsymmetricPublicKey(pubKey, pvx.Version4)
		var cc claims.CustomClaims
		err = ppv4.
			Verify(pasetoToken, asymPK).
			ScanClaims(&cc)
		if err != nil {
			var validationErr *pvx.ValidationError
			if errors.As(err, &validationErr) {
				if validationErr.HasExpiredErr() {
					err = fmt.Errorf("failed to scan claims for paseto token, %s", err)
					logValidationFailed(headers.Authorization, err)
					httpo.NewErrorResponse(httpo.TokenExpired, "token expired").Send(c, http.StatusUnauthorized)
					c.Abort()
					return
				}

			}
			err = fmt.Errorf("failed to scan claims for paseto token, %s", err)
			logValidationFailed(headers.Authorization, err)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		} else {
			if err := cc.Valid(); err != nil {
				logValidationFailed(headers.Authorization, err)
				if err.Error() == gorm.ErrRecordNotFound.Error() {
					c.AbortWithStatus(http.StatusUnauthorized)
				} else {
					err = fmt.Errorf("failed to validate claim, %s", err)
					logwrapper.Log.Error(err)
					c.AbortWithStatus(http.StatusInternalServerError)
				}
			} else {
				db := dbconfig.GetDb()
				var userFetch models.User
				err := db.Model(&models.User{}).Where("user_id = ?", strings.ToLower(cc.UserId)).First(&userFetch).Error
				if err != nil {
					err = fmt.Errorf("failed to get wallet address, %s", err)
					logwrapper.Log.Error(err)
					c.AbortWithStatus(http.StatusInternalServerError)
					return
				}
				if userFetch.WalletAddress != nil {
					c.Set(CTX_WALLET_ADDRES, *userFetch.WalletAddress)
				}
				c.Set(CTX_USER_ID, userFetch.UserId)
				c.Next()
			}
		}
	}
}

func logValidationFailed(token string, err error) {
	logwrapper.Warnf("validation failed with token %v and error: %v", token, err)
}
