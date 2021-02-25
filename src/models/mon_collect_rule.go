package models

import (
	"encoding/json"
	"fmt"

	"github.com/didi/nightingale/src/common/dataobj"
	"xorm.io/xorm"
)

const (
	defaultStep = 10
)

type CollectRule struct {
	Id          int64           `json:"id"`
	Nid         int64           `json:"nid"`
	Step        int64           `json:"step" description:"interval"`
	Timeout     int             `json:"timeout"`
	CollectType string          `json:"collect_type" description:"plugin name"`
	Name        string          `json:"name" describes:"customize name"`
	Region      string          `json:"region"`
	Comment     string          `json:"comment"`
	Data        json.RawMessage `json:"data"`
	Tags        string          `json:"tags" description:"k1=v1,k2=v2,k3=v3,..."`
	Creator     string          `json:"creator" description:"just for output"`
	Updater     string          `json:"updater" description:"just for output"`
	CreatedAt   int64           `json:"created_at" description:"just for output"`
	UpdatedAt   int64           `json:"updated_at" description:"just for output"`
}

type validator interface {
	Validate() error
}

func (p CollectRule) PluginName() string {
	return p.CollectType
}

func (p *CollectRule) String() string {
	return fmt.Sprintf("id %d type %s name %s", p.Id, p.CollectType, p.Name)
}

func (p *CollectRule) Validate(v ...interface{}) error {
	if p.Name == "" {
		return fmt.Errorf("invalid collectRule.name")
	}

	if p.Step == 0 {
		p.Step = defaultStep
	}

	if _, err := dataobj.SplitTagsString(p.Tags); err != nil {
		return err
	}

	if len(v) > 0 && v[0] != nil {
		obj := v[0]
		if err := json.Unmarshal(p.Data, obj); err != nil {
			return err
		}
		if o, ok := obj.(validator); ok {
			if err := o.Validate(); err != nil {
				return err
			}
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		p.Data = json.RawMessage(b)
	}

	return nil
}

func DumpCollectRules() ([]*CollectRule, error) {
	rules := []*CollectRule{}
	err := DB["mon"].Find(&rules)
	return rules, err
}

func GetCollectRules(typ string, nid int64, limit, offset int) (total int64, list []*CollectRule, err error) {
	search := func() *xorm.Session {
		session := DB["mon"].Where("1=1")
		if nid != 0 {
			session = session.And("nid=?", nid)
		}
		if typ != "" {
			return session.And("collect_type=?", typ)
		}
		return session
	}

	if total, err = search().Count(new(CollectRule)); err != nil {
		return
	}

	err = search().Desc("updated_at").Limit(limit, offset).Find(&list)
	return
}

func (p *CollectRule) Update() error {
	session := DB["mon"].NewSession()
	defer session.Close()

	err := session.Begin()
	if err != nil {
		return err
	}

	if _, err = session.Id(p.Id).AllCols().Update(p); err != nil {
		session.Rollback()
		return err
	}

	b, err := json.Marshal(p)
	if err != nil {
		session.Rollback()
		return err
	}

	if err := saveHistory(p.Id, p.CollectType, "update", p.Creator, string(b), session); err != nil {
		session.Rollback()
		return err
	}

	if err = session.Commit(); err != nil {
		return err
	}

	return err
}

func DeleteCollectRule(sid int64) error {
	_, err := DB["mon"].Where("id=?", sid).Delete(new(CollectRule))
	return err
}
