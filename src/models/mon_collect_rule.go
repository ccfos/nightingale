package models

import (
	"encoding/json"
	"fmt"
	"time"
)

type CollectRule struct {
	Id          int64           `json:"id"`
	Nid         int64           `json:"nid"`
	Step        int             `json:"step" description:"interval"`
	Timeout     int             `json:"timeout"`
	CollectType string          `json:"collect_type" description:"just for output"`
	Name        string          `json:"name"`
	Region      string          `json:"region"`
	Comment     string          `json:"comment"`
	Data        json.RawMessage `json:"data"`
	Creator     string          `json:"creator" description:"just for output"`
	LastUpdator string          `xorm:"last_updator" json:"last_updator" description:"just for output"`
	Created     time.Time       `xorm:"updated" json:"created" description:"just for output"`
	LastUpdated time.Time       `xorm:"updated" json:"last_updated" description:"just for output"`
}

type validator interface {
	Validate() error
}

func (p *CollectRule) Validate(v interface{}) error {
	if p.Name == "" {
		return fmt.Errorf("invalid collectRule.name")
	}

	if v != nil {
		if err := json.Unmarshal(p.Data, v); err != nil {
			return err
		}
		if o, ok := v.(validator); ok {
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
