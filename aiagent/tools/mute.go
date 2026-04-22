package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type alertMuteResult struct {
	Id       int64  `json:"id"`
	GroupId  int64  `json:"group_id"`
	Cause    string `json:"cause,omitempty"`
	Disabled int    `json:"disabled"`
	Btime    string `json:"btime"`
	Etime    string `json:"etime"`
	CreateBy string `json:"create_by,omitempty"`
}

type alertMuteDetailResult struct {
	Id       int64       `json:"id"`
	GroupId  int64       `json:"group_id"`
	Note     string      `json:"note,omitempty"`
	Cause    string      `json:"cause,omitempty"`
	Cate     string      `json:"cate,omitempty"`
	Tags     interface{} `json:"tags,omitempty"`
	Disabled int         `json:"disabled"`
	Btime    string      `json:"btime"`
	Etime    string      `json:"etime"`
	CreateBy string      `json:"create_by,omitempty"`
	UpdateBy string      `json:"update_by,omitempty"`
}

func init() {
	register(defs.ListAlertMutes, listAlertMutes)
	register(defs.GetAlertMuteDetail, getAlertMuteDetail)
}

func listAlertMutes(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertMutes); err != nil {
		return "", err
	}

	bgids, isAdmin, err := getUserBgids(deps, user)
	if err != nil {
		return "", err
	}

	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	var mutes []models.AlertMute
	if isAdmin {
		mutes, err = models.AlertMuteGetsByBGIds(deps.DBCtx, nil)
	} else {
		if len(bgids) == 0 {
			return marshalList(0, []alertMuteResult{}), nil
		}
		mutes, err = models.AlertMuteGetsByBGIds(deps.DBCtx, bgids)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query alert mutes: %v", err)
	}

	results := make([]alertMuteResult, 0)
	for _, m := range mutes {
		if query != "" && !containsIgnoreCase(m.Cause, query) {
			continue
		}
		results = append(results, alertMuteResult{
			Id:       m.Id,
			GroupId:  m.GroupId,
			Cause:    m.Cause,
			Disabled: m.Disabled,
			Btime:    formatUnixTime(m.Btime),
			Etime:    formatUnixTime(m.Etime),
			CreateBy: m.CreateBy,
		})
		if len(results) >= limit {
			break
		}
	}

	logger.Debugf("list_alert_mutes: user_id=%d, found %d mutes", user.Id, len(results))
	return marshalList(len(results), results), nil
}

func getAlertMuteDetail(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertMutes); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	mute, err := models.AlertMuteGetById(deps.DBCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get alert mute: %v", err)
	}
	if mute == nil {
		return fmt.Sprintf(`{"error":"alert mute not found: id=%d"}`, id), nil
	}

	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		if !int64SliceContains(bgids, mute.GroupId) {
			return "", fmt.Errorf("forbidden: no access to this alert mute")
		}
	}

	result := alertMuteDetailResult{
		Id:       mute.Id,
		GroupId:  mute.GroupId,
		Note:     mute.Note,
		Cause:    mute.Cause,
		Cate:     mute.Cate,
		Tags:     mute.Tags,
		Disabled: mute.Disabled,
		Btime:    formatUnixTime(mute.Btime),
		Etime:    formatUnixTime(mute.Etime),
		CreateBy: mute.CreateBy,
		UpdateBy: mute.UpdateBy,
	}

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
