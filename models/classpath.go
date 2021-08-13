package models

import (
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type Classpath struct {
	Id       int64  `json:"id"`
	Path     string `json:"path"`
	Note     string `json:"note"`
	Preset   int    `json:"preset"`
	CreateAt int64  `json:"create_at"`
	CreateBy string `json:"create_by"`
	UpdateAt int64  `json:"update_at"`
	UpdateBy string `json:"update_by"`
}

type ClasspathTree struct {
	Id       int64           `json:"id"`
	Path     string          `json:"path"`
	Note     string          `json:"note"`
	Preset   int             `json:"preset"`
	CreateAt int64           `json:"create_at"`
	CreateBy string          `json:"create_by"`
	UpdateAt int64           `json:"update_at"`
	UpdateBy string          `json:"update_by"`
	Children []ClasspathTree `json:"child"`
}

type Node struct {
	Children []*Node
	Val      string
	Obj      Classpath
}

func (c *Classpath) TableName() string {
	return "classpath"
}

func (c *Classpath) Validate() error {
	if str.Dangerous(c.Path) {
		return _e("Classpath path has invalid characters")
	}

	if strings.Contains(c.Path, " ") {
		return _e("Classpath path has invalid characters")
	}

	if str.Dangerous(c.Note) {
		return _e("Classpath note has invalid characters")
	}

	return nil
}

func (c *Classpath) Add() error {
	if err := c.Validate(); err != nil {
		return err
	}

	num, err := ClasspathCount("path=?", c.Path)
	if err != nil {
		return err
	}

	if num > 0 {
		return _e("Classpath %s already exists", c.Path)
	}

	now := time.Now().Unix()
	c.CreateAt = now
	c.UpdateAt = now
	return DBInsertOne(c)
}

func ClasspathCount(where string, args ...interface{}) (num int64, err error) {
	num, err = DB.Where(where, args...).Count(new(Classpath))
	if err != nil {
		logger.Errorf("mysql.error: count classpath fail: %v", err)
		return num, internalServerError
	}
	return num, nil
}

func (c *Classpath) Update(cols ...string) error {
	if err := c.Validate(); err != nil {
		return err
	}

	_, err := DB.Where("id=?", c.Id).Cols(cols...).Update(c)
	if err != nil {
		logger.Errorf("mysql.error: update classpath(id=%d) fail: %v", c.Id, err)
		return internalServerError
	}

	return nil
}

func ClasspathTotal(query string) (num int64, err error) {
	if query != "" {
		q := "%" + query + "%"
		num, err = DB.Where("path like ?", q).Count(new(Classpath))
	} else {
		num, err = DB.Count(new(Classpath))
	}

	if err != nil {
		logger.Errorf("mysql.error: count classpath fail: %v", err)
		return 0, internalServerError
	}

	return num, nil
}

func ClasspathGets(query string, limit, offset int) ([]Classpath, error) {
	objs, err := ClasspathQuery(query, limit, offset)
	if err != nil {
		logger.Errorf("mysql.error: query classpath fail: %v", err)
		return objs, internalServerError
	}

	if len(objs) == 0 {
		return []Classpath{}, nil
	}

	return objs, nil
}

func ClasspathGetAll() ([]Classpath, error) {
	var objs []Classpath
	err := DB.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query classpath fail: %v", err)
		return objs, internalServerError
	}

	if len(objs) == 0 {
		return []Classpath{}, nil
	}

	return objs, nil
}

func ClasspathGet(where string, args ...interface{}) (*Classpath, error) {
	var obj Classpath
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query classpath(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func ClasspathGetsByPrefix(prefix string) ([]Classpath, error) {
	var objs []Classpath
	err := DB.Where("path like ?", prefix+"%").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query classpath fail: %v", err)
		return objs, internalServerError
	}

	if len(objs) == 0 {
		return []Classpath{}, nil
	}

	return objs, nil
}

// Del classpath的删除，前提是挂载的机器、配置的采集策略都要提前删除
func (c *Classpath) Del() error {
	num, err := ClasspathResourceCount("classpath_id=?", c.Id)
	if err != nil {
		return err
	}

	if num > 0 {
		return _e("There are still resources under the classpath")
	}

	num, err = CollectRuleCount("classpath_id=?", c.Id)
	if err != nil {
		return err
	}

	if num > 0 {
		return _e("There are still collect rules under the classpath")
	}

	session := DB.NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM classpath_favorite WHERE classpath_id=?", c.Id); err != nil {
		logger.Errorf("mysql.error: delete classpath_favorite fail: %v", err)
		return err
	}

	if _, err := session.Exec("DELETE FROM classpath WHERE id=?", c.Id); err != nil {
		logger.Errorf("mysql.error: delete classpath fail: %v", err)
		return err
	}

	return session.Commit()
}

