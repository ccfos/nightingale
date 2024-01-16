package flashduty

import (
	"errors"
	"github.com/ccfos/nightingale/v6/pkg/poster"
)

type Team struct {
	TeamName         string   `json:"team_name"`
	ResetIfNameExist bool     `json:"reset_if_name_exist"`
	Description      string   `json:"description"`
	Emails           []string `json:"emails"`
	Phones           []string `json:"phones"`
}

func (t *Team) AddTeam(appKey string) error {
	if t.TeamName == "" {
		return errors.New("team_name must be set")
	}
	_, _, err := poster.PostFlashDuty("/team/upsert", appKey, t)
	return err
}

func (t *Team) UpdateTeam(appKey string) error {
	t.ResetIfNameExist = true
	err := t.AddTeam(appKey)
	return err
}

func (t *Team) DelTeam(appKey string) error {
	if t.TeamName == "" {
		return errors.New("team_name must be set")
	}
	_, _, err := poster.PostFlashDuty("/team/delete", appKey, t)
	return err
}
