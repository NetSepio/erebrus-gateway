package nodes

import (
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
		g.GET("/active", FetchAllActiveNodes)

	}
}

func FetchAllNodes(c *gin.Context) {
	db := dbconfig.GetDb()
	var nodes *[]models.Node
	if err := db.Find(&nodes).Error; err != nil {
		logwrapper.Errorf("failed to get nodes from DB: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	httpo.NewSuccessResponseP(200, "Nodes fetched succesfully", nodes).SendD(c)

}

func FetchAllActiveNodes(c *gin.Context) {
	db := dbconfig.GetDb()
	var nodes *[]models.Node
	if err := db.Select(&nodes).Where("status = ? ", "active").Error; err != nil {
		logwrapper.Errorf("failed to get active nodes from DB: %s", err)
		httpo.NewErrorResponse(http.StatusInternalServerError, err.Error()).SendD(c)
		return
	}
	httpo.NewSuccessResponseP(200, "Active Nodes fetched succesfully", nodes).SendD(c)

}
