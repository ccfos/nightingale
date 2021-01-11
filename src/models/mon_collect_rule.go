package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
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
	LastUpdator string          `xorm:"last_updator" json:"last_updator" description:"just for output"`
	Created     time.Time       `xorm:"updated" json:"created" description:"just for output"`
	LastUpdated time.Time       `xorm:"updated" json:"last_updated" description:"just for output"`
}

type validator interface {
	Validate() error
}

func (p CollectRule) PluginName() string {
	return p.CollectType
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
		if err := json.Unmarshal(p.Data, v[0]); err != nil {
			return err
		}
		if o, ok := v[0].(validator); ok {
			if err := o.Validate(); err != nil {
				return err
			}
		}
	}

	return nil
}

func GetCollectRules() ([]*CollectRule, error) {
	rules := []*CollectRule{}
	err := DB["mon"].Find(&rules)
	return rules, err
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

	if err := saveHist(p.Id, p.CollectType, "update", p.Creator, string(b), session); err != nil {
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
