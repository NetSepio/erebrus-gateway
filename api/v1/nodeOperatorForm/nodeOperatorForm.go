package nodeOperatorForm

import (
	"net/http"
	"regexp"

	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/NetSepio/erebrus-gateway/util/pkg/logwrapper"
	"github.com/NetSepio/gateway/config/dbconfig"
	"github.com/TheLazarusNetwork/go-helpers/httpo"
	"github.com/gin-gonic/gin"
)

func ApplyRoutes(r *gin.RouterGroup) {
	g := r.Group("/operator")
	{
		g.POST("/new", Nodeoperatorform)
		g.GET("", GetOperator)
	}
}

func isValidEmail(email string) bool {
	const emailRegex = `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	re := regexp.MustCompile(emailRegex)
	return re.MatchString(email)
}

func Nodeoperatorform(c *gin.Context) {
	db := dbconfig.GetDb()
	if db.Error != nil {
		logwrapper.Errorf("failed to get DB: %s", db.Error)
		httpo.NewErrorResponse(http.StatusInternalServerError, db.Error.Error()).SendD(c)
		return
	}

	var formData models.FormData
	if err := c.ShouldBindJSON(&formData); err != nil {
		logwrapper.Errorf("Bad Request: %s", err)
		httpo.NewErrorResponse(http.StatusBadRequest, err.Error()).SendD(c)
		return
	}

	// Validate email format
	if !isValidEmail(formData.Email) {
		logwrapper.Errorf("Invalid email format: %s", formData.Email)
		httpo.NewErrorResponse(http.StatusBadRequest, "Invalid email format").SendD(c)
		return
	}

	result := db.Model(&models.FormData{}).Create(&formData)
	if result.Error != nil {
		logwrapper.Error(result.Error)
		httpo.NewErrorResponse(http.StatusInternalServerError, result.Error.Error()).SendD(c)
		return
	}
	httpo.NewSuccessResponseP(200, "Form Data saved successfully", formData).SendD(c)
}

func GetOperator(c *gin.Context) {
	db := dbconfig.GetDb()
	var formData []models.FormData
	result := db.Model(&models.FormData{}).Find(&formData)
	if result.Error != nil {
		logwrapper.Error(result.Error.Error())
		httpo.NewErrorResponse(http.StatusInternalServerError, db.Error.Error()).SendD(c)
		return
	}
	httpo.NewSuccessResponseP(200, "Form Data fetched successfully", formData).SendD(c)
}
