package authenticate

import (
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/config/envconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/models/claims"
	"github.com/TheLazarusNetwork/go-helpers/httpo"

	"github.com/NetSepio/erebrus-gateway/util/pkg/auth"
	"github.com/NetSepio/erebrus-gateway/util/pkg/cryptosign"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"

	// "github.com/TheLazarusNetwork/go-helpers/httpo"

	"github.com/gin-gonic/gin"
)

// ApplyRoutes applies router to gin Router
func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/authenticate")
	{
		g.POST("", authenticate)
	}
}

func authenticate(c *gin.Context) {
	db := dbconfig.GetDb()
	//TODO remove flow id if 200
	var req AuthenticateRequest
	err := c.BindJSON(&req)
	if err != nil {
		httpo.NewErrorResponse(http.StatusBadRequest, fmt.Sprintf("payload is invalid: %s", err)).SendD(c)
		return
	}

	//Get flowid type
	var flowIdData models.FlowId
	err = db.Model(&models.FlowId{}).Where("flow_id = ?", req.FlowId).First(&flowIdData).Error
	if err != nil {
		logwrapper.Errorf("failed to get flowId, error %v", err)
		httpo.NewErrorResponse(http.StatusNotFound, "flow id not found").SendD(c)
		return
	}

	if flowIdData.FlowIdType != models.AUTH {
		httpo.NewErrorResponse(http.StatusBadRequest, "flow id not created for auth").SendD(c)
		return
	}

	if err != nil {
		logwrapper.Error(err)
		httpo.NewErrorResponse(500, "Unexpected error occured").SendD(c)
		return
	}
	userAuthEULA := envconfig.EnvVars.AUTH_EULA
	message := fmt.Sprintf("APTOS\nmessage: %v\nnonce: %v", userAuthEULA, req.FlowId)

	userId, walletAddr, isCorrect, err := cryptosign.CheckSign(req.Signature, req.FlowId, message, req.PubKey)

	if err == cryptosign.ErrFlowIdNotFound {
		httpo.NewErrorResponse(http.StatusNotFound, "Flow Id not found")
		return
	}

	if err != nil {
		logwrapper.Errorf("failed to CheckSignature, error %v", err.Error())
		httpo.NewErrorResponse(http.StatusInternalServerError, "Unexpected error occured").SendD(c)
		return
	}
	if isCorrect {
		// update wallet address for that user_id
		err = db.Model(&models.User{}).Where("user_id = ?", userId).Update("wallet_address", walletAddr).Error
		if err != nil {
			httpo.NewErrorResponse(http.StatusInternalServerError, "Unexpected error occured").SendD(c)
			logwrapper.Errorf("failed to update wallet address, error %v", err.Error())
			return
		}

		customClaims := claims.NewWithWallet(userId, &walletAddr)
		pvKey, err := hex.DecodeString(envconfig.EnvVars.PASETO_PRIVATE_KEY[2:])
		if err != nil {
			httpo.NewErrorResponse(http.StatusInternalServerError, "Unexpected error occured").SendD(c)
			logwrapper.Errorf("failed to generate token, error %v", err.Error())
			return
		}
		pasetoToken, err := auth.GenerateToken(customClaims, pvKey)
		if err != nil {
			httpo.NewErrorResponse(http.StatusInternalServerError, "Unexpected error occured").SendD(c)
			logwrapper.Errorf("failed to generate token, error %v", err.Error())
			return
		}
		err = db.Where("flow_id = ?", req.FlowId).Delete(&models.FlowId{}).Error
		if err != nil {
			httpo.NewErrorResponse(http.StatusInternalServerError, "Unexpected error occured").SendD(c)
			logwrapper.Errorf("failed to delete flowId, error %v", err.Error())
			return
		}
		payload := AuthenticatePayload{
			Token:  pasetoToken,
			UserId: userId,
		}
		httpo.NewSuccessResponseP(200, "Token generated successfully", payload).SendD(c)
	} else {
		httpo.NewErrorResponse(http.StatusForbidden, "Wallet Address is not correct").SendD(c)
		return
	}
}

// create api handler which will take
