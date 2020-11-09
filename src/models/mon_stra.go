package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"xorm.io/xorm"
)

type Stra struct {
	Id                  int64     `json:"id"`
	Name                string    `json:"name"`
	Category            int       `json:"category"` //机器，非机器
	Nid                 int64     `json:"nid"`
	ExclNidStr          string    `xorm:"excl_nid" json:"-"`            //排除的叶子节点
	AlertDur            int       `json:"alert_dur"`                    //单位秒，持续异常10分钟则产生异常event
	RecoveryDur         int       `json:"recovery_dur"`                 //单位秒，持续正常2分钟则产生恢复event，0表示立即产生恢复event
	RecoveryNotify      int       `json:"recovery_notify"`              //1 发送恢复通知 0不发送恢复通知
	ExprsStr            string    `xorm:"exprs" json:"-"`               //多个条件的监控实例需要相同，并且同时满足才产生event
	TagsStr             string    `xorm:"tags" json:"-"`                //tag过滤条件
	EnableStime         string    `json:"enable_stime"`                 //策略生效开始时间
	EnableEtime         string    `json:"enable_etime"`                 //策略生效终止时间 支持23:00-02:00
	EnableDaysOfWeekStr string    `xorm:"enable_days_of_week" json:"-"` //策略生效日期
	ConvergeStr         string    `xorm:"converge" json:"-"`            //告警通知收敛，第1个值表示收敛周期，单位秒，第2个值表示周期内允许发送告警次数
	Priority            int       `json:"priority"`
	Callback            string    `json:"callback"`
	NotifyGroupStr      string    `xorm:"notify_group" json:"-"`
	NotifyUserStr       string    `xorm:"notify_user" json:"-"`
	Creator             string    `json:"creator"`
	Created             time.Time `xorm:"created" json:"created"`
	LastUpdator         string    `xorm:"last_updator" json:"last_updator"`
	LastUpdated         time.Time `xorm:"<-" json:"last_updated"`
	NeedUpgrade         int       `xorm:"need_upgrade" json:"need_upgrade"`
	AlertUpgradeStr     string    `xorm:"alert_upgrade" json:"-"`
	WorkGroupsStr       string    `xorm:"work_groups" json:"-"`
	Runbook             string    `xorm:"runbook" json:"runbook"`

	ExclNid          []int64      `xorm:"-" json:"excl_nid"`
	Nids             []string     `xorm:"-" json:"nids"`
	Exprs            []Exp        `xorm:"-" json:"exprs"`
	Tags             []Tag        `xorm:"-" json:"tags"`
	EnableDaysOfWeek []int        `xorm:"-" json:"enable_days_of_week"`
	Converge         []int        `xorm:"-" json:"converge"`
	NotifyGroup      []int        `xorm:"-" json:"notify_group"`
	NotifyUser       []int        `xorm:"-" json:"notify_user"`
	LeafNids         []int64      `xorm:"-" json:"leaf_nids"` //叶子节点id
	Endpoints        []string     `xorm:"-" json:"endpoints"`
	AlertUpgrade     AlertUpgrade `xorm:"-" json:"alert_upgrade"`
	JudgeInstance    string       `xorm:"-" json:"judge_instance"`
	WorkGroups       []int        `xorm:"-" json:"work_groups"`
}

func (s *Stra) GetMetric() string {
	for _, e := range s.Exprs {
		return e.Metric
	}
	return ""
}

type StraLog struct {
	Id      int64     `json:"id"`
	Sid     int64     `json:"sid"`
	Action  string    `json:"action"` // update|delete
	Body    string    `json:"body"`
	Creator string    `json:"creator"`
	Created time.Time `json:"created" xorm:"created"`
}

type Exp struct {
	Eopt      string  `json:"eopt"`
	Func      string  `json:"func"`      //all,max,min
	Metric    string  `json:"metric"`    //metric
	Params    []int   `json:"params"`    //连续n秒
	Threshold float64 `json:"threshold"` //阈值
}

type Tag struct {
	Tkey string   `json:"tkey"`
	Topt string   `json:"topt"`
	Tval []string `json:"tval"` //修改为数组
}

type AlertUpgrade struct {
	Users    []int64 `json:"users"`
	Groups   []int64 `json:"groups"`
	Duration int     `json:"duration"`
	Level    int     `json:"level"`
}

var MathOperators = map[string]bool{
	">":  true,
	"<":  true,
	">=": true,
	"<=": true,
	"!=": true,
	"=":  true,
}

func (s *Stra) Save() error {
	session := DB["mon"].NewSession()
	defer session.Close()

	err := session.Begin()
	if err != nil {
		session.Rollback()
		return err
	}

	_, err = session.Insert(s)
	if err != nil {
		session.Rollback()
		return err
	}

	straByte, err := json.Marshal(s)
	if err != nil {
		session.Rollback()
		return err
	}

	err = SaveStraCommit(s.Id, "add", s.Creator, string(straByte), session)
	if err != nil {
		session.Rollback()
		return err
	}

	session.Commit()
	return nil
}

