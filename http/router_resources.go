package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/models"
)

func classpathGetsResources(c *gin.Context) {
	limit := queryInt(c, "limit", defaultLimit)
	prefix := queryInt(c, "prefix", 0)
	query := queryStr(c, "query", "")

	cp := Classpath(urlParamInt64(c, "id"))
	var classpathIds []int64
	if prefix == 1 {
		cps, err := models.ClasspathGetsByPrefix(cp.Path)
		dangerous(err)
		for i := range cps {
			classpathIds = append(classpathIds, cps[i].Id)
		}
	} else {
		classpathIds = append(classpathIds, cp.Id)
	}

	total, err := models.ResourceTotalByClasspathId(classpathIds, query)
	dangerous(err)

	reses, err := models.ResourceGetsByClasspathId(classpathIds, query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"classpath": cp,
		"list":      reses,
		"total":     total,
	}, nil)
}

func resourcesQuery(c *gin.Context) {
	limit := queryInt(c, "limit", defaultLimit)
	qres := queryStr(c, "qres", "")

	// qpaths 可以选择多个，英文逗号分隔的多个id
	qpaths := str.IdsInt64(queryStr(c, "qpaths", ""))

	total, err := models.ResourceTotalByClasspathQuery(qpaths, qres)
	dangerous(err)

	reses, err := models.ResourceGetsByClasspathQuery(qpaths, qres, limit, offset(c, limit))
	dangerous(err)

	if len(reses) == 0 {
		renderZeroPage(c)
		return
	}

	renderData(c, gin.H{
		"list":  reses,
		"total": total,
	}, nil)
}

func resourceGet(c *gin.Context) {
	renderData(c, Resource(urlParamInt64(c, "id")), nil)
}

func resourceDel(c *gin.Context) {
	loginUser(c).MustPerm("resource_modify")
	renderData(c, Resource(urlParamInt64(c, "id")).Del(), nil)
}

type resourceNoteForm struct {
	Ids  []int64 `json:"ids"`
	Note string  `json:"note"`
}

// 修改主机设备的备注
func resourceNotePut(c *gin.Context) {
	var f resourceNoteForm
	bind(c, &f)

	if len(f.Ids) == 0 {
		bomb(http.StatusBadRequest, "ids is empty")
	}

	loginUser(c).MustPerm("resource_modify")

	renderMessage(c, models.ResourceUpdateNote(f.Ids, f.Note))
}

type resourceTagsForm struct {
	Ids  []int64 `json:"ids"`
	Tags string  `json:"tags"`
}

func resourceTagsPut(c *gin.Context) {
	var f resourceTagsForm
	bind(c, &f)

	if len(f.Ids) == 0 {
		bomb(http.StatusBadRequest, "ids is empty")
	}

	loginUser(c).MustPerm("resource_modify")

	renderMessage(c, models.ResourceUpdateTags(f.Ids, f.Tags))
}

type resourceMuteForm struct {
	Ids   []int64 `json:"ids"`
	Btime int64   `json:"btime"`
	Etime int64   `json:"etime"`
}

func resourceMutePut(c *gin.Context) {
	var f resourceMuteForm
	bind(c, &f)

	if len(f.Ids) == 0 {
		bomb(http.StatusBadRequest, "ids is empty")
	}

	loginUser(c).MustPerm("resource_modify")

	renderMessage(c, models.ResourceUpdateMute(f.Ids, f.Btime, f.Etime))
}

type resourceClasspathsForm struct {
	ResIdents    []string `json:"res_idents"`
	ClasspathIds []int64  `json:"classpath_ids"`
}

func resourceClasspathsPut(c *gin.Context) {
	var f resourceClasspathsForm
	m := make(map[string]map[int64]struct{}) //store database data to compare
	toAdd := make(map[string][]int64)

	bind(c, &f)
	loginUser(c).MustPerm("resource_modify")

	sql := "res_ident in (\"" + strings.Join(f.ResIdents, ",") + "\")"
	oldClasspathResources, err := models.ClasspathResourceGets(sql)
	dangerous(err)

	for _, obj := range oldClasspathResources {
		if _, exists := m[obj.ResIdent]; !exists {
			m[obj.ResIdent] = make(map[int64]struct{})
		}
		m[obj.ResIdent][obj.ClasspathId] = struct{}{}
	}

	for _, ident := range f.ResIdents {
		toAdd[ident] = []int64{}
		if _, exists := m[ident]; exists {
			for _, classpathId := range f.ClasspathIds {
				if _, exists := m[ident][classpathId]; exists {
					// classpathResource 在数据库中已存在，不做处理
					delete(m[ident], classpathId)
				} else {
					toAdd[ident] = append(toAdd[ident], classpathId)
				}
			}
		} else {
			toAdd[ident] = f.ClasspathIds
		}
	}

	//删除数据库中多余的classpathResources
	for ident := range m {
		for classpathId := range m[ident] {
			if classpathId == 1 {
				continue
			}

			dangerous(models.ClasspathResourceDel(classpathId, []string{ident}))
		}
	}

	//添加数据库没有的classpathResources
	for ident, cids := range toAdd {
		for _, cid := range cids {
			dangerous(models.ClasspathResourceAdd(cid, ident))
		}
	}
	renderMessage(c, nil)
}
