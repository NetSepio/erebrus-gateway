package nodes

import (
	"encoding/json"
	"net/http"

	"github.com/NetSepio/erebrus-gateway/config/dbconfig"
	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"
	"github.com/TheLazarusNetwork/go-helpers/httpo"
	"github.com/gin-gonic/gin"
)

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/nodes")
	{
		g.GET("/all", FetchAllNodes)
	}
}

func FetchAllNodes(c *gin.Context) {
	db := dbconfig.GetDb()
	var nodes *[]models.Node
	// var node *models.Node
	if err := db.Find(&nodes).Error; err != nil {
		logwrapper.Errorf("failed to get nodes from DB: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	// Unmarshal SystemInfo into OSInfo struct

	var responses []models.NodeResponse
	var response models.NodeResponse

	for _, i := range *nodes {
		var osInfo models.OSInfo
		err := json.Unmarshal([]byte(i.SystemInfo), &osInfo)
		if err != nil {
			logwrapper.Errorf("failed to get nodes from DB: %s", err)
			httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		}

		// Unmarshal IpInfo into IPInfo struct
		var ipGeoAddress models.IpGeoAddress
		err = json.Unmarshal([]byte(i.IpGeoData), &ipGeoAddress)
		if err != nil {
			logwrapper.Errorf("failed to get nodes from DB: %s", err)
			httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		}
		response.Id = i.PeerId
		response.Name = i.Name
		response.HttpPort = i.HttpPort
		response.Domain = i.Host
		response.NodeName = i.Name
		response.Address = i.PeerAddress
		response.Region = i.Region
		response.Status = i.Status
		response.DownloadSpeed = i.DownloadSpeed
		response.UploadSpeed = i.UploadSpeed
		response.StartTimeStamp = i.RegistrationTime
		response.LastPingedTimeStamp = i.LastPing
		response.WalletAddressSui = i.WalletAddress
		response.WalletAddressSolana = i.WalletAddress
		response.IpInfoIP = ipGeoAddress.IpInfoIP
		response.IpInfoCity = ipGeoAddress.IpInfoCity
		response.IpInfoCountry = ipGeoAddress.IpInfoCountry
		response.IpInfoLocation = ipGeoAddress.IpInfoLocation
		response.IpInfoOrg = ipGeoAddress.IpInfoOrg
		response.IpInfoPostal = ipGeoAddress.IpInfoPostal
		response.IpInfoTimezone = ipGeoAddress.IpInfoTimezone

		responses = append(responses, response)
	}

	httpo.NewSuccessResponseP(200, "Nodes fetched succesfully", responses).SendD(c)

}
