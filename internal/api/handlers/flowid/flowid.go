package flowid

import (
	"net/http"

	"github.com/NetSepio/gateway/internal/api/middleware/auth/paseto"
	"github.com/NetSepio/gateway/models"
	"github.com/NetSepio/gateway/utils/httpo"
	"github.com/NetSepio/gateway/utils/load"
	"github.com/NetSepio/gateway/utils/pkg/flowid"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/gin-gonic/gin"
)

// ApplyRoutes applies router to gin Router
func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/flowid")
	{
		g.Use(paseto.PASETO(true))
		g.GET("", GetFlowId)
		g.GET("/cyreneai", GetFlowIdForCyreneAi)
	}
}

func GetFlowId(c *gin.Context) {
	userId := c.GetString(paseto.CTX_USER_ID)
	walletAddress := c.Query("walletAddress")
	chain_symbol := c.Query("chain")
	var verify bool

	if walletAddress == "" {
		httpo.NewErrorResponse(http.StatusBadRequest, "Wallet address (walletAddress) is required").SendD(c)
		return
	}
	if chain_symbol != "sol" {
		_, err := hexutil.Decode(walletAddress)
		if err != nil {
			httpo.NewErrorResponse(http.StatusBadRequest, "Please pass the valid chain name").SendD(c)
			return
		}
	}
	var flowId string
	if chain_symbol == "sol" {
		var err error

		flowId, err, verify = flowid.GenerateFlowIdSol(walletAddress, models.AUTH, "", userId, "")
		if err != nil {
			load.Logger.Error(err.Error())
			httpo.NewErrorResponse(http.StatusInternalServerError, "Unexpected error occured").SendD(c)
			return
		}
		c.Set(paseto.CTX_VERIFIED, verify)
	} else {
		var err error
		flowId, err, verify = flowid.GenerateFlowId(walletAddress, models.AUTH, "", userId, "")
		if err != nil {
			load.Logger.Error(err.Error())
			httpo.NewErrorResponse(http.StatusInternalServerError, "Unexpected error occured").SendD(c)
			return
		}
		c.Set(paseto.CTX_VERIFIED, verify)
	}
	userAuthEULA := load.Cfg.AUTH_EULA

	payload := GetFlowIdPayload{
		FlowId: flowId,
		Eula:   userAuthEULA,
	}
	httpo.NewSuccessResponseP(200, "Flowid successfully generated", payload).SendD(c)
}

func GetFlowIdForCyreneAi(c *gin.Context) {
	userId := c.GetString(paseto.CTX_USER_ID)
	walletAddress := c.Query("walletAddress")
	chain_symbol := c.Query("chain")
	var verify bool

	if walletAddress == "" {
		httpo.NewErrorResponse(http.StatusBadRequest, "Wallet address (walletAddress) is required").SendD(c)
		return
	}
	if chain_symbol != "sol" {
		_, err := hexutil.Decode(walletAddress)
		if err != nil {
			httpo.NewErrorResponse(http.StatusBadRequest, "Please pass the valid chain name").SendD(c)
			return
		}
	}
	var flowId string
	if chain_symbol == "sol" {
		var err error

		flowId, err, verify = flowid.GenerateFlowIdSol(walletAddress, models.AUTH, "", userId, "")
		if err != nil {
			load.Logger.Error(err.Error())
			httpo.NewErrorResponse(http.StatusInternalServerError, "Unexpected error occured").SendD(c)
			return
		}
		c.Set(paseto.CTX_VERIFIED, verify)
	} else {
		var err error
		flowId, err, verify = flowid.GenerateFlowId(walletAddress, models.AUTH, "", userId, "")
		if err != nil {
			load.Logger.Error(err.Error())
			httpo.NewErrorResponse(http.StatusInternalServerError, "Unexpected error occured").SendD(c)
			return
		}
		c.Set(paseto.CTX_VERIFIED, verify)
	}
	userAuthEULA := "I agree to CyreneAi Terms & Conditions specified at https://cyreneai.com/terms."
	payload := GetFlowIdPayload{
		FlowId: flowId,
		Eula:   userAuthEULA,
	}
	httpo.NewSuccessResponseP(200, "Flowid successfully generated", payload).SendD(c)
}
