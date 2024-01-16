package flashduty

import (
	"errors"
	"github.com/ccfos/nightingale/v6/center/cconf"
)

type Team struct {
	TeamName         string   `json:"team_name"`
	ResetIfNameExist bool     `json:"reset_if_name_exist"`
	Description      string   `json:"description"`
	Emails           []string `json:"emails"`
	Phones           []string `json:"phones"`
}

func (t *Team) AddTeam(fdConf *cconf.FlashDuty, appKey string) error {
	if t.TeamName == "" {
		return errors.New("team_name must be set")
	}
	_, _, err := PostFlashDuty(fdConf.Api, "/team/upsert", fdConf.Timeout, appKey, t)
	return err
}

func (t *Team) UpdateTeam(fdConf *cconf.FlashDuty, appKey string) error {
	t.ResetIfNameExist = true
	err := t.AddTeam(fdConf, appKey)
	return err
}

func (t *Team) DelTeam(fdConf *cconf.FlashDuty, appKey string) error {
	if t.TeamName == "" {
		return errors.New("team_name must be set")
	}
	_, _, err := PostFlashDuty(fdConf.Api, "/team/delete", fdConf.Timeout, appKey, t)
	return err
}
