package victorialogs

import (
	"context"
	"testing"
)

var v = VictoriaLogsClient{
	Url:      "http://192.168.31.231:9428",
	User:     "",
	Password: "",
}

func TestVictoriaLogs_InitCli(t *testing.T) {
	if err := v.InitCli(); err != nil {
		t.Fatalf("InitCli failed: %v", err)
	}
}

func TestVictoriaLogs_QueryLogs(t *testing.T) {
	ctx := context.Background()
	if err := v.InitCli(); err != nil {
		t.Fatalf("InitCli failed: %v", err)
	}
	data, err := v.QueryLogs(ctx, &QueryParam{
		Query: "*",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	t.Logf("QueryLogs data: %v, Length: %d", data, len(data))
}

func TestVictoriaLogs_QueryLogs_Count(t *testing.T) {
	ctx := context.Background()
	if err := v.InitCli(); err != nil {
		t.Fatalf("InitCli failed: %v", err)
	}
	count, err := v.HitsLogs(ctx, &QueryParam{
		Query: "*",
	})
	if err != nil {
		t.Fatalf("QueryLogsCount failed: %v", err)
	}
	t.Logf("QueryLogsCount: %d", count)
}
