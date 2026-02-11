package ginx

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/i18n"
)

type Render struct {
	code int
	ctx  *gin.Context
}

func NewRender(c *gin.Context, code ...int) Render {
	r := Render{ctx: c}
	if len(code) > 0 {
		r.code = code[0]
	} else {
		r.code = 200
	}
	return r
}

func (r Render) Message(v interface{}, a ...interface{}) {
	requestId := r.ctx.GetString("trace_id")
	if v == nil {
		if r.code == 200 {
			r.ctx.JSON(r.code, gin.H{"err": "", "request_id": requestId})
		} else {
			r.ctx.String(r.code, "")
		}
		return
	}

	switch t := v.(type) {
	case string:
		msg := i18n.Sprintf(r.ctx.GetHeader("X-Language"), t, a...)
		if r.code == 200 {
			r.ctx.JSON(r.code, gin.H{"err": msg, "request_id": requestId})
		} else {
			r.ctx.String(r.code, msg)
		}
	case error:
		msg := i18n.Sprintf(r.ctx.GetHeader("X-Language"), t.Error(), a...)
		if r.code == 200 {
			r.ctx.JSON(r.code, gin.H{"err": msg, "request_id": requestId})
		} else {
			r.ctx.String(r.code, msg)
		}
	}
}

func (r Render) Data(data interface{}, err interface{}, a ...interface{}) {
	if err == nil {
		r.ctx.JSON(r.code, gin.H{"dat": data, "err": "", "request_id": r.ctx.GetString("trace_id")})
		return
	}

	r.Message(err, a...)
}

func (r Render) ZeroPage(c *gin.Context) {
	r.Data(c, gin.H{
		"list":  []int{},
		"total": 0,
	}, nil)
}
