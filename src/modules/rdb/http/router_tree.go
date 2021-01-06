package http

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/src/models"
)

func treeUntilLeafGets(c *gin.Context) {
	me := loginUser(c)
	ret, err := models.UserPermNodes(me)
	dangerous(err)

	query := queryStr(c, "query", "")
	if query == "" {
		// 没有搜索条件，直接返回即可
		renderData(c, ret, nil)
		return
	}

	// 所有的节点在内存里组织为map，方便后续搜索匹配
	cnt := len(ret)
	all := make(map[string]models.Node, cnt)
	for i := 0; i < cnt; i++ {
		all[ret[i].Path] = ret[i]
	}

	// 把搜索条件切分，允许空格分隔多个查询字符串
	arr := strings.Fields(query)
	qsz := len(arr)
	if qsz == 0 {
		renderData(c, ret, nil)
		return
	}

	// 搜索之后，匹配的path放到这个set里
	pathSet := make(map[string]struct{})

	if qsz == 1 {
		// 可能是搜索资源，也可能是搜索节点，先当成资源的uuid或资源的ident来搜索
		res, err := models.ResourceGet("uuid=? or ident=?", arr[0], arr[0])
		dangerous(err)

		if res != nil {
			// 说明用户确实是拿着资源标识在搜索
			nids, err := models.NodeIdsGetByResIds([]int64{res.Id})
			dangerous(err)

			if len(nids) == 0 {
				// 这个资源没有挂载在任何节点上
				renderData(c, []models.Node{}, nil)
				return
			}

			resNodes, err := models.NodeByIds(nids)
			dangerous(err)

			resNodesCount := len(resNodes)
			if resNodesCount == 0 {
				renderData(c, []models.Node{}, nil)
				return
			}

			resNodeMap := make(map[string]struct{})
			for i := 0; i < resNodesCount; i++ {
				resNodeMap[resNodes[i].Path] = struct{}{}
			}

			// 求交集：我有权限的 and 机器有挂载关系的
			for i := 0; i < cnt; i++ {
				if _, ok := resNodeMap[ret[i].Path]; ok {
					pathSet[ret[i].Path] = struct{}{}
				}
			}
		} else {
			// 说明不是resource，那就是在搜索节点
			for i := 0; i < cnt; i++ {
				if strings.Contains(ret[i].Path, arr[0]) {
					pathSet[ret[i].Path] = struct{}{}
				}

				// 根据节点名搜索
				if strings.Contains(ret[i].Name, arr[0]) {
					for j := 0; j < cnt; j++ {
						if strings.HasPrefix(ret[j].Path, ret[i].Path) {
							pathSet[ret[j].Path] = struct{}{}
						}
					}
				}
			}
		}
	} else {
		// 按照空格切分，发现有多个搜索字符串，那必然是在搜索节点
		for i := 0; i < cnt; i++ {
			match := true
			for j := 0; j < qsz; j++ {
				if !strings.Contains(ret[i].Path, arr[j]) {
					match = false
				}

				// 根据节点名搜索
				if strings.Contains(ret[i].Name, arr[j]) {
					for k := 0; k < cnt; k++ {
						if strings.HasPrefix(ret[k].Path, ret[i].Path) {
							pathSet[ret[k].Path] = struct{}{}
						}
					}
				}
			}

			if match {
				pathSet[ret[i].Path] = struct{}{}
			}
		}
	}

	var oks []models.Node

	// 符合条件的这些path，肯定都是长path，还要做个切分，把这些长path的父祖节点也返回，否则没法组成树状结构
	paths := make(map[string]struct{})
	for path := range pathSet {
		lst := models.Paths(path)
		for i := 0; i < len(lst); i++ {
			_, has := paths[lst[i]]
			if !has {
				paths[lst[i]] = struct{}{}
				oks = append(oks, all[lst[i]])
			}
		}
	}

	renderData(c, oks, nil)
}

func v1treeUntilProjectGetsByNid(c *gin.Context) {
	nid := urlParamInt64(c, "id")

	oks, err := models.TreeUntilProjectsGetByNid(nid)
	renderData(c, oks, err)
}

// 这个方法，展示的树只到project，节点搜索功能放到前台去
func treeUntilProjectGets(c *gin.Context) {
	me := loginUser(c)
	oks, err := models.TreeUntilTypGetByUser(me, "project")

	renderData(c, oks, err)
}

// 这个方法，展示的树只到project，节点搜索功能放到前台去
func v1TreeUntilProjectGets(c *gin.Context) {
	username := queryStr(c, "username")
	user, err := models.UserGet("username=?", username)
	dangerous(err)

	oks, err := models.TreeUntilTypGetByUser(user, "project")

	renderData(c, oks, err)
}

// 这个方法，展示的树只到organization
func treeUntilOrganizationGets(c *gin.Context) {
	me := loginUser(c)
	oks, err := models.TreeUntilTypGetByUser(me, "organization")

	renderData(c, oks, err)
}
