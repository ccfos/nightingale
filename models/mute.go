package models

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
)

type Mute struct {
	Id         int64             `json:"id"`
	Classpath  string            `json:"classpath"`
	Metric     string            `json:"metric"`
	ResFilters string            `json:"res_filters"`
	TagFilters string            `json:"tags_filters"`
	Cause      string            `json:"cause"`
	Btime      int64             `json:"btime"`
	Etime      int64             `json:"etime"`
	CreateBy   string            `json:"create_by"`
	CreateAt   int64             `json:"create_at"`
	ResRegexp  *regexp.Regexp    `xorm:"-" json:"-"`
	TagsMap    map[string]string `xorm:"-" json:"-"`
}

func (m *Mute) TableName() string {
	return "mute"
}

func (m *Mute) Parse() error {
	var err error
	if m.ResFilters != "" {
		m.ResRegexp, err = regexp.Compile(m.ResFilters)
		if err != nil {
			return err
		}
	}

	if m.TagFilters != "" {
		tags := strings.Fields(m.TagFilters)
		m.TagsMap = make(map[string]string)
		for i := 0; i < len(tags); i++ {
			pair := strings.Split(tags[i], "=")
			if len(pair) != 2 {
				return fmt.Errorf("tagfilters format error")
			}
			m.TagsMap[pair[0]] = pair[1]
		}
	}

	return nil
}

func (m *Mute) Validate() error {
	m.Metric = strings.TrimSpace(m.Metric)
	m.ResFilters = strings.TrimSpace(m.ResFilters)
	m.TagFilters = strings.TrimSpace(m.TagFilters)
	return m.Parse()
}

func (m *Mute) Add() error {
	if err := m.Validate(); err != nil {
		return err
	}
	m.CreateAt = time.Now().Unix()
	return DBInsertOne(m)
}

func (m *Mute) Del() error {
	_, err := DB.Where("id=?", m.Id).Delete(new(Mute))
	if err != nil {
		logger.Errorf("mysql.error: delete mute(id=%d) fail: %v", m.Id, err)
		return internalServerError
	}
	return nil
}

func MuteGet(where string, args ...interface{}) (*Mute, error) {
	var obj Mute
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query mute(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func MuteTotal(query string) (num int64, err error) {
	if query != "" {
		q := "%" + query + "%"
		num, err = DB.Where("metric like ? or cause like ? or res_filters like ? or tag_filters like ?", q, q, q, q).Count(new(Mute))
	} else {
		num, err = DB.Count(new(Mute))
	}

	if err != nil {
		logger.Errorf("mysql.error: count mute(query: %s) fail: %v", query, err)
		return num, internalServerError
	}

	return num, nil
}

func MuteGets(query string, limit, offset int) ([]Mute, error) {
	session := DB.Limit(limit, offset).OrderBy("metric")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("metric like ? or cause like ? or res_filters like ? or tag_filters like ?", q, q, q, q)
	}

	var objs []Mute
	err := session.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: select mute(query: %s) fail: %v", query, err)
		return objs, internalServerError
	}

	if len(objs) == 0 {
		return []Mute{}, nil
	}

	return objs, nil
}

func MuteGetsAll() ([]Mute, error) {
	var objs []Mute

	err := DB.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: get all mute fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []Mute{}, nil
	}

	return objs, nil
}

// MuteCleanExpire 这个方法应该由cron调用，所以返回error不需要是用户友好的
func MuteCleanExpire() error {
	_, err := DB.Where("etime < unix_timestamp(now())").Delete(new(Mute))
	if err != nil {
		logger.Errorf("mysql.error: MuteCleanExpire fail: %v", err)
	}
	return err
}
