package router

import (
	"github.com/didi/nightingale/v5/src/pkg/secu"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

type confPropCrypto struct {
	Data string `json:"data" binding:"required"`
	Key  string `json:"key" binding:"required"`
}

func confPropEncrypt(c *gin.Context) {
	var f confPropCrypto
	ginx.BindJSON(c, &f)

	k := len(f.Key)
	switch k {
	default:
		c.String(400, "The key length should be 16, 24 or 32")
		return
	case 16, 24, 32:
		break
	}

	s, err := secu.DealWithEncrypt(f.Data, f.Key)
	if err != nil {
		c.String(500, err.Error())
	}

	c.JSON(200, gin.H{
		"src":     f.Data,
		"key":     f.Key,
		"encrypt": s,
	})
}

func confPropDecrypt(c *gin.Context) {
	var f confPropCrypto
	ginx.BindJSON(c, &f)

	k := len(f.Key)
	switch k {
	default:
		c.String(400, "The key length should be 16, 24 or 32")
		return
	case 16, 24, 32:
		break
	}

	s, err := secu.DealWithDecrypt(f.Data, f.Key)
	if err != nil {
		c.String(500, err.Error())
	}

	c.JSON(200, gin.H{
		"src":     f.Data,
		"key":     f.Key,
		"decrypt": s,
	})
}