func (s *Stra) Update() error {
	var obj Stra

	session := DB["mon"].NewSession()
	defer session.Close()

	err := session.Begin()
	if err != nil {
		session.Rollback()
		return err
	}

	exists, err := session.Id(s.Id).Get(&obj)
	if err != nil {
		session.Rollback()
		return err
	}

	if !exists {
		session.Rollback()
		return fmt.Errorf("%d not exists", s.Id)
	}

	_, err = session.Id(s.Id).AllCols().Update(s)
	if err != nil {
		session.Rollback()
		return err
	}

	straByte, err := json.Marshal(s)
	if err != nil {
		session.Rollback()
		return err
	}

	err = SaveStraCommit(s.Id, "update", s.Creator, string(straByte), session)
	if err != nil {
		session.Rollback()
		return err
	}

	session.Commit()
	return nil
}

func StraGet(col string, val interface{}) (*Stra, error) {
	var obj Stra
	has, err := DB["mon"].Where(col+"=?", val).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func StraDel(id int64) error {
	session := DB["mon"].NewSession()
	defer session.Close()
	var obj Stra

	if err := session.Begin(); err != nil {
		return err
	}

	exists, err := session.Id(id).Get(&obj)
	if err != nil {
		session.Rollback()
		return err
	}

	if !exists {
		session.Rollback()
		return fmt.Errorf("%d not exists", obj.Id)
	}

	if _, err := session.Id(id).Delete(new(Stra)); err != nil {
		session.Rollback()
		return err
	}

	straByte, err := json.Marshal(obj)
	if err != nil {
		session.Rollback()
		return err
	}

	err = SaveStraCommit(obj.Id, "delete", obj.Creator, string(straByte), session)
	if err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func StraDelByNid(nid int64) error {
	_, err := DB["mon"].Where("nid=?", nid).Delete(new(Stra))
	return err
}

func StrasList(name string, priority int, nid int64) ([]*Stra, error) {
	session := DB["mon"].NewSession()
	defer session.Close()

	objs := make([]*Stra, 0)

	whereClause := "1 = 1"
	params := []interface{}{}

	if name != "" {
		whereClause += " AND name LIKE ?"
		params = append(params, "%"+name+"%")
	}

	if priority <= 3 {
		whereClause += " AND priority = ?"
		params = append(params, priority)
	}

	var err error
	if nid != 0 {
		err = session.Where(whereClause, params...).Where("nid=?", nid).Find(&objs)
	} else {
		err = session.Where(whereClause, params...).Find(&objs)
	}
	if err != nil {
		return objs, err
	}

	stras := make([]*Stra, 0)
	for _, obj := range objs {
		err = obj.Decode()
		if err != nil {
			return stras, err
		}
		stras = append(stras, obj)
	}
	return stras, err
}

func StrasAll() ([]*Stra, error) {
	objs := make([]*Stra, 0)

	err := DB["mon"].Find(&objs)
	if err != nil {
		return objs, err
	}

	stras := make([]*Stra, 0)
	for _, obj := range objs {
		err = obj.Decode()
		if err != nil {
			return stras, err
		}
		stras = append(stras, obj)
	}
	return stras, err
}

func EffectiveStrasList() ([]*Stra, error) {
	session := DB["mon"].NewSession()
	defer session.Close()

	objs := make([]*Stra, 0)
	t := time.Now()

	now := t.Format("15:04")
	weekday := strconv.Itoa(int(t.Weekday()))

	err := session.Where("((enable_stime <= ? and enable_etime >= ? or (enable_stime > enable_etime and !(enable_stime > ? and enable_etime < ?))) and enable_days_of_week like ?)", now, now, now, now, "%"+weekday+"%").Find(&objs)
	if err != nil {
		return objs, err
	}

	stras := make([]*Stra, 0)
	for _, obj := range objs {
		err = obj.Decode()
		if err != nil {
			return stras, err
		}
		stras = append(stras, obj)
	}
	return stras, err
}

func SaveStraCommit(id int64, action, username, body string, session *xorm.Session) error {
	strategyLog := StraLog{
		Sid:     id,
		Action:  action,
		Body:    body,
		Creator: username,
	}

	if _, err := session.Insert(&strategyLog); err != nil {
		session.Rollback()
		return err
	}

	return nil
}

func (s *Stra) HasPermssion() error {
	return nil
}

func (s *Stra) Encode() error {
	alertUpgrade, err := AlertUpgradeMarshal(s.AlertUpgrade)
	if err != nil {
		return fmt.Errorf("encode alert_upgrade err:%v", err)
	}

	if s.NeedUpgrade == 1 {
		if len(s.AlertUpgrade.Users) == 0 && len(s.AlertUpgrade.Groups) == 0 {
			return fmt.Errorf("alert upgrade: users and groups is blank")
		}
	}

	s.AlertUpgradeStr = alertUpgrade

	exclNid, err := json.Marshal(s.ExclNid)
	if err != nil {
		return fmt.Errorf("encode excl_nid err:%v", err)
	}
	s.ExclNidStr = string(exclNid)

	exprs, err := json.Marshal(s.Exprs)
	if err != nil {
		return fmt.Errorf("encode exprs err:%v", err)
	}
	s.ExprsStr = string(exprs)

	//校验exprs
	var exprsTmp []Exp
	err = json.Unmarshal(exprs, &exprsTmp)
	for _, exp := range exprsTmp {
		if _, found := MathOperators[exp.Eopt]; !found {
			return fmt.Errorf("unknown exp.eopt:%s", exp)
		}
	}

	tags, err := json.Marshal(s.Tags)
	if err != nil {
		return fmt.Errorf("encode Tags err:%v", err)
	}
	s.TagsStr = string(tags)

	workGroupsByte, err := json.Marshal(s.WorkGroups)
	if err != nil {
		return fmt.Errorf("encode work_group err:%v", err)
	}
	s.WorkGroupsStr = string(workGroupsByte)

	//校验tags
	var tagsTmp []Tag
	err = json.Unmarshal(tags, &tagsTmp)
	for _, tag := range tagsTmp {
		if tag.Topt != "=" && tag.Topt != "!=" {
			return fmt.Errorf("unknown tag.topt")
		}
	}

	//校验时间
	err = checkDurationString(s.EnableStime)
	if err != nil {
		return fmt.Errorf("unknown enable_stime: %s", s.EnableStime)
	}

	err = checkDurationString(s.EnableEtime)
	if err != nil {
		return fmt.Errorf("unknown enable_etime: %s", s.EnableEtime)
	}

	for _, day := range s.EnableDaysOfWeek {
		if day > 7 || day < 0 {
			return fmt.Errorf("illegal period_days_of_week %v", s.EnableDaysOfWeek)
		}
	}
	enableDaysOfWeek, err := json.Marshal(s.EnableDaysOfWeek)
	if err != nil {
		return fmt.Errorf("encode EnableDaysOfWeek err:%v", err)
	}
	s.EnableDaysOfWeekStr = string(enableDaysOfWeek)

	//校验收敛配置
	if len(s.Converge) != 2 {
		return fmt.Errorf("illegal converge %v", s.Converge)
	}
	Converge, err := json.Marshal(s.Converge)
	if err != nil {
		return err
	}
	s.ConvergeStr = string(Converge)

	notifyGroup, err := json.Marshal(s.NotifyGroup)
	if err != nil {
		return err
	}
	s.NotifyGroupStr = string(notifyGroup)

	notifyUser, err := json.Marshal(s.NotifyUser)
	if err != nil {
		return err
	}
	s.NotifyUserStr = string(notifyUser)

	return nil
}

func (s *Stra) Decode() error {
	var err error

	s.AlertUpgrade, err = AlertUpgradeUnMarshal(s.AlertUpgradeStr)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(s.ExclNidStr), &s.ExclNid)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(s.ExprsStr), &s.Exprs)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(s.TagsStr), &s.Tags)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(s.EnableDaysOfWeekStr), &s.EnableDaysOfWeek)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(s.ConvergeStr), &s.Converge)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(s.NotifyUserStr), &s.NotifyUser)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(s.NotifyGroupStr), &s.NotifyGroup)
	if err != nil {
		return err
	}

	if s.WorkGroupsStr != "" {
		err = json.Unmarshal([]byte(s.WorkGroupsStr), &s.WorkGroups)
		if err != nil {
			return err
		}
	}

	return nil
}

