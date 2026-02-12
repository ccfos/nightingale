package ginx

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errorx"
)

func BindJSON(c *gin.Context, ptr interface{}) {
	err := c.ShouldBindJSON(ptr)
	if err != nil {
		errorx.Bomb(http.StatusBadRequest, "json body invalid: %v", err)
	}
}

func UrlParamStr(c *gin.Context, field string) string {
	val := c.Param(field)

	if val == "" {
		errorx.Bomb(http.StatusBadRequest, "url param[%s] is blank", field)
	}

	return val
}

func UrlParamInt64(c *gin.Context, field string) int64 {
	strval := UrlParamStr(c, field)
	intval, err := strconv.ParseInt(strval, 10, 64)
	if err != nil {
		errorx.Bomb(http.StatusBadRequest, "cannot convert %s to int64", strval)
	}

	return intval
}

func UrlParamInt(c *gin.Context, field string) int {
	return int(UrlParamInt64(c, field))
}

func QueryStr(c *gin.Context, key string, defaultVal ...string) string {
	val := c.Query(key)
	if val != "" {
		return val
	}

	if len(defaultVal) == 0 {
		errorx.Bomb(http.StatusBadRequest, "query param[%s] is necessary", key)
	}

	return defaultVal[0]
}

func QueryInt(c *gin.Context, key string, defaultVal ...int) int {
	strv := c.Query(key)
	if strv != "" {
		intv, err := strconv.Atoi(strv)
		if err != nil {
			errorx.Bomb(http.StatusBadRequest, "cannot convert [%s] to int", strv)
		}
		return intv
	}

	if len(defaultVal) == 0 {
		errorx.Bomb(http.StatusBadRequest, "query param[%s] is necessary", key)
	}

	return defaultVal[0]
}

func QueryInt64(c *gin.Context, key string, defaultVal ...int64) int64 {
	strv := c.Query(key)
	if strv != "" {
		intv, err := strconv.ParseInt(strv, 10, 64)
		if err != nil {
			errorx.Bomb(http.StatusBadRequest, "cannot convert [%s] to int64", strv)
		}
		return intv
	}

	if len(defaultVal) == 0 {
		errorx.Bomb(http.StatusBadRequest, "query param[%s] is necessary", key)
	}

	return defaultVal[0]
}

func QueryBool(c *gin.Context, key string, defaultVal ...bool) bool {
	strv := c.Query(key)
	if strv != "" {
		if strv == "true" || strv == "1" || strv == "on" || strv == "checked" || strv == "yes" || strv == "Y" {
			return true
		} else if strv == "false" || strv == "0" || strv == "off" || strv == "no" || strv == "N" {
			return false
		} else {
			errorx.Bomb(http.StatusBadRequest, "unknown arg[%s] value: %s", key, strv)
		}
	}

	if len(defaultVal) == 0 {
		errorx.Bomb(http.StatusBadRequest, "arg[%s] is necessary", key)
	}

	return defaultVal[0]
}
