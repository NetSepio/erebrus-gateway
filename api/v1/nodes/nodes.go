package nodes

import (
	"encoding/json"
	"fmt"
	"net/http"

	nodeactivity "github.com/NetSepio/erebrus-gateway/api/v1/nodes/nodeActivity"
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
		g.GET("/:status", FetchAllNodesByStatus)
		g.GET("/status_wallet_address/:status/:wallet_address", FetchAllNodesByStatusAndWalletAddress)
		g.GET("/nodes_details_by_wallet_adddress_and_chain", HandlerGetNodesByChainAndWallet())

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
		if len(i.SystemInfo) > 0 {
			err := json.Unmarshal([]byte(i.SystemInfo), &osInfo)
			if err != nil {
				logwrapper.Errorf("failed to get nodes from DB OSInfo: %s", err)
				// httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
			}
		}
		// Unmarshal IpInfo into IPInfo struct
		var ipGeoAddress models.IpGeoAddress
		if len(i.IpGeoData) > 0 {
			err := json.Unmarshal([]byte(i.IpGeoData), &ipGeoAddress)
			if err != nil {
				logwrapper.Errorf("failed to get nodes from DB IpGeoAddress: %s", err)
				// httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
			}
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
		response.Chain = i.Chain
		response.WalletAddressSui = i.WalletAddress
		response.WalletAddressSolana = i.WalletAddress
		response.IpInfoIP = ipGeoAddress.IpInfoIP
		response.IpInfoCity = ipGeoAddress.IpInfoCity
		response.IpInfoCountry = ipGeoAddress.IpInfoCountry
		response.IpInfoLocation = ipGeoAddress.IpInfoLocation
		response.IpInfoOrg = ipGeoAddress.IpInfoOrg
		response.IpInfoPostal = ipGeoAddress.IpInfoPostal
		response.IpInfoTimezone = ipGeoAddress.IpInfoTimezone
		// Round TotalActiveDuration and TodayActiveDuration to two decimal places

		response.TotalActiveDuration, response.TodayActiveDuration = nodeactivity.CalculateTotalAndTodayActiveDuration(i.PeerId)
		response.UptimeUnit = "hrs"

		// response.TotalActiveDuration = math.Round(i.TotalActiveDuration*100) / 100
		// response.TodayActiveDuration = math.Round(i.TodayActiveDuration*100) / 100

		responses = append(responses, response)
	}

	httpo.NewSuccessResponseP(200, "Nodes fetched succesfully", responses).SendD(c)
}

