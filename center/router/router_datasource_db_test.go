package router

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
)

func withMetaHook(t *testing.T, h DatasourceMetaHookFunc) {
	t.Helper()
	prev := DatasourceMetaHook
	DatasourceMetaHook = h
	t.Cleanup(func() { DatasourceMetaHook = prev })
}

func TestDatasourceMetaHook_DefaultPassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	got, err := DatasourceMetaHook(c, &models.QueryParam{}, []string{"a", "b"})
	if err != nil {
		t.Fatalf("default hook must not return error, got %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("default hook must return response as-is, got %v", got)
	}
}

func TestDatasourceMetaHook_InjectedFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	withMetaHook(t, func(c *gin.Context, req *models.QueryParam, resp []string) ([]string, error) {
		filtered := make([]string, 0, len(resp))
		for _, d := range resp {
			if d != "secret" {
				filtered = append(filtered, d)
			}
		}
		return filtered, nil
	})

	got, err := DatasourceMetaHook(c, &models.QueryParam{}, []string{"public", "secret", "shared"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "public" || got[1] != "shared" {
		t.Fatalf("filter did not apply, got %v", got)
	}
}

func TestDatasourceMetaHook_InjectedDeny(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	wantErr := errors.New("no permission for db.tbl")
	withMetaHook(t, func(c *gin.Context, req *models.QueryParam, resp []string) ([]string, error) {
		return nil, wantErr
	})

	got, err := DatasourceMetaHook(c, &models.QueryParam{}, []string{"any"})
	if err == nil || err.Error() != wantErr.Error() {
		t.Fatalf("want deny error %q, got err=%v", wantErr, err)
	}
	if got != nil {
		t.Fatalf("deny path must return nil payload, got %v", got)
	}
}
