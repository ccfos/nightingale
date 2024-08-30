package router

import (
	"github.com/ccfos/nightingale/v6/models"
	"net/http"
	"strings"

	"github.com/toolkits/pkg/errorx"
)

func TaskMeta(id int64) *models.TaskMeta {
	obj, err := models.TaskMetaGet("id = ?", id)
	errorx.Dangerous(err)

	if obj == nil {
		errorx.Bomb(http.StatusNotFound, "no such task meta")
	}

	return obj
}

func cleanHosts(formHosts []string) []string {
	cnt := len(formHosts)
	arr := make([]string, 0, cnt)
	for i := 0; i < cnt; i++ {
		item := strings.TrimSpace(formHosts[i])
		if item == "" {
			continue
		}

		if strings.HasPrefix(item, "#") {
			continue
		}

		arr = append(arr, item)
	}

	return arr
}
