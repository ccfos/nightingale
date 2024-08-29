package poster

import (
	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
)

type DataResponse[T any] struct {
	Dat T      `json:"dat"`
	Err string `json:"err"`
}

func NewN9eCtx(centerApi conf.CenterApi) *ctx.Context {
	return &ctx.Context{
		CenterApi: centerApi,
	}
}

func PostByUrls(centerApi conf.CenterApi, path string, v interface{}) (err error) {
	n9eCtx := NewN9eCtx(centerApi)

	return poster.PostByUrls(n9eCtx, path, v)
}

func PostByUrlsWithResp[T any](centerApi conf.CenterApi, path string, v interface{}) (t T, err error) {
	n9eCtx := NewN9eCtx(centerApi)

	return poster.PostByUrlsWithResp[T](n9eCtx, path, v)
}
