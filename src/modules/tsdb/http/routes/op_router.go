package routes

import (
	"net/http"
	"sync/atomic"

	"github.com/didi/nightingale/src/modules/tsdb/cache"
	"github.com/didi/nightingale/src/modules/tsdb/http/render"
	"github.com/didi/nightingale/src/modules/tsdb/index"
	"github.com/didi/nightingale/src/modules/tsdb/rrdtool"
	"github.com/didi/nightingale/src/modules/tsdb/utils"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/toolkits/pkg/file"
)

func getItemBySeriesID(w http.ResponseWriter, r *http.Request) {
	seriesID, err := String(r, "series_id", "")
	if err != nil {
		render.Message(w, err)
		return
	}

	item := index.GetItemFronIndex(seriesID)
	render.Data(w, item, nil)
}

func indexTotal(w http.ResponseWriter, r *http.Request) {
	var total int
	for _, indexMap := range index.IndexedItemCacheBigMap {
		total += indexMap.Size()
	}

	render.Data(w, total, nil)
}

func seriesTotal(w http.ResponseWriter, r *http.Request) {
	render.Data(w, atomic.LoadInt64(&cache.TotalCount), nil)
}

type delRRDRecv struct {
	Endpoint string            `json:"endpoint"`
	Metric   string            `json:"metric"`
	TagsMap  map[string]string `json:"tags"`
	Step     int               `json:"step"`
}

func delRRDByCounter(w http.ResponseWriter, r *http.Request) {
	var inputs []delRRDRecv
	err := BindJson(r, &inputs)
	if err != nil {
		render.Message(w, err)
		return
	}

	for _, input := range inputs {
		seriesId := str.Checksum(input.Endpoint, input.Metric, str.SortedTags(input.TagsMap))
		index.DeleteItemFronIndex(seriesId)

		cache.Caches.Remove(seriesId)

		filename := utils.RrdFileName(rrdtool.Config.Storage, seriesId, "GAUGE", input.Step)
		err = file.Remove(filename)
	}
	render.Data(w, "ok", err)
}

func indexList(w http.ResponseWriter, r *http.Request) {
	render.Data(w, index.IndexList.Get(), nil)
}
