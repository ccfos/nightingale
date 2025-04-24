package flashduty

import (
	"errors"
	"strconv"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

type UserGroupSyncer struct {
	ctx    *ctx.Context
	ug     *models.UserGroup
	appKey string
	teamID int64
}

func NewUserGroupSyncer(ctx *ctx.Context, ug *models.UserGroup) (*UserGroupSyncer, error) {
	appKey, err := models.ConfigsGetFlashDutyAppKey(ctx)
	if err != nil {
		return nil, err
	}

	return &UserGroupSyncer{
		ctx:    ctx,
		ug:     ug,
		appKey: appKey,
	}, nil
}

func (ugs *UserGroupSyncer) SyncUGAdd() error {
	// 新建团队(无用户仅有团队名称)
	fdt := Team{
		TeamName: ugs.ug.Name,
		RefID:    strconv.FormatInt(ugs.ug.Id, 10),
	}
	err := fdt.UpdateTeam(ugs.appKey)
	if err != nil {
		return err
	}
	return ugs.syncTeamMember()
}

func (ugs *UserGroupSyncer) SyncUGPut() error {
	// 修改为查询 ref_ID
	refID := strconv.FormatInt(ugs.ug.Id, 10)
	teamID, err := ugs.CheckTeam(refID)
	// 如果没有找到团队，说明是新建的团队
	ugs.teamID = teamID
	if err != nil && strings.Contains(err.Error(), "no team found by ref_id") {
		emails := make([]string, 0)
		phones := make([]string, 0)

		for _, user := range ugs.ug.Users {
			if user.Email != "" {
				emails = append(emails, user.Email)
			} else if user.Phone != "" {
				phones = append(phones, user.Phone)
			} else {
				logger.Warningf("The user %s has no email and phone, and failed to sync to flashduty's team", user.Username)
			}
		}
		//根据 team_id 去更新 duty 中这个团队的信息
		fdt := Team{
			RefID:    refID,
			TeamName: ugs.ug.Name,
			Emails:   emails,
			Phones:   phones,
		}
		if err := fdt.AddTeam(ugs.appKey); err != nil {
			return err
		}

		if err := ugs.syncTeamMember(); err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	emails := make([]string, 0)
	phones := make([]string, 0)

	for _, user := range ugs.ug.Users {
		if user.Email != "" {
			emails = append(emails, user.Email)
		} else if user.Phone != "" {
			phones = append(phones, user.Phone)
		} else {
			logger.Warningf("The user %s has no email and phone, and failed to sync to flashduty's team", user.Username)
		}
	}
	//根据 team_id 去更新 duty 中这个团队的信息
	fdt := Team{
		TeamID:   teamID,
		RefID:    refID,
		TeamName: ugs.ug.Name,
		Emails:   emails,
		Phones:   phones,
	}

	if err := fdt.UpdateTeam(ugs.appKey); err != nil {
		return err
	}
	if err := ugs.syncTeamMember(); err != nil {
		return err
	}
	return nil
}

func (ugs *UserGroupSyncer) SyncUGDel() error {
	fdt := Team{
		RefID: strconv.FormatInt(ugs.ug.Id, 10),
	}
	err := fdt.DelTeam(ugs.appKey)
	return err
}

func (ugs *UserGroupSyncer) SyncMembersAdd() error {
	return ugs.syncTeamMember()
}

func (ugs *UserGroupSyncer) SyncMembersDel() error {
	return ugs.syncTeamMember()
}

func (ugs *UserGroupSyncer) syncTeamMember() error {
	uids, err := models.MemberIds(ugs.ctx, ugs.ug.Id)
	if err != nil {
		return err
	}
	users, err := models.UserGetsByIds(ugs.ctx, uids)
	if err != nil {
		return err
	}

	toDutyErr := ugs.addMemberToFDTeam(users)
	if toDutyErr != nil {
		logger.Warningf("failed to sync user group %s %v to flashduty's team: %v", ugs.ug.Name, users, toDutyErr)
	}

	return err
}

func (ugs *UserGroupSyncer) addMemberToFDTeam(users []models.User) error {
	if err := fdAddUsers(ugs.appKey, users); err != nil {
		return err
	}

	emails := make([]string, 0)
	phones := make([]string, 0)
	for _, user := range users {
		if user.Email != "" {
			emails = append(emails, user.Email)
		} else if user.Phone != "" {
			phones = append(phones, user.Phone)
		} else {
			logger.Warningf("The user %s has no email and phone, and failed to sync to flashduty's team", user.Username)
		}
	}
	teamID := ugs.teamID
	refID := strconv.FormatInt(ugs.ug.Id, 10)
	var err error
	if teamID == 0 {
		teamID, err = ugs.CheckTeam(refID)
		if err != nil {
			logger.Warningf("CheckTeam failed for refID=%v: %v", refID, err)
		}
	}

	fdt := Team{
		TeamID:   teamID,
		TeamName: ugs.ug.Name,
		Emails:   emails,
		Phones:   phones,
		RefID:    refID,
	}
	err = fdt.UpdateTeam(ugs.appKey)
	return err
}

type Team struct {
	TeamID           int64    `json:"team_id"`
	TeamName         string   `json:"team_name"`
	ResetIfNameExist bool     `json:"reset_if_name_exist"`
	Description      string   `json:"description"`
	Emails           []string `json:"emails"`
	Phones           []string `json:"phones"`
	RefID            string   `json:"ref_id"`
}

func (t *Team) AddTeam(appKey string) error {
	if t.TeamName == "" {
		return errors.New("team_name must be set")
	}
	return PostFlashDuty("/team/upsert", appKey, t)
}

func (t *Team) UpdateTeam(appKey string) error {
	t.ResetIfNameExist = true
	err := t.AddTeam(appKey)
	return err
}

func (t *Team) DelTeam(appKey string) error {
	err := PostFlashDuty("/team/delete", appKey, t)
	return err
}

func NeedSyncTeam(ctx *ctx.Context) bool {
	configs, err := models.ConfigsSelectByCkey(ctx, "flashduty_sync_team")
	if err != nil {
		logger.Warningf("failed to query flashduty_sync_team: %v", err)
		return false
	}

	if len(configs) == 0 || configs[0].Cval == "" {
		return false
	}

	return configs[0].Cval == "true"
}

func NeedSyncUser(ctx *ctx.Context) bool {
	configs, err := models.ConfigsSelectByCkey(ctx, "flashduty_app_key")
	if err != nil {
		logger.Warningf("failed to query flashduty_app_key: %v", err)
		return false
	}

	if len(configs) == 0 || configs[0].Cval == "" {
		return false
	}

	return true
}

// CheckTeam 检查ref_id是否存在
func (ugs *UserGroupSyncer) CheckTeam(ref_id string) (int64, error) {
	// Construct the request to query the team by name
	info, err := PostFlashDutyWithResp[TeamInfo]("/team/info", ugs.appKey, map[string]interface{}{
		"ref_id": ref_id,
	})
	if err != nil || info.TeamID == 0 {
		return 0, err
	}

	return info.TeamID, nil
}
