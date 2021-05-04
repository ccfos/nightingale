package models

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"xorm.io/xorm"
)

type Module struct {
	// A list of OIDs.
	Walk       []string   `yaml:"walk,omitempty"`
	Get        []string   `yaml:"get,omitempty"`
	Metrics    []*Metric  `yaml:"metrics"`
	WalkParams WalkParams `yaml:",inline"`
}

type WalkParams struct {
	Version        int           `yaml:"version,omitempty"`
	MaxRepetitions uint8         `yaml:"max_repetitions,omitempty"`
	Retries        int           `yaml:"retries,omitempty"`
	Timeout        time.Duration `yaml:"timeout,omitempty"`
	Auth           Auth          `yaml:"auth,omitempty"`
}

type Metric struct {
	Name           string                     `yaml:"name"`
	Oid            string                     `yaml:"oid"`
	Type           string                     `yaml:"type"`
	Help           string                     `yaml:"help"`
	Indexes        []*Index                   `yaml:"indexes,omitempty"`
	Lookups        []*Lookup                  `yaml:"lookups,omitempty"`
	RegexpExtracts map[string][]RegexpExtract `yaml:"regex_extracts,omitempty"`
	EnumValues     map[int]string             `yaml:"enum_values,omitempty"`
}

type RegexpExtract struct {
	Value string `yaml:"value"`
	Regex Regexp `yaml:"regex"`
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshalable.
type Regexp struct {
	*regexp.Regexp
}

type Mib struct {
	Id         int64  `json:"id"`
	Module     string `json:"module"`
	Metric     string `json:"metric"`
	Oid        string `json:"oid"`
	Mtype      string `json:"mtype"` //gauge,counter
	EnumValues string `json:"enum_values"`
	Indexes    string `json:"indexes"`
	Note       string `json:"note"`
}

func NewMib(module string, m *Metric) *Mib {
	enumValues, _ := json.Marshal(m.EnumValues)
	indexes, _ := json.Marshal(m.Indexes)

	mib := &Mib{
		Module:     module,
		Metric:     m.Name,
		Oid:        m.Oid,
		Mtype:      m.Type,
		EnumValues: string(enumValues),
		Indexes:    string(indexes),
		Note:       m.Help,
	}
	return mib
}

func (m *Mib) Save() error {
	_, err := DB["nems"].InsertOne(m)
	return err
}

func MibDel(id int64) error {
	_, err := DB["nems"].Where("id=?", id).Delete(new(Mib))
	return err
}

func MibTotal(query string) (int64, error) {
	return buildMibWhere(query).Count()
}

func MibGet(where string, args ...interface{}) (*Mib, error) {
	var obj Mib
	has, err := DB["nems"].Where(where, args...).Get(&obj)
	if !has {
		return nil, err
	}

	return &obj, err
}

func MibGets(where string, args ...interface{}) ([]Mib, error) {
	var objs []Mib
	err := DB["nems"].Where(where, args...).Find(&objs)
	return objs, err
}

func MibGetsGroupBy(group string, where string, args ...interface{}) ([]Mib, error) {
	var objs []Mib
	var err error
	if where == "" {
		err = DB["nems"].GroupBy(group).Find(&objs)
	} else {
		err = DB["nems"].Where(where, args...).GroupBy(group).Find(&objs)
	}
	return objs, err
}

func MibGetsByQuery(query string, limit, offset int) ([]Mib, error) {
	session := buildMibWhere(query)
	var objs []Mib
	err := session.Limit(limit, offset).Find(&objs)
	return objs, err
}

func buildMibWhere(query string) *xorm.Session {
	session := DB["nems"].Table(new(Mib))
	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			q := "%" + arr[i] + "%"
			session = session.Where("module like ? or oid like ? or metric like ?", q, q, q)
		}
	}
	return session
}
