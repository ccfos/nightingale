package router

import (
	"reflect"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// TestApplyHeartbeatFields: like gorm's Updates(struct), only the non-zero
// fields of the update reach the cached target.
func TestApplyHeartbeatFields(t *testing.T) {
	cached := &models.Target{
		Ident: "host-1", HostIp: "10.0.0.1", HostTags: []string{"old=1"}, Tags: "env=prod ",
		EngineName: "default", AgentVersion: "v1", OS: "linux", UpdateAt: 100,
	}

	applyHeartbeatFields(cached, &models.Target{AgentVersion: "v2", HostTags: []string{}, UpdateAt: 200})

	want := &models.Target{
		Ident: "host-1", HostIp: "10.0.0.1", HostTags: []string{}, Tags: "env=prod ",
		EngineName: "default", AgentVersion: "v2", OS: "linux", UpdateAt: 200,
	}
	if !reflect.DeepEqual(cached, want) {
		t.Fatalf("cached = %+v, want %+v", cached, want)
	}
}
