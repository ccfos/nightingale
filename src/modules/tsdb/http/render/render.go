package render

import (
	"net/http"

	"github.com/unrolled/render"
)

var Render *render.Render

func Init() {
	Render = render.New(render.Options{
		Directory:  "tsdb",
		Extensions: []string{".html"},
		Delims:     render.Delims{"{{", "}}"},
		IndentJSON: false,
	})
}

func Message(w http.ResponseWriter, v interface{}) {
	if v == nil {
		Render.JSON(w, http.StatusOK, map[string]string{"err": ""})
		return
	}

	switch t := v.(type) {
	case string:
		Render.JSON(w, http.StatusOK, map[string]string{"err": t})
	case error:
		Render.JSON(w, http.StatusOK, map[string]string{"err": t.Error()})
	}
}

func Data(w http.ResponseWriter, v interface{}, err error) {
	if err != nil {
		Render.JSON(w, http.StatusOK, map[string]interface{}{"err": err.Error(), "dat": v})
	} else {
		Render.JSON(w, http.StatusOK, map[string]interface{}{"err": "", "dat": v})
	}
}