func FetchAllNodesByStatus(c *gin.Context) {
	status := c.Param("status") // active , inactive
	db := dbconfig.GetDb()
	var nodes *[]models.Node
	// var node *models.Node
	if err := db.Where("status = ?", status).Find(&nodes).Error; err != nil {
		logwrapper.Errorf("failed to get nodes from DB: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}

	// Unmarshal SystemInfo into OSInfo struct

	var responses []models.NodeResponse
	var response models.NodeResponse

	for _, i := range *nodes {
		var osInfo models.OSInfo
		if len(i.SystemInfo) > 0 {
			err := json.Unmarshal([]byte(i.SystemInfo), &osInfo)
			if err != nil {
				logwrapper.Errorf("failed to get nodes from DB OSInfo: %s", err)
				// httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
			}
		}
		// Unmarshal IpInfo into IPInfo struct
		var ipGeoAddress models.IpGeoAddress
		if len(i.IpGeoData) > 0 {
			err := json.Unmarshal([]byte(i.IpGeoData), &ipGeoAddress)
			if err != nil {
				logwrapper.Errorf("failed to get nodes from DB IpGeoAddress: %s", err)
				// httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
			}
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
		response.Chain = i.Chain
		response.WalletAddressSui = i.WalletAddress
		response.WalletAddressSolana = i.WalletAddress
		response.IpInfoIP = ipGeoAddress.IpInfoIP
		response.IpInfoCity = ipGeoAddress.IpInfoCity
		response.IpInfoCountry = ipGeoAddress.IpInfoCountry
		response.IpInfoLocation = ipGeoAddress.IpInfoLocation
		response.IpInfoOrg = ipGeoAddress.IpInfoOrg
		response.IpInfoPostal = ipGeoAddress.IpInfoPostal
		response.IpInfoTimezone = ipGeoAddress.IpInfoTimezone
		// Round TotalActiveDuration and TodayActiveDuration to two decimal places
		// response.TotalActiveDuration = math.Round(i.TotalActiveDuration*100) / 100
		// response.TodayActiveDuration = math.Round(i.TodayActiveDuration*100) / 100

		response.TotalActiveDuration, response.TodayActiveDuration = nodeactivity.CalculateTotalAndTodayActiveDuration(i.PeerId)

		responses = append(responses, response)
	}

	httpo.NewSuccessResponseP(200, "Nodes fetched succesfully", responses).SendD(c)
}

func FetchAllNodesByStatusAndWalletAddress(c *gin.Context) {
	status := c.Param("status")                // active , inactive
	walletAddress := c.Param("wallet_address") // active , inactive

	fmt.Printf("status : %v , wallet address : %v ", status, walletAddress)

	if len(walletAddress) == 0 || walletAddress == "wallet_address" {
		logwrapper.Errorf("please pass the wallet address : ", walletAddress)
		httpo.NewErrorResponse(http.StatusBadRequest, "Please provide the wallet_address").SendD(c)
		return
	}

	db := dbconfig.GetDb()
	var nodes *[]models.Node
	// var node *models.Node
	if len(status) == 0 || status == ":status" {
		if err := db.Where("wallet_address = ?", walletAddress).Find(&nodes).Error; err != nil {
			logwrapper.Errorf("failed to get nodes from DB: %s", err)
			httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
			return
		}
	} else {
		if err := db.Where("wallet_address = ? AND status = ? ", walletAddress, status).Find(&nodes).Error; err != nil {
			logwrapper.Errorf("failed to get nodes from DB: %s", err)
			httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
			return
		}
	}

	// Unmarshal SystemInfo into OSInfo struct

	var responses []models.NodeResponse
	var response models.NodeResponse

	for _, i := range *nodes {
		var osInfo models.OSInfo
		if len(i.SystemInfo) > 0 {
			err := json.Unmarshal([]byte(i.SystemInfo), &osInfo)
			if err != nil {
				logwrapper.Errorf("failed to get nodes from DB OSInfo: %s", err)
				// httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
			}
		}
		// Unmarshal IpInfo into IPInfo struct
		var ipGeoAddress models.IpGeoAddress
		if len(i.IpGeoData) > 0 {
			err := json.Unmarshal([]byte(i.IpGeoData), &ipGeoAddress)
			if err != nil {
				logwrapper.Errorf("failed to get nodes from DB IpGeoAddress: %s", err)
				// httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
			}
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
		response.Chain = i.Chain
		response.WalletAddressSui = i.WalletAddress
		response.WalletAddressSolana = i.WalletAddress
		response.IpInfoIP = ipGeoAddress.IpInfoIP
		response.IpInfoCity = ipGeoAddress.IpInfoCity
		response.IpInfoCountry = ipGeoAddress.IpInfoCountry
		response.IpInfoLocation = ipGeoAddress.IpInfoLocation
		response.IpInfoOrg = ipGeoAddress.IpInfoOrg
		response.IpInfoPostal = ipGeoAddress.IpInfoPostal
		response.IpInfoTimezone = ipGeoAddress.IpInfoTimezone
		// Round TotalActiveDuration and TodayActiveDuration to two decimal places
		// response.TotalActiveDuration = math.Round(i.TotalActiveDuration*100) / 100
		// response.TodayActiveDuration = math.Round(i.TodayActiveDuration*100) / 100

		response.TotalActiveDuration, response.TodayActiveDuration = nodeactivity.CalculateTotalAndTodayActiveDuration(i.PeerId)

		responses = append(responses, response)
	}

	httpo.NewSuccessResponseP(200, "Nodes fetched succesfully", responses).SendD(c)
}

// NodesHandler handles the API request to fetch nodes
func HandlerGetNodesByChainAndWallet() gin.HandlerFunc {
	return func(c *gin.Context) {
		chain := c.Query("chain")
		walletAddress := c.Query("wallet_address")

		if chain == "" || walletAddress == "" {
			logwrapper.Errorf("chain and wallet_address are required { HandlerGetNodesByChainAndWallet }")

			return
		}
		db := dbconfig.GetDb()
		var nodes *[]models.Node

		err := db.Where("chain = ? AND wallet_address = ?", chain, walletAddress).Find(&nodes).Error

		// nodes, err := GetNodesByChainAndWallet(db, chain, walletAddress)
		if err != nil {
			logwrapper.Errorf("failed to get nodes from DB { HandlerGetNodesByChainAndWallet }: %s", err)
			return
		}

		var responses []models.NodeResponse
		var response models.NodeResponse

		for _, i := range *nodes {
			var osInfo models.OSInfo
			if len(i.SystemInfo) > 0 {
				err := json.Unmarshal([]byte(i.SystemInfo), &osInfo)
				if err != nil {
					logwrapper.Errorf("failed to get nodes from DB OSInfo: %s", err)
					// httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
				}
			}
			// Unmarshal IpInfo into IPInfo struct
			var ipGeoAddress models.IpGeoAddress
			if len(i.IpGeoData) > 0 {
				err := json.Unmarshal([]byte(i.IpGeoData), &ipGeoAddress)
				if err != nil {
					logwrapper.Errorf("failed to get nodes from DB IpGeoAddress: %s", err)
					// httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
				}
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
			response.Chain = i.Chain
			response.WalletAddressSui = i.WalletAddress
			response.WalletAddressSolana = i.WalletAddress
			response.IpInfoIP = ipGeoAddress.IpInfoIP
			response.IpInfoCity = ipGeoAddress.IpInfoCity
			response.IpInfoCountry = ipGeoAddress.IpInfoCountry
			response.IpInfoLocation = ipGeoAddress.IpInfoLocation
			response.IpInfoOrg = ipGeoAddress.IpInfoOrg
			response.IpInfoPostal = ipGeoAddress.IpInfoPostal
			response.IpInfoTimezone = ipGeoAddress.IpInfoTimezone
			// Round TotalActiveDuration and TodayActiveDuration to two decimal places
			// response.TotalActiveDuration = math.Round(i.TotalActiveDuration*100) / 100
			// response.TodayActiveDuration = math.Round(i.TodayActiveDuration*100) / 100

			response.TotalActiveDuration, response.TodayActiveDuration = nodeactivity.CalculateTotalAndTodayActiveDuration(i.PeerId)

			responses = append(responses, response)
		}

		httpo.NewSuccessResponseP(200, "Nodes fetched succesfully", responses).SendD(c)

	}
}
