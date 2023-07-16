package obs

import "github.com/gin-gonic/gin"

// package level functions
func ConfigRouter(r *gin.Engine) {
	syncObs.ConfigRouter(r)
}
