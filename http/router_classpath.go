package http

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

func classpathListGets(c *gin.Context) {
	limit := queryInt(c, "limit", defaultLimit)
	query := queryStr(c, "query", "")

	total, err := models.ClasspathTotal(query)
	dangerous(err)

	list, err := models.ClasspathGets(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func classpathListNodeGets(c *gin.Context) {
	query := queryStr(c, "query", "")

	list, err := models.ClasspathNodeGets(query)
	dangerous(err)

	renderData(c, list, nil)
}

func classpathListNodeGetsById(c *gin.Context) {
	cp := Classpath(urlParamInt64(c, "id"))
	children, err := cp.DirectChildren()
	dangerous(err)

	renderData(c, children, nil)
}

func classpathFavoriteGet(c *gin.Context) {
	lst, err := loginUser(c).FavoriteClasspaths()
	renderData(c, lst, err)
}

type classpathForm struct {
	Path string `json:"path"`
	Note string `json:"note"`
}

func classpathAdd(c *gin.Context) {
	var f classpathForm
	bind(c, &f)

	me := loginUser(c).MustPerm("classpath_create")

	cp := models.Classpath{
		Path:     f.Path,
		Note:     f.Note,
		Preset:   0,
		CreateBy: me.Username,
		UpdateBy: me.Username,
	}

	renderMessage(c, cp.Add())
}

func classpathPut(c *gin.Context) {
	var f classpathForm
	bind(c, &f)

	me := loginUser(c).MustPerm("classpath_modify")
	cp := Classpath(urlParamInt64(c, "id"))

	if cp.Path != f.Path {
		num, err := models.ClasspathCount("path=? and id<>?", f.Path, cp.Id)
		dangerous(err)

		if num > 0 {
			bomb(200, "Classpath %s already exists", f.Path)
		}
	}

	cp.Path = f.Path
	cp.Note = f.Note
	cp.UpdateBy = me.Username
	cp.UpdateAt = time.Now().Unix()

	renderMessage(c, cp.Update("path", "note", "update_by", "update_at"))
}

func classpathDel(c *gin.Context) {
	loginUser(c).MustPerm("classpath_delete")

	cp := Classpath(urlParamInt64(c, "id"))
	if cp.Preset == 1 {
		bomb(200, "Preset classpath %s cannot delete", cp.Path)
	}

	renderMessage(c, cp.Del())
}

func classpathAddResources(c *gin.Context) {
	var arr []string
	bind(c, &arr)

	me := loginUser(c).MustPerm("classpath_add_resource")
	cp := Classpath(urlParamInt64(c, "id"))

	dangerous(cp.AddResources(arr))

	cp.UpdateAt = time.Now().Unix()
	cp.UpdateBy = me.Username
	cp.Update("update_at", "update_by")

	renderMessage(c, nil)
}

func classpathDelResources(c *gin.Context) {
	var arr []string
	bind(c, &arr)
	classpathId := urlParamInt64(c, "id")
	me := loginUser(c).MustPerm("classpath_del_resource")

	if classpathId == 1 {
		bomb(200, _s("Resource cannot delete in preset classpath"))
	}

	cp := Classpath(classpathId)

	dangerous(cp.DelResources(arr))

	cp.UpdateAt = time.Now().Unix()
	cp.UpdateBy = me.Username
	cp.Update("update_at", "update_by")

	renderMessage(c, nil)
}

func classpathFavoriteAdd(c *gin.Context) {
	me := loginUser(c)
	cp := Classpath(urlParamInt64(c, "id"))
	renderMessage(c, models.ClasspathFavoriteAdd(cp.Id, me.Id))
}

func classpathFavoriteDel(c *gin.Context) {
	me := loginUser(c)
	cp := Classpath(urlParamInt64(c, "id"))
	renderMessage(c, models.ClasspathFavoriteDel(cp.Id, me.Id))
}
