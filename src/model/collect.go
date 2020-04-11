package model

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"xorm.io/xorm"
)

type Collect struct {
	sync.RWMutex
	Ports map[int]*PortCollect    `json:"ports"`
	Procs map[string]*ProcCollect `json:"procs"`
	Logs  map[string]*LogCollect  `json:"logs"`
}

func NewCollect() *Collect {
	return &Collect{
		Ports: make(map[int]*PortCollect),
		Procs: make(map[string]*ProcCollect),
		Logs:  make(map[string]*LogCollect),
	}
}

func (c *Collect) Update(cc *Collect) {
	c.Lock()
	defer c.Unlock()
	//更新端口采集配置
	c.Ports = make(map[int]*PortCollect)
	for k, v := range cc.Ports {
		c.Ports[k] = v
	}

	//更新进程采集配置
	c.Procs = make(map[string]*ProcCollect)
	for k, v := range cc.Procs {
		c.Procs[k] = v
	}

	//更新log采集配置
	c.Logs = make(map[string]*LogCollect)
	for k, v := range cc.Logs {
		c.Logs[k] = v
	}
}

func (c *Collect) GetPorts() map[int]*PortCollect {
	c.RLock()
	defer c.RUnlock()

	tmp := make(map[int]*PortCollect)
	for k, v := range c.Ports {
		tmp[k] = v
	}
	return tmp
}

func (c *Collect) GetProcs() map[string]*ProcCollect {
	c.RLock()
	defer c.RUnlock()

	tmp := make(map[string]*ProcCollect)
	for k, v := range c.Procs {
		tmp[k] = v
	}
	return tmp
}

func (c *Collect) GetLogConfig() map[string]*LogCollect {
	c.RLock()
	defer c.RUnlock()

	tmp := make(map[string]*LogCollect)
	for k, v := range c.Logs {
		tmp[k] = v
	}
	return tmp
}

type PortCollect struct {
	Id          int64     `json:"id"`
	Nid         int64     `json:"nid"`
	CollectType string    `json:"collect_type"`
	Name        string    `json:"name"`
	Tags        string    `json:"tags"`
	Step        int       `json:"step"`
	Comment     string    `json:"comment"`
	Creator     string    `json:"creator"`
	Created     time.Time `xorm:"updated" json:"created"`
	LastUpdator string    `xorm:"last_updator" json:"last_updator"`
	LastUpdated time.Time `xorm:"updated" json:"last_updated"`

	Port    int `json:"port"`
	Timeout int `json:"timeout"`
}

type ProcCollect struct {
	Id          int64     `json:"id"`
	Nid         int64     `json:"nid"`
	CollectType string    `json:"collect_type"`
	Name        string    `json:"name"`
	Tags        string    `json:"tags"`
	Step        int       `json:"step"`
	Comment     string    `json:"comment"`
	Creator     string    `json:"creator"`
	Created     time.Time `xorm:"updated" json:"created"`
	LastUpdator string    `xorm:"last_updator" json:"last_updator"`
	LastUpdated time.Time `xorm:"updated" json:"last_updated"`

	Target        string `json:"target"`
	CollectMethod string `json:"collect_method"`
}

type LogCollect struct {
	Id          int64     `json:"id"`
	Nid         int64     `json:"nid"`
	CollectType string    `json:"collect_type"`
	Name        string    `json:"name"`
	TagsStr     string    `xorm:"tags" json:"-"`
	Step        int       `json:"step"`
	Comment     string    `json:"comment"`
	Creator     string    `json:"creator"`
	Created     time.Time `xorm:"updated" json:"created"`
	LastUpdator string    `xorm:"last_updator" json:"last_updator"`
	LastUpdated time.Time `xorm:"updated" json:"last_updated"`

	Tags map[string]string `xorm:"-" json:"tags"`

	FilePath   string `json:"file_path"`
	TimeFormat string `json:"time_format"`
	Pattern    string `json:"pattern"`
	Func       string `json:"func"`
	FuncType   string `json:"func_type"`
	Unit       string `json:"unit"`

	Degree    int    `json:"degree"`
	Zerofill  int    `xorm:"zero_fill" json:"zerofill"`
	Aggregate string `json:"aggregate"`

	LocalUpdated int64                     `xorm:"-" json:"-"`
	TimeReg      *regexp.Regexp            `xorm:"-" json:"-"`
	PatternReg   *regexp.Regexp            `xorm:"-" json:"-"`
	ExcludeReg   *regexp.Regexp            `xorm:"-" json:"-"`
	TagRegs      map[string]*regexp.Regexp `xorm:"-" json:"-"`
	ParseSucc    bool                      `xorm:"-" json:"-"`
}

type CollectHist struct {
	Id          int64     `json:"id"`
	Cid         int64     `json:"cid"`
	CollectType string    `json:"collect_type"`
	Action      string    `json:"action"`
	Body        string    `json:"body"`
	Creator     string    `json:"creator"`
	Created     time.Time `xorm:"created" json:"created"`
}

func (l *LogCollect) Encode() error {
	tags, err := json.Marshal(l.Tags)
	if err != nil {
		return fmt.Errorf("encode excl_nid err:%v", err)
	}
	l.TagsStr = string(tags)
	return nil
}

func (l *LogCollect) Decode() error {
	err := json.Unmarshal([]byte(l.TagsStr), &l.Tags)
	if err != nil {
		return err
	}
	return nil
}

