package model

import (
	"fmt"
	"log"
	"strings"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type Node struct {
	Id   int64  `json:"id"`
	Pid  int64  `json:"pid"`
	Name string `json:"name"`
	Path string `json:"path"`
	Leaf int    `json:"leaf"`
	Note string `json:"note"`
}

// InitNode 初始化第一个node节点
func InitNode() {
	num, err := DB["mon"].Where("pid=0").Count(new(Node))
	if err != nil {
		log.Fatalln("cannot query first node", err)
	}

	if num > 0 {
		return
	}

	node := Node{
		Pid:  0,
		Name: "cop",
		Path: "cop",
		Leaf: 0,
		Note: "公司节点",
	}

	_, err = DB["mon"].Insert(&node)
	if err != nil {
		log.Fatalln("cannot insert node[cop]")
	}

	logger.Info("node cop init done")
}

func NodeGets(where string, args ...interface{}) (nodes []Node, err error) {
	if where != "" {
		err = DB["mon"].Where(where, args...).Find(&nodes)
	} else {
		err = DB["mon"].Find(&nodes)
	}
	return nodes, err
}

func NodeGetsByPaths(paths []string) ([]Node, error) {
	if len(paths) == 0 {
		return []Node{}, nil
	}

	var nodes []Node
	err := DB["mon"].In("path", paths).Find(&nodes)
	return nodes, err
}

func NodeByIds(ids []int64) ([]Node, error) {
	if len(ids) == 0 {
		return []Node{}, nil
	}

	return NodeGets(fmt.Sprintf("id in (%s)", str.IdsString(ids)))
}

func NodeQueryPath(query string, limit int) (nodes []Node, err error) {
	err = DB["mon"].Where("path like ?", "%"+query+"%").OrderBy("path").Limit(limit).Find(&nodes)
	return nodes, err
}

func TreeSearchByPath(query string) (nodes []Node, err error) {
	session := DB["mon"].NewSession()
	defer session.Close()

	if strings.Contains(query, " ") {
		arr := strings.Fields(query)
		cnt := len(arr)
		for i := 0; i < cnt; i++ {
			session.Where("path like ?", "%"+arr[i]+"%")
		}
		err = session.Find(&nodes)
	} else {
		err = session.Where("path like ?", "%"+query+"%").Find(&nodes)
	}

	if err != nil {
		return
	}

	cnt := len(nodes)
	if cnt == 0 {
		return
	}

	pathset := make(map[string]struct{})
	for i := 0; i < cnt; i++ {
		pathset[nodes[i].Path] = struct{}{}

		paths := Paths(nodes[i].Path)
		for j := 0; j < len(paths); j++ {
			pathset[paths[j]] = struct{}{}
		}
	}

	var objs []Node
	err = session.In("path", str.MtoL(pathset)).Find(&objs)
	return objs, err
}

func NodeGet(col string, val interface{}) (*Node, error) {
	var obj Node
	has, err := DB["mon"].Where(col+"=?", val).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func NodesGetByIds(ids []int64) ([]Node, error) {
	var objs []Node
	err := DB["mon"].In("id", ids).Find(&objs)
	return objs, err
}

func NodeValid(name, path string) error {
	if len(name) > 32 {
		return fmt.Errorf("name too long")
	}

	if len(path) > 255 {
		return fmt.Errorf("path too long")
	}

	if !str.IsMatch(name, `^[a-z0-9\-]+$`) {
		return fmt.Errorf("name permissible characters: [a-z0-9] and -")
	}

	arr := strings.Split(path, ".")
	if name != arr[len(arr)-1] {
		return fmt.Errorf("name and path not match")
	}

	return nil
}

func (n *Node) CreateChild(name string, leaf int, note string) (int64, error) {
	if n.Leaf == 1 {
		return 0, fmt.Errorf("parent node is leaf, cannot create child")
	}

	path := n.Path + "." + name
	node, err := NodeGet("path", path)
	if err != nil {
		return 0, err
	}

	if node != nil {
		return 0, fmt.Errorf("node[%s] already exists", path)
	}

	child := Node{
		Pid:  n.Id,
		Name: name,
		Path: path,
		Leaf: leaf,
		Note: note,
	}

	_, err = DB["mon"].Insert(&child)
	return child.Id, err
}

func (n *Node) Bind(endpointIds []int64, delOld int) error {
	if delOld == 1 {
		bindings, err := NodeEndpointGetByEndpointIds(endpointIds)
		if err != nil {
			return err
		}

		for i := 0; i < len(bindings); i++ {
			err = NodeEndpointUnbind(bindings[i].NodeId, bindings[i].EndpointId)
			if err != nil {
				return err
			}
		}
	}

	cnt := len(endpointIds)
	for i := 0; i < cnt; i++ {
		if err := NodeEndpointBind(n.Id, endpointIds[i]); err != nil {
			return err
		}
	}

	return nil
}

func (n *Node) Unbind(hostIds []int64) error {
	if len(hostIds) == 0 {
		return nil
	}

	for i := 0; i < len(hostIds); i++ {
		if err := NodeEndpointUnbind(n.Id, hostIds[i]); err != nil {
			return err
		}
	}

	return nil
}

func (n *Node) LeafIds() ([]int64, error) {
	if n.Leaf == 1 {
		return []int64{n.Id}, nil
	}

	var nodes []Node
	err := DB["mon"].Where("path like ? and leaf=1", n.Path+".%").Find(&nodes)
	if err != nil {
		return []int64{}, err
	}

	cnt := len(nodes)
	arr := make([]int64, 0, cnt)
	for i := 0; i < cnt; i++ {
		arr = append(arr, nodes[i].Id)
	}

	return arr, nil
}

func (n *Node) Pids() ([]int64, error) {
	if n.Pid == 0 {
		return []int64{n.Pid}, nil
	}

	var objs []Node
	arr := []int64{}
	paths := []string{}

	nodes := strings.Split(n.Path, ".")
	cnt := len(nodes)

	for i := 1; i < cnt; i++ {
		path := strings.Join(nodes[:cnt-i], ".")
		paths = append(paths, path)
	}

	err := DB["mon"].In("path", paths).Find(&objs)
	if err != nil {
		return []int64{}, err
	}

	cnt = len(objs)
	for i := 0; i < cnt; i++ {
		arr = append(arr, objs[i].Id)
	}

	return arr, nil
}

func (n *Node) Rename(name string) error {
	oldprefix := n.Path + "."

	arr := strings.Split(n.Path, ".")
	arr[len(arr)-1] = name
	newpath := strings.Join(arr, ".")

	newprefix := newpath + "."

	brother, err := NodeGet("path", newpath)
	if err != nil {
		return err
	}

	if brother != nil {
		return fmt.Errorf("%s already exists", newpath)
	}

	var nodes []Node
	err = DB["mon"].Where("path like ?", oldprefix+"%").Find(&nodes)
	if err != nil {
		return err
	}

	session := DB["mon"].NewSession()
	defer session.Close()

	if err = session.Begin(); err != nil {
		return err
	}

	if _, err = session.Exec("UPDATE node SET name=?, path=? WHERE id=?", name, newpath, n.Id); err != nil {
		session.Rollback()
		return err
	}

	cnt := len(nodes)
	for i := 0; i < cnt; i++ {
		if _, err = session.Exec("UPDATE node SET path=? WHERE id=?", strings.Replace(nodes[i].Path, oldprefix, newprefix, 1), nodes[i].Id); err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func (n *Node) Del() error {
	if n.Pid == 0 {
		return fmt.Errorf("cannot delete root node")
	}

	// 叶子节点下不能有endpoint
	if n.Leaf == 1 {
		cnt, err := DB["mon"].Where("node_id=?", n.Id).Count(new(NodeEndpoint))
		if err != nil {
			return err
		}

		if cnt > 0 {
			return fmt.Errorf("there are endpoint binding this node")
		}
	}

	// 非叶子节点下不能有子节点
	if n.Leaf == 0 {
		cnt, err := DB["mon"].Where("pid=?", n.Id).Count(new(Node))
		if err != nil {
			return err
		}

		if cnt > 0 {
			return fmt.Errorf("node[%s] has children node", n.Path)
		}
	}

	_, err := DB["mon"].Where("id=?", n.Id).Delete(new(Node))
	return err
}
