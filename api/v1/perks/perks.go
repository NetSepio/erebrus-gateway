package perks

import "github.com/gin-gonic/gin"

func ApplyRoutesPerks(r *gin.RouterGroup) {
	g := r.Group("/perks")
	{
		ApplyRoutesPerksNFT(g)
		ApplyRoutesPerksToken(g)
	}

}