func GetPortCollects() ([]*PortCollect, error) {
	collects := []*PortCollect{}
	err := DB["mon"].Find(&collects)
	return collects, err
}

func (p *PortCollect) Update() error {
	session := DB["mon"].NewSession()
	defer session.Close()

	err := session.Begin()
	if err != nil {
		return err
	}

	if _, err = session.ID(p.Id).AllCols().Update(p); err != nil {
		session.Rollback()
		return err
	}

	portByte, err := json.Marshal(p)
	if err != nil {
		session.Rollback()
		return err
	}

	if err := saveHist(p.Id, "port", "update", p.Creator, string(portByte), session); err != nil {
		session.Rollback()
		return err
	}

	if err = session.Commit(); err != nil {
		return err
	}

	return err
}

func GetProcCollects() ([]*ProcCollect, error) {
	collects := []*ProcCollect{}
	err := DB["mon"].Find(&collects)
	return collects, err
}

func (p *ProcCollect) Update() error {
	session := DB["mon"].NewSession()
	defer session.Close()

	err := session.Begin()
	if err != nil {
		return err
	}

	if _, err = session.ID(p.Id).AllCols().Update(p); err != nil {
		session.Rollback()
		return err
	}

	b, err := json.Marshal(p)
	if err != nil {
		session.Rollback()
		return err
	}

	if err := saveHist(p.Id, "port", "update", p.Creator, string(b), session); err != nil {
		session.Rollback()
		return err
	}

	if err = session.Commit(); err != nil {
		return err
	}

	return err
}

func GetLogCollects() ([]*LogCollect, error) {
	collects := []*LogCollect{}
	err := DB["mon"].Find(&collects)
	return collects, err
}

func (l *LogCollect) Update() error {
	session := DB["mon"].NewSession()
	defer session.Close()

	err := session.Begin()
	if err != nil {
		return err
	}

	if _, err = session.ID(l.Id).AllCols().Update(l); err != nil {
		session.Rollback()
		return err
	}

	b, err := json.Marshal(l)
	if err != nil {
		session.Rollback()
		return err
	}

	if err := saveHist(l.Id, "log", "update", l.Creator, string(b), session); err != nil {
		session.Rollback()
		return err
	}

	if err = session.Commit(); err != nil {
		return err
	}

	return err
}

func CreateCollect(collectType, creator string, collect interface{}) error {
	session := DB["mon"].NewSession()
	defer session.Close()

	err := session.Begin()
	if err != nil {
		return err
	}

	if _, err := session.Insert(collect); err != nil {
		session.Rollback()
		return err
	}

	b, err := json.Marshal(collect)
	if err != nil {
		session.Rollback()
		return err
	}

	if err := saveHist(0, collectType, "create", creator, string(b), session); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func GetCollectByNid(collectType string, nids []int64) ([]interface{}, error) {
	var res []interface{}
	switch collectType {
	case "port":
		collects := []PortCollect{}
		err := DB["mon"].In("nid", nids).Find(&collects)
		for _, c := range collects {
			res = append(res, c)
		}
		return res, err

	case "proc":
		collects := []ProcCollect{}
		err := DB["mon"].In("nid", nids).Find(&collects)
		for _, c := range collects {
			res = append(res, c)
		}
		return res, err

	case "log":
		collects := []LogCollect{}
		err := DB["mon"].In("nid", nids).Find(&collects)
		for _, c := range collects {
			c.Decode()
			res = append(res, c)
		}
		return res, err

	default:
		return nil, fmt.Errorf("illegal collectType")
	}

}

func GetCollectById(collectType string, cid int64) (interface{}, error) {
	switch collectType {
	case "port":
		collect := new(PortCollect)
		_, err := DB["mon"].Where("id = ?", cid).Get(collect)
		return collect, err
	case "proc":
		collect := new(ProcCollect)
		_, err := DB["mon"].Where("id = ?", cid).Get(collect)
		return collect, err
	case "log":
		collect := new(LogCollect)
		_, err := DB["mon"].Where("id = ?", cid).Get(collect)
		collect.Decode()
		return collect, err

	default:
		return nil, fmt.Errorf("illegal collectType")
	}
}

func GetCollectByName(collectType string, name string) (interface{}, error) {
	var collect interface{}
	_, err := DB["mon"].Table(collectType+"_collect").Where("name = ?", name).Get(&collect)
	return collect, err
}

func DeleteCollectById(collectType, creator string, cid int64) error {
	session := DB["mon"].NewSession()
	defer session.Close()

	sql := "delete from " + collectType + "_collect where id = ?"
	_, err := DB["mon"].Exec(sql, cid)
	if err != nil {
		session.Rollback()
		return err
	}

	if err := saveHist(cid, collectType, "delete", creator, strconv.FormatInt(cid, 10), session); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func saveHist(id int64, tp string, action, username, body string, session *xorm.Session) error {
	h := CollectHist{
		Cid:         id,
		CollectType: tp,
		Action:      action,
		Creator:     username,
		Body:        body,
	}

	_, err := session.Insert(&h)
	if err != nil {
		session.Rollback()
		return err
	}

	return err
}

func GetCollectsModel(t string) (interface{}, error) {
	collects := make([]*PortCollect, 0)
	switch t {
	case "port":
		return collects, nil
	case "proc":
		return collects, nil

	default:
		return nil, fmt.Errorf("illegal collectType")
	}
}
