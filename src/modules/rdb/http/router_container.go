package http

import (
	"github.com/gin-gonic/gin"
)

type v1ContainersRegisterItem struct {
	UUID   string `json:"uuid"`
	Ident  string `json:"ident"`
	Name   string `json:"name"`
	Labels string `json:"labels"`
	Extend string `json:"extend"`
	Cate   string `json:"cate"`
	NID    int64  `json:"nid"`
}

func (f v1ContainersRegisterItem) Validate() {
	if f.Cate == "" {
		bomb("cate is blank")
	}

	if f.UUID == "" {
		bomb("uuid is blank")
	}

	if f.Ident == "" {
		bomb("ident is blank")
	}
}

func v1ContainersBindPost(c *gin.Context) {
	var items []v1ContainersRegisterItem
	bind(c, &items)

	count := len(items)
	if count == 0 {
		bomb("items empty")
	}

	resourceHttpRegister(count, items)

	renderMessage(c, nil)
}
