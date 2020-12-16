package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/slice"

	"github.com/toolkits/pkg/str"
)

type Node struct {
	Id          int64     `json:"id"`
	Pid         int64     `json:"pid"`
	Ident       string    `json:"ident"`
	Name        string    `json:"name"`
	Note        string    `json:"note"`
	Path        string    `json:"path"`
	Leaf        int       `json:"leaf"`
	Cate        string    `json:"cate"`
	IconColor   string    `json:"icon_color"`
	IconChar    string    `json:"icon_char"`
	Proxy       int       `json:"proxy"`
	Creator     string    `json:"creator"`
	LastUpdated time.Time `json:"last_updated" xorm:"<-"`
	Admins      []User    `json:"admins" xorm:"-"`
}

func (n *Node) FilterMyChildren(nodes []Node) []Node {
	if len(nodes) == 0 {
		return []Node{}
	}

	var children []Node
	var prefix = n.Path + "."
	for i := 0; i < len(nodes); i++ {
		if strings.HasPrefix(nodes[i].Path, prefix) {
			children = append(children, nodes[i])
		}
	}

	return children
}

func (n *Node) FillAdmins() error {
	var ids []int64
	err := DB["rdb"].Table(new(NodeAdmin)).Where("node_id=?", n.Id).Select("user_id").Find(&ids)
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		return nil
	}

	admins, err := UserGetByIds(ids)
	if err != nil {
		return err
	}

	for i := 0; i < len(admins); i++ {
		admins[i].UUID = ""
	}

	n.Admins = admins
	return nil
}

func UpdateIconColor(newColor, cate string) error {
	_, err := DB["rdb"].Exec("UPDATE node SET icon_color=? WHERE cate=?", newColor, cate)
	return err
}

