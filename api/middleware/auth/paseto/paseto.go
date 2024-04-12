package paseto

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"
	"github.com/TheLazarusNetwork/go-helpers/httpo"
	"github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"
)

var CTX_WALLET_ADDRESS = "WALLET_ADDRESS"
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
		//auth req to gateway
		contractReq, err := http.NewRequest(http.MethodGet, os.Getenv("GATEWAY_URL")+"/api/v1.0/authenticate", nil)
		if err != nil {
			logrus.Errorf("failed to send request: %s", err)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		contractReq.Header.Set("Authorization", "Bearer "+pasetoToken)
		client := &http.Client{}
		resp, err := client.Do(contractReq)
		if err != nil {
			logrus.Errorf("failed to send request: %s", err)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if resp.StatusCode != 200 {
			logrus.Errorf("Error in response: %s", err)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		defer resp.Body.Close()
		var responseBody AuthenticateTokenResponse
		err = json.NewDecoder(resp.Body).Decode(&responseBody)
		if err != nil {
			fmt.Printf("Failed to decode response body: %s\n", err)
			return
		} else {
			if responseBody.Payload.WalletAddress != "" {
				c.Set(CTX_WALLET_ADDRESS, responseBody.Payload.WalletAddress)
			}
			c.Set(CTX_USER_ID, responseBody.Payload.UserID)
			c.Next()
		}
	}
}

func logValidationFailed(token string, err error) {
	logwrapper.Warnf("validation failed with token %v and error: %v", token, err)
}
