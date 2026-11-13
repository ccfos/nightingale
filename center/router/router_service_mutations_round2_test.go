package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestTargetUnbindTagsByService_ResolvesHostIPAndReportsMissingTargets(t *testing.T) {
	rt := targetMutationRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	seedTargetMutationHosts(t, rt)
	body := targetTagsForm{HostIps: []string{"10.0.0.1"}, Idents: []string{"missing"}, Tags: []string{"env=prod"}}

	w := targetMutationRequest(t, rt.targetUnbindTagsByService, body, nil)

	failed, message := targetMutationReply(t, w)
	require.Empty(t, message)
	require.Contains(t, failed, "missing")
	var hosts []models.Target
	require.NoError(t, rt.Ctx.DB.Order("ident").Find(&hosts).Error)
	require.Empty(t, hosts[0].Tags)
	require.Equal(t, "region=us ", hosts[1].Tags)
}

func TestTargetUpdateNoteByService_UpdatesResolvedHostsOnly(t *testing.T) {
	rt := targetMutationRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	seedTargetMutationHosts(t, rt)
	body := targetNoteForm{HostIps: []string{"10.0.0.1"}, Idents: []string{"missing"}, Note: "updated"}

	w := targetMutationRequest(t, rt.targetUpdateNoteByService, body, nil)

	failed, message := targetMutationReply(t, w)
	require.Empty(t, message)
	require.Contains(t, failed, "missing")
	var hosts []models.Target
	require.NoError(t, rt.Ctx.DB.Order("ident").Find(&hosts).Error)
	require.Equal(t, "updated", hosts[0].Note)
	require.Equal(t, "original", hosts[1].Note)
}

func TestTargetUpdateBgidByService_ReplacesOnlyResolvedHostsBindings(t *testing.T) {
	rt := targetMutationRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	seedTargetMutationHosts(t, rt)
	seedTargetMutationBindings(t, rt)
	body := targetBgidForm{HostIps: []string{"10.0.0.1"}, Idents: []string{"missing"}, Bgid: 7}

	w := targetMutationRequest(t, rt.targetUpdateBgidByService, body, nil)

	failed, message := targetMutationReply(t, w)
	require.Empty(t, message)
	require.Contains(t, failed, "missing")
	var bindings []models.TargetBusiGroup
	require.NoError(t, rt.Ctx.DB.Order("target_ident").Find(&bindings).Error)
	require.Len(t, bindings, 2)
	require.Equal(t, "host-a", bindings[0].TargetIdent)
	require.Equal(t, int64(7), bindings[0].GroupId)
	require.Equal(t, "host-b", bindings[1].TargetIdent)
	require.Equal(t, int64(2), bindings[1].GroupId)
}