func NodeGet(where string, args ...interface{}) (*Node, error) {
	var obj Node
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func NodeGetById(id int64) (*Node, error) {
	return NodeGet("id=?", id)
}

// NodeGets 在所有节点范围查询，比如管理员看服务树，就需要load所有数据
func NodeGets(where string, args ...interface{}) (nodes []Node, err error) {
	if where != "" {
		err = DB["rdb"].Where(where, args...).OrderBy("path").Find(&nodes)
	} else {
		err = DB["rdb"].OrderBy("path").Find(&nodes)
	}
	return nodes, err
}

func NodeByIds(ids []int64) ([]Node, error) {
	if len(ids) == 0 {
		return []Node{}, nil
	}

	return NodeGets(fmt.Sprintf("id in (%s)", str.IdsString(ids)))
}

func NodeByPaths(paths []string) ([]Node, error) {
	if paths == nil || len(paths) == 0 {
		return []Node{}, nil
	}

	var nodes []Node
	err := DB["rdb"].In("path", paths).Find(&nodes)
	return nodes, err
}

func NodeNew(objPtr *Node, adminIds []int64) error {
	node, err := NodeGet("path=?", objPtr.Path)
	if err != nil {
		return err
	}

	if node != nil {
		return fmt.Errorf("node[%s] already exists", objPtr.Path)
	}

	session := DB["rdb"].NewSession()
	defer session.Close()

	if err = session.Begin(); err != nil {
		return err
	}

	if _, err = session.Insert(objPtr); err != nil {
		session.Rollback()
		return err
	}

	for i := 0; i < len(adminIds); i++ {
		if err := NodeAdminNew(session, objPtr.Id, adminIds[i]); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

var protectedNodeIdents = []string{
	"mysql",
	"rds",
	"redis",
	"mongo",
	"mongodb",
	"pg",
	"postgresql",
	"postgres",
	"api",
	"es",
	"elasticsearch",
	"topic",
	"kvm",
	"dc2",
	"ec2",
	"vm",
	"host",
	"bms",
	"pod",
	"container",
}

// CreateChild 返回创建的子节点
func (n *Node) CreateChild(ident, name, note, cate, creator string, leaf, proxy int, adminIds []int64) (*Node, error) {
	if n.Leaf == 1 {
		return nil, fmt.Errorf("parent node is leaf, cannot create child")
	}

	if cate == "tenant" {
		return nil, fmt.Errorf("tenant node should be root node only")
	}

	if cate == "project" && (n.Cate != "tenant" && n.Cate != "organization") {
		return nil, fmt.Errorf("project node should be under tenant or organization")
	}

	if ident == "" {
		return nil, fmt.Errorf("ident is blank")
	}

	if !str.IsMatch(ident, "^[a-z0-9\\-\\_]+$") {
		return nil, fmt.Errorf("ident invalid")
	}

	if len(ident) >= 32 {
		return nil, fmt.Errorf("ident length should be less than 32")
	}

	if creator != "system" {
		// 人为创建的节点，有些保留名字不能使用，是为了给PaaS各个子系统注册资源所用
		if (n.Path == "inner" || n.Cate == "project") && slice.ContainsString(protectedNodeIdents, ident) {
			return nil, fmt.Errorf("ident: %s is reserved", ident)
		}
	}

	// 对于项目节点比较特殊，ident要求全局唯一
	if cate == "project" {
		node, err := NodeGet("ident=? and cate=?", ident, "project")
		if err != nil {
			return nil, err
		}
		if node != nil {
			return nil, fmt.Errorf("project[%s] already exists", ident)
		}

		if leaf == 1 {
			return nil, fmt.Errorf("project[%s] should not be leaf", ident)
		}
	}

	nc, err := NodeCateGet("ident=?", cate)
	if err != nil {
		return nil, err
	}

	if nc == nil {
		return nil, fmt.Errorf("node-category[%s] not found", cate)
	}

	path := n.Path + "." + ident
	node, err := NodeGet("path=?", path)
	if err != nil {
		return nil, err
	}

	if node != nil {
		return nil, fmt.Errorf("node[%s] already exists", path)
	}

	child := Node{
		Pid:       n.Id,
		Ident:     ident,
		Name:      name,
		Path:      path,
		Leaf:      leaf,
		Cate:      cate,
		IconColor: nc.IconColor,
		Proxy:     proxy,
		Note:      note,
		Creator:   creator,
	}

	child.IconChar = strings.ToUpper(cate[0:1])

	session := DB["rdb"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return nil, err
	}

	if _, err = session.Insert(&child); err != nil {
		session.Rollback()
		return nil, err
	}

	for i := 0; i < len(adminIds); i++ {
		if err := NodeAdminNew(session, child.Id, adminIds[i]); err != nil {
			session.Rollback()
			return nil, err
		}
	}

	err = session.Commit()

	return &child, err
}

func (n *Node) Modify(name, cate, note string, adminIds []int64) error {
	nc, err := NodeCateGet("ident=?", cate)
	if err != nil {
		return err
	}

	if nc == nil {
		return fmt.Errorf("node-category[%s] not found", cate)
	}

	n.Name = name
	n.Cate = cate
	n.IconChar = strings.ToUpper(cate[0:1])
	n.IconColor = nc.IconColor
	n.Note = note

	session := DB["rdb"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err = session.Where("id=?", n.Id).Cols("name", "cate", "icon_char", "icon_color", "note").Update(n); err != nil {
		session.Rollback()
		return err
	}

	if err = NodeClearAdmins(session, n.Id); err != nil {
		session.Rollback()
		return err
	}

	for i := 0; i < len(adminIds); i++ {
		if err := NodeAdminNew(session, n.Id, adminIds[i]); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func (n *Node) Del() error {
	// inner 租户节点不允许删除
	if n.Path == InnerTenantIdent {
		return fmt.Errorf("cannot delete inner tenant")
	}

	// 叶子节点下不能有机器
	if n.Leaf == 1 {
		cnt, err := DB["rdb"].Where("node_id=?", n.Id).Count(new(NodeResource))
		if err != nil {
			return err
		}

		if cnt > 0 {
			return fmt.Errorf("there are resources binding this node")
		}
	}

	// 非叶子节点下不能有子节点
	if n.Leaf == 0 {
		cnt, err := DB["rdb"].Where("pid=?", n.Id).Count(new(Node))
		if err != nil {
			return err
		}

		if cnt > 0 {
			return fmt.Errorf("node[%s] has children node", n.Path)
		}
	}

	session := DB["rdb"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM node WHERE id=?", n.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM node_admin WHERE node_id=?", n.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM node_role WHERE node_id=?", n.Id); err != nil {
		session.Rollback()
		return err
	}

	// 在垃圾桶保留一份，以防万一后面要找回，只有超管可以看到这个垃圾桶页面
	nt := NodeTrash{
		Id:        n.Id,
		Pid:       n.Pid,
		Ident:     n.Ident,
		Name:      n.Name,
		Note:      n.Note,
		Path:      n.Path,
		Leaf:      n.Leaf,
		Cate:      n.Cate,
		IconColor: n.IconColor,
		IconChar:  n.IconChar,
		Proxy:     n.Proxy,
		Creator:   n.Creator,
	}

	if _, err := session.Insert(&nt); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func (n *Node) RoleTotal(username string) (int64, error) {
	session := DB["rdb"].Where("node_id = ? or node_id in (select id from node where path like ?)", n.Id, n.Path+".%")
	if username != "" {
		session = session.Where("username = ?", username)
	}
	return session.Count(new(NodeRole))
}

func (n *Node) RoleList(username string, limit, offset int) ([]NodeRole, error) {
	sql := "select node_role.id id, node_role.node_id node_id, node_role.username username, node_role.role_id role_id, node.path node_path from node_role, node where node_role.node_id = node.id and (node.id = %d or node.path like '%s')"

	sql = fmt.Sprintf(sql, n.Id, n.Path+".%")

	if username != "" {
		sql += fmt.Sprintf(" and node_role.username = '%s'", username)
	}

	sql += " order by node.path limit ? offset ?"

	var objs []NodeRole
	err := DB["rdb"].SQL(sql, limit, offset).Find(&objs)
	return objs, err
}

func (n *Node) Tenant() string {
	return strings.Split(n.Path, ".")[0]
}

// LeafIds 叶子节点的id
func (n *Node) LeafIds() ([]int64, error) {
	if n.Leaf == 1 {
		return []int64{n.Id}, nil
	}

	var ids []int64
	err := DB["rdb"].Table(new(Node)).Where("path like ? and leaf = 1", n.Path+".%").Select("id").Find(&ids)
	return ids, err
}

// 根据一堆节点获取下面的叶子节点的ID列表
func LeafIdsByNodes(nodes []Node) ([]int64, error) {
	count := len(nodes)
	if count == 0 {
		return []int64{}, nil
	}

	lidsMap := make(map[int64]struct{})
	for i := 0; i < count; i++ {
		lids, err := nodes[i].LeafIds()
		if err != nil {
			return nil, err
		}

		for j := 0; j < len(lids); j++ {
			lidsMap[lids[j]] = struct{}{}
		}
	}

	count = len(lidsMap)
	list := make([]int64, 0, count)
	for lid := range lidsMap {
		list = append(list, lid)
	}

	return list, nil
}

func NodeIdsByPaths(paths []string) ([]int64, error) {
	if len(paths) == 0 {
		return []int64{}, nil
	}

	var ids []int64
	err := DB["rdb"].Table("node").In("path", paths).Select("id").Find(&ids)
	return ids, err
}

func PermNodes(myNodes []Node) ([]Node, error) {
	var ret []Node

	for i := 0; i < len(myNodes); i++ {
		objs, err := NodeByPaths(Paths(myNodes[i].Path))
		if err != nil {
			return nil, err
		}
		ret = append(ret, objs...)

		if myNodes[i].Leaf == 0 {
			var nodes []Node
			err = DB["rdb"].Where("path like ?", myNodes[i].Path+".%").Find(&nodes)
			if err != nil {
				return nil, err
			}

			if len(nodes) > 0 {
				ret = append(ret, nodes...)
			}
		}
	}

	cnt := len(ret)
	set := make(map[string]struct{}, cnt)
	lst := make([]Node, 0, cnt)
	for i := 0; i < cnt; i++ {
		if _, has := set[ret[i].Path]; has {
			continue
		}

		lst = append(lst, ret[i])
		set[ret[i].Path] = struct{}{}
	}

	return lst, nil
}

// Unbind 从某个服务树节点解挂机器
func (n *Node) Unbind(resIds []int64) error {
	if n.Leaf != 1 {
		return fmt.Errorf("node[%s] not leaf", n.Path)
	}

	if len(resIds) == 0 {
		return nil
	}

	for i := 0; i < len(resIds); i++ {
		if err := NodeResourceUnbind(n.Id, resIds[i]); err != nil {
			return err
		}
	}

	return nil
}

// Bind 把资源挂载到某个树节点
func (n *Node) Bind(resIds []int64) error {
	if n.Leaf != 1 {
		return fmt.Errorf("node[%s] not leaf", n.Path)
	}

	tenant := n.Tenant()

	if tenant != InnerTenantIdent {
		// 所有机器必须属于这个租户才可以挂载到这个租户下的节点，唯独inner节点特殊，inner节点可以挂载其他租户的资源
		var notMine []Resource
		err := DB["rdb"].In("id", resIds).Where("tenant<>?", tenant).Find(&notMine)
		if err != nil {
			return err
		}

		size := len(notMine)
		if size > 0 {
			arr := make([]string, size)
			for i := 0; i < size; i++ {
				arr[i] = fmt.Sprintf("%s[%s]", notMine[i].Ident, notMine[i].Name)
			}
			return fmt.Errorf("%s dont belong to tenant[%s]", strings.Join(arr, ", "), tenant)
		}
	}

	cnt := len(resIds)
	for i := 0; i < cnt; i++ {
		if err := NodeResourceBind(n.Id, resIds[i]); err != nil {
			return err
		}
	}

	return nil
}

//todo 是否需要待确认
func (n *Node) RelatedNodes() ([]Node, error) {
	var nodes []Node
	err := DB["rdb"].Table(new(Node)).Where("id="+fmt.Sprint(n.Id)+" or path like ?", n.Path+".%").Find(&nodes)
	return nodes, err
}

func GetLeafNidsForMon(nid int64, exclNid []int64) ([]int64, error) {
	var nids []int64
	idsMap := make(map[int64]struct{})

	node, err := NodeGet("id=?", nid)
	if err != nil {
		return nids, err
	}

	if node == nil {
		// 节点已经被删了，相关的告警策略也删除
		StraDelByNid(nid)
		return []int64{}, nil
	}

	nodeIds, err := node.LeafIds()
	if err != nil {
		return nids, err
	}

	for _, id := range nodeIds {
		idsMap[id] = struct{}{}
	}

	for _, id := range exclNid {
		node, err := NodeGet("id=?", id)
		if err != nil {
			return nids, err
		}

		if node == nil {
			continue
		}
		if node.Leaf == 1 {
			delete(idsMap, id)
		} else {
			nodeIds, err := node.LeafIds()
			if err != nil {
				return nids, err
			}

			for _, id := range nodeIds {
				delete(idsMap, id)
			}
		}
	}

	for id, _ := range idsMap {
		nids = append(nids, id)
	}

	return nids, err
}

func GetRelatedNidsForMon(nid int64, exclNid []int64) ([]int64, error) {
	var nids []int64
	idsMap := make(map[int64]struct{})

	node, err := NodeGet("id=?", nid)
	if err != nil {
		return nids, err
	}

	nodes, err := node.RelatedNodes()
	if err != nil {
		return nids, err
	}

	for _, node := range nodes {
		idsMap[node.Id] = struct{}{}
	}

	for _, id := range exclNid {
		node, err := NodeGet("id=?", id)
		if err != nil {
			return nids, err
		}

		if node == nil {
			continue
		}
		if node.Leaf == 1 {
			delete(idsMap, id)
		} else {
			nodes, err := node.RelatedNodes()
			if err != nil {
				return nids, err
			}

			for _, node := range nodes {
				delete(idsMap, node.Id)
			}
		}
	}

	for id, _ := range idsMap {
		nids = append(nids, id)
	}

	return nids, err
}