// 00:00-23:59
func checkDurationString(str string) error {
	slice := strings.Split(str, ":")
	if len(slice) != 2 {
		return fmt.Errorf("illegal duration", str)
	}

	hour, err := strconv.Atoi(slice[0])
	if err != nil {
		return fmt.Errorf("illegal duration", str)
	}
	if hour < 0 || hour > 23 {
		return fmt.Errorf("illegal duration", str)
	}
	minute, err := strconv.Atoi(slice[1])
	if err != nil {
		return fmt.Errorf("illegal duration", str)
	}
	if minute < 0 || minute > 59 {
		return fmt.Errorf("illegal duration", str)
	}

	return nil
}

func AlertUpgradeMarshal(alterUpgrade AlertUpgrade) (string, error) {
	dat := AlertUpgrade{
		Duration: alterUpgrade.Duration,
		Level:    alterUpgrade.Level,
	}

	if alterUpgrade.Duration == 0 {
		dat.Duration = 60
	}

	if alterUpgrade.Level == 0 {
		dat.Level = 1
	}

	if alterUpgrade.Groups == nil {
		dat.Groups = []int64{}
	} else {
		dat.Groups = alterUpgrade.Groups
	}

	if alterUpgrade.Users == nil {
		dat.Users = []int64{}
	} else {
		dat.Users = alterUpgrade.Users
	}

	data, err := json.Marshal(dat)
	return string(data), err
}

func AlertUpgradeUnMarshal(str string) (AlertUpgrade, error) {
	var obj AlertUpgrade
	if strings.TrimSpace(str) == "" {
		return AlertUpgrade{
			Users:    []int64{},
			Groups:   []int64{},
			Duration: 0,
			Level:    0,
		}, nil
	}

	err := json.Unmarshal([]byte(str), &obj)
	return obj, err
}
