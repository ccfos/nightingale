package routes

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/didi/nightingale/src/modules/tsdb/http/render"
	"github.com/didi/nightingale/src/modules/tsdb/index"
)

func ConfigRoutes(r *mux.Router) {
	r.HandleFunc("/api/tsdb/ping", ping)
	r.HandleFunc("/api/tsdb/addr", addr)
	r.HandleFunc("/api/tsdb/pid", pid)

	r.HandleFunc("/api/tsdb/get-item-by-series-id", getItemBySeriesID)
	r.HandleFunc("/api/tsdb/update-index", rebuildIndex)
	r.HandleFunc("/api/tsdb/index-total", indexTotal)
	r.HandleFunc("/api/tsdb/series-total", seriesTotal)
	r.HandleFunc("/api/tsdb/del-rrd-by-counter", delRRDByCounter)
	r.HandleFunc("/api/tsdb/alive-index", indexList)

	r.PathPrefix("/debug").Handler(http.DefaultServeMux)
}

func rebuildIndex(w http.ResponseWriter, r *http.Request) {
	go index.RebuildAllIndex()
	render.Data(w, "ok", nil)
}

func String(r *http.Request, key string, defVal string) (string, error) {
	if val, ok := r.URL.Query()[key]; ok {
		if val[0] == "" {
			return defVal, nil
		}
		return strings.TrimSpace(val[0]), nil
	}

	if r.Form == nil {
		err := r.ParseForm()
		if err != nil {
			return "", err
		}
	}

	val := r.Form.Get(key)
	if val == "" {
		return defVal, nil
	}

	return strings.TrimSpace(val), nil
}

func BindJson(r *http.Request, obj interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("Empty request body")
	}
	defer r.Body.Close()
	body, _ := ioutil.ReadAll(r.Body)

	err := json.Unmarshal(body, obj)
	if err != nil {
		return fmt.Errorf("unmarshal body %s err:%v", string(body), err)
	}
	return err
}