func (c *Classpath) AddResources(idents []string) error {
	count := len(idents)
	for i := 0; i < count; i++ {
		err := ClasspathResourceAdd(c.Id, strings.TrimSpace(idents[i]))
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Classpath) DelResources(idents []string) error {
	return ClasspathResourceDel(c.Id, idents)
}

func ClasspathTreeNodesGets(query string, limit, offset int) ([]ClasspathTree, error) {
	objs, err := ClasspathQuery(query, limit, offset)
	if err != nil {
		logger.Errorf("mysql.error: query classpath fail: %v", err)
		return []ClasspathTree{}, internalServerError
	}

	if len(objs) == 0 {
		return []ClasspathTree{}, nil
	}
	pcs := ClasspathTreeAllChildren(objs)

	return pcs, nil
}

func ClasspathQuery(query string, limit, offset int) ([]Classpath, error) {
	session := DB.Limit(limit, offset).OrderBy("path")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("path like ?", q)
	}
	var objs []Classpath
	err := session.Find(&objs)

	return objs, err
}

func ClasspathNodeGetsById(cp Classpath) ([]ClasspathTree, error) {
	objs, err := ClasspathGetsByPrefix(cp.Path)
	if err != nil {
		logger.Errorf("mysql.error: query prefix classpath fail: %v", err)
		return []ClasspathTree{}, internalServerError
	}
	if len(objs) == 0 {
		return []ClasspathTree{}, nil
	}
	pcs := ClasspathNodeChild(objs)

	return pcs, nil
}

func ClasspathTreeAllChildren(cps []Classpath) []ClasspathTree {
	var node Node
	var objs []ClasspathTree

	for _, cp := range cps {
		TreeInsert(cp, &node)
	}

	for _, child := range node.Children {
		objs = append(objs, TreeAllChildren(child))
	}

	return objs
}

func ClasspathNodeChild(cps []Classpath) []ClasspathTree {
	var node Node
	var objs []ClasspathTree

	for _, cp := range cps {
		TreeInsert(cp, &node)
	}

	for _, child := range node.Children {
		objs = append(objs, NodeChild(child))
	}

	return objs
}

func TreeInsert(obj Classpath, node *Node) {
	path := obj.Path
	ok := true
	for {
		if len(node.Children) == 0 {
			break
		}
		child := node.Children[len(node.Children)-1]
		prefix := child.Val
		ok = strings.HasPrefix(path, prefix)
		if !ok {
			break
		}
		path = path[len(prefix):]
		node = child
	}
	var newNode Node
	newNode.Obj = obj
	newNode.Val = path
	node.Children = append(node.Children, &newNode)
}

func ToClasspathTree(node *Node) ClasspathTree {
	var obj ClasspathTree

	obj.Id = node.Obj.Id
	obj.Path = node.Val
	obj.Note = node.Obj.Note
	obj.Preset = node.Obj.Preset
	obj.CreateAt = node.Obj.CreateAt
	obj.CreateBy = node.Obj.CreateBy
	obj.UpdateAt = node.Obj.UpdateAt
	obj.UpdateBy = node.Obj.UpdateBy
	obj.Children = []ClasspathTree{}
	return obj
}

func TreeAllChildren(node *Node) ClasspathTree {
	obj := ToClasspathTree(node)

	if len(node.Children) == 0 {
		obj.Children = []ClasspathTree{}
		return obj
	}

	for _, child := range node.Children {
		obj.Children = append(obj.Children, TreeAllChildren(child))
	}
	return obj
}

func NodeChild(node *Node) ClasspathTree {
	obj := ToClasspathTree(node)

	if len(node.Children) == 0 {
		obj.Children = []ClasspathTree{}
		return obj
	}

	for _, child := range node.Children {
		obj.Children = append(obj.Children, ToClasspathTree(child))
	}
	return obj
}
