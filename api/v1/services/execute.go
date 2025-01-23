package caddyservices

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/NetSepio/erebrus-gateway/api/middleware/auth/paseto"
	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/httpo"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
)

func ApplyRoutes(r *gin.RouterGroup) {
	api := r.Group("")
	{
		r.Use(paseto.PASETO(true))
		api.POST("/add/service", CallAddService)
		api.GET("/all/services", CallGetAllServices)
		api.GET("/service/:name", CallGetService)
		api.DELETE("/service/:name", CallDeleteService)
	}
}

func baseURL(c *gin.Context) (string, int, error) {
	walletAddress := c.GetString(paseto.CTX_WALLET_ADDRESS)
	if len(walletAddress) == 0 {
		return "", http.StatusNotFound, errors.New("invalid wallet address : walletAddress is empty")
	}
	db := dbconfig.GetDb()
	var node models.Node

	err := db.Where("wallet_address = ?", walletAddress).First(&node).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return "", http.StatusNotFound, errors.New("failed to get wallet details : node not found")
		} else {
			return "", http.StatusInternalServerError, fmt.Errorf("error fetching node for wallet address %s: %v", walletAddress, err)

		}
	}
	fmt.Println()
	fmt.Println("node host : ", node.Host+"/api/v1.0/caddy")
	fmt.Println()

	return node.Host + "/api/v1.0/caddy", http.StatusOK, nil
}

// CallAddService calls the `addServices` API
func CallAddService(c *gin.Context) {
	var payload RequestPayload
	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	baseURL, statusCode, err := baseURL(c)
	if err != nil && len(baseURL) == 0 {
		logwrapper.Errorf("failed to get node details by wallet address %s: %v\n", err, err)
		httpo.NewErrorResponse(statusCode, fmt.Sprintf("failed to get node details by wallet address: %v\n", err)).SendD(c)
		return
	}

	if response, err := AddServiceInErebrusNode(RequestData{Name: payload.Name, IpAddress: payload.IPAddress, Port: payload.Port}, baseURL); err != nil {
		if response.Message.Name != "" {
			logwrapper.Errorf("error adding service: %v\n", response.Message.Name)
			httpo.NewErrorResponse(500, "error adding service , "+" node status code = "+fmt.Sprintf("%d", response.Status)+", api response message : "+response.Message.Name).SendD(c)
		} else {
			logwrapper.Errorf("Failed to add the services", err.Error())
			httpo.NewErrorResponse(500, "Failed to add the services").SendD(c)
		}
	} else {
		httpo.NewSuccessResponseP(200, "Service added Successfully", response).SendD(c)
	}

}

// CallGetServices calls the `getServices` API
func CallGetAllServices(c *gin.Context) {

	baseURL, statusCode, err := baseURL(c)
	if err != nil && len(baseURL) == 0 {
		logwrapper.Errorf("failed to get node details by wallet address %s: %v\n", err, err)
		httpo.NewErrorResponse(statusCode, fmt.Sprintf("failed to get node details by wallet address: %v\n", err)).SendD(c)
		return
	}

	response, err := FetchServices(baseURL)

	if err != nil {
		logwrapper.Errorf("Failed to get the services %v", err.Error())
		httpo.NewErrorResponse(500, "Failed to get the services").SendD(c)
		return
	} else {
		httpo.NewSuccessResponseP(200, "Service get successfully", response).SendD(c)

	}

}

// CallGetService calls the `getService` API for a specific service
func CallGetService(c *gin.Context) {
	name := c.Param("name")
	if len(name) == 0 {
		httpo.NewErrorResponse(400, "name is required").SendD(c)
		return
	}
	baseURL, statusCode, err := baseURL(c)
	if err != nil && len(baseURL) == 0 {
		logwrapper.Errorf("failed to get node details by wallet address %s: %v\n", err, err)
		httpo.NewErrorResponse(statusCode, fmt.Sprintf("failed to get node details by wallet address: %v\n", err)).SendD(c)
		return
	}
	url := fmt.Sprintf("%s/%s", baseURL, name)

	response, err := FetchServiceDetails(url)

	if err != nil {
		logwrapper.Errorf("Failed to get the services %v", err.Error())
		httpo.NewErrorResponse(500, "Failed to get the services").SendD(c)
		return
	} else {
		httpo.NewSuccessResponseP(200, "Service get successfully", response).SendD(c)
	}

}

func CallDeleteService(c *gin.Context) {
	name := c.Param("name")
	if len(name) == 0 {
		httpo.NewErrorResponse(400, "name is required").SendD(c)
		return
	}

	baseURL, statusCode, err := baseURL(c)
	if err != nil && len(baseURL) == 0 {
		logwrapper.Errorf("failed to get node details by wallet address %s: %v\n", err, err)
		httpo.NewErrorResponse(statusCode, fmt.Sprintf("failed to get node details by wallet address : %v\n", err)).SendD(c)
		return
	}
	url := fmt.Sprintf("%s/%s", baseURL, name)

	response, err := DeleteService(url)

	if err != nil {
		logwrapper.Errorf("Failed to get the services %v", err.Error())
		httpo.NewErrorResponse(500, "Failed to get the services").SendD(c)
		return
	} else {
		httpo.NewSuccessResponseP(200, "Service get successfully", response).SendD(c)
	}

}
