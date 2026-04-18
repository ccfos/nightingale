package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

type targetResult struct {
	Id           int64    `json:"id"`
	Ident        string   `json:"ident"`
	Note         string   `json:"note,omitempty"`
	HostIp       string   `json:"host_ip,omitempty"`
	OS           string   `json:"os,omitempty"`
	AgentVersion string   `json:"agent_version,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

type targetDetailResult struct {
	Id           int64    `json:"id"`
	Ident        string   `json:"ident"`
	Note         string   `json:"note,omitempty"`
	HostIp       string   `json:"host_ip,omitempty"`
	OS           string   `json:"os,omitempty"`
	AgentVersion string   `json:"agent_version,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	HostTags     []string `json:"host_tags,omitempty"`
	GroupIds     []int64  `json:"group_ids,omitempty"`
	UpdateAt     int64    `json:"update_at"`
}

func init() {
	register(defs.ListTargets, listTargets)
	register(defs.GetTargetDetail, getTargetDetail)
}

func listTargets(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	// targets listing has no perm middleware in router

	bgids, isAdmin, err := getUserBgids(deps, user)
	if err != nil {
		return "", err
	}

	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	var options []models.BuildTargetWhereOption
	if !isAdmin {
		if len(bgids) == 0 {
			return marshalList(0, []targetResult{}), nil
		}
		options = append(options, models.BuildTargetWhereWithBgids(bgids))
	}
	if query != "" {
		options = append(options, models.BuildTargetWhereWithQuery(query))
	}

	targets, err := models.TargetGets(deps.DBCtx, limit, 0, "ident", false, options...)
	if err != nil {
		return "", fmt.Errorf("failed to query targets: %v", err)
	}

	results := make([]targetResult, 0, len(targets))
	for _, t := range targets {
		results = append(results, targetResult{
			Id:           t.Id,
			Ident:        t.Ident,
			Note:         t.Note,
			HostIp:       t.HostIp,
			OS:           t.OS,
			AgentVersion: t.AgentVersion,
			Tags:         strings.Fields(t.Tags),
		})
	}

	logger.Debugf("list_targets: user_id=%d, query=%s, found %d targets", user.Id, query, len(results))
	return marshalList(len(results), results), nil
}

func getTargetDetail(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	ident := getArgString(args, "ident")

	var target *models.Target
	if id > 0 {
		target, err = models.TargetGetById(deps.DBCtx, id)
	} else if ident != "" {
		target, err = models.TargetGet(deps.DBCtx, "ident=?", ident)
	} else {
		return "", fmt.Errorf("id or ident is required")
	}
	if err != nil {
		return "", fmt.Errorf("failed to get target: %v", err)
	}
	if target == nil {
		return `{"error":"target not found"}`, nil
	}

	// Check data-level permission
	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		targetGids, _ := models.TargetGroupIdsGetByIdent(deps.DBCtx, target.Ident)
		if !int64SlicesOverlap(bgids, targetGids) {
			return "", fmt.Errorf("forbidden: no access to this target")
		}
	}

	result := targetDetailResult{
		Id:           target.Id,
		Ident:        target.Ident,
		Note:         target.Note,
		HostIp:       target.HostIp,
		OS:           target.OS,
		AgentVersion: target.AgentVersion,
		Tags:         strings.Fields(target.Tags),
		HostTags:     target.HostTags,
		UpdateAt:     target.UpdateAt,
	}
	result.GroupIds, _ = models.TargetGroupIdsGetByIdent(deps.DBCtx, target.Ident)

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
