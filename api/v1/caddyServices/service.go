package caddyservices

import (
	"fmt"
	"net/http"

	"github.com/NetSepio/erebrus-gateway/api/middleware/auth/paseto"
	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"
	"github.com/TheLazarusNetwork/go-helpers/httpo"
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

func baseURL(c *gin.Context) string {
	walletAddress := c.GetString(paseto.CTX_WALLET_ADDRESS)
	db := dbconfig.GetDb()
	var node models.Node

	err := db.Where("wallet_address = ?", walletAddress).First(&node).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			logwrapper.Errorf("node not found for wallet address %s: %v\n", walletAddress, err)
			httpo.NewErrorResponse(404, "node not found").SendD(c)
			return ""
		} else {
			logwrapper.Errorf("error fetching node for wallet address %s: %v\n", walletAddress, err)
			httpo.NewErrorResponse(500, "error fetching node").SendD(c)
			return ""
		}
	}
	fmt.Println()
	fmt.Println("node host : ", node.Host+"/api/v1.0/caddy")
	fmt.Println()

	return node.Host + "/api/v1.0/caddy"
}

// CallAddService calls the `addServices` API
func CallAddService(c *gin.Context) {
	var payload RequestPayload
	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if response, err := AddServiceInErebrusNode(RequestData{Name: payload.Name, IpAddress: payload.IPAddress, Port: payload.Port}, baseURL(c)); err != nil {
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

	response, err := FetchServices(baseURL(c))

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
	url := fmt.Sprintf("%s/%s", baseURL(c), name)

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
	url := fmt.Sprintf("%s/%s", baseURL(c), name)

	response, err := DeleteService(url)

	if err != nil {
		logwrapper.Errorf("Failed to get the services %v", err.Error())
		httpo.NewErrorResponse(500, "Failed to get the services").SendD(c)
		return
	} else {
		httpo.NewSuccessResponseP(200, "Service get successfully", response).SendD(c)
	}

}
