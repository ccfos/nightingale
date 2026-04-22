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

type taskTplResult struct {
	Id       int64    `json:"id"`
	GroupId  int64    `json:"group_id"`
	Title    string   `json:"title"`
	Tags     []string `json:"tags,omitempty"`
	Account  string   `json:"account,omitempty"`
	CreateBy string   `json:"create_by,omitempty"`
}

type taskTplDetailResult struct {
	Id        int64    `json:"id"`
	GroupId   int64    `json:"group_id"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags,omitempty"`
	Batch     int      `json:"batch"`
	Tolerance int      `json:"tolerance"`
	Timeout   int      `json:"timeout"`
	Account   string   `json:"account,omitempty"`
	Args      string   `json:"args,omitempty"`
	Script    string   `json:"script,omitempty"`
	CreateBy  string   `json:"create_by,omitempty"`
	UpdateBy  string   `json:"update_by,omitempty"`
}

func init() {
	register(defs.ListTaskTpls, listTaskTpls)
	register(defs.GetTaskTplDetail, getTaskTplDetail)
}

func listTaskTpls(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermJobTpls); err != nil {
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

	var tpls []models.TaskTpl
	if isAdmin {
		tpls, err = models.TaskTplGets(deps.DBCtx, nil, query, limit, 0)
	} else {
		if len(bgids) == 0 {
			return marshalList(0, []taskTplResult{}), nil
		}
		tpls, err = models.TaskTplGets(deps.DBCtx, bgids, query, limit, 0)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query task templates: %v", err)
	}

	results := make([]taskTplResult, 0, len(tpls))
	for _, t := range tpls {
		results = append(results, taskTplResult{
			Id:       t.Id,
			GroupId:  t.GroupId,
			Title:    t.Title,
			Tags:     t.TagsJSON,
			Account:  t.Account,
			CreateBy: t.CreateBy,
		})
	}

	logger.Debugf("list_task_tpls: user_id=%d, query=%s, found %d templates", user.Id, query, len(results))
	return marshalList(len(results), results), nil
}

func getTaskTplDetail(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermJobTpls); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	tpl, err := models.TaskTplGetById(deps.DBCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get task template: %v", err)
	}
	if tpl == nil {
		return fmt.Sprintf(`{"error":"task template not found: id=%d"}`, id), nil
	}

	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		if !int64SliceContains(bgids, tpl.GroupId) {
			return "", fmt.Errorf("forbidden: no access to this task template")
		}
	}

	result := taskTplDetailResult{
		Id:        tpl.Id,
		GroupId:   tpl.GroupId,
		Title:     tpl.Title,
		Tags:      tpl.TagsJSON,
		Batch:     tpl.Batch,
		Tolerance: tpl.Tolerance,
		Timeout:   tpl.Timeout,
		Account:   tpl.Account,
		Args:      tpl.Args,
		Script:    tpl.Script,
		CreateBy:  tpl.CreateBy,
		UpdateBy:  tpl.UpdateBy,
	}

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
