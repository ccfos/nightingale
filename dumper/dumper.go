package dumper

import "github.com/gin-gonic/gin"

// package level functions
func ConfigRouter(r *gin.Engine) {
	syncDumper.ConfigRouter(r)
}
