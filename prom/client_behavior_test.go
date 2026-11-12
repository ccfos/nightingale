package prom

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	n9ectx "github.com/ccfos/nightingale/v6/pkg/ctx"
	promkit "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPromClientMap_SelectsReaderAndWriterByDatasource(t *testing.T) {
	pc := &PromClientMap{}
	pc.Reset()
	require.NoError(t, pc.setClientFromPromOption(11, PromOption{
		Url: "http://127.0.0.1:19090/read", WriteAddr: "http://127.0.0.1:19091/write",
	}))
	reader, writer := pc.GetCli(11), pc.GetWriterCli(11)

	pc.Set(22, reader, writer)

	require.Same(t, reader, pc.GetCli(22))
	require.Equal(t, writer, pc.GetWriterCli(22))
	require.False(t, pc.IsNil(11))
	require.ElementsMatch(t, []int64{11, 22}, pc.GetDatasourceIds())
	require.Nil(t, pc.GetCli(99))
	require.Equal(t, promkit.WriterType{}, pc.GetWriterCli(99))
	require.True(t, pc.IsNil(99))
}

func TestPromClientMap_NilReaderDoesNotReplaceClients(t *testing.T) {
	pc := &PromClientMap{}
	pc.Reset()
	require.NoError(t, pc.setClientFromPromOption(1, PromOption{Url: "http://127.0.0.1:19090"}))
	reader, writer := pc.GetCli(1), pc.GetWriterCli(1)

	pc.Set(1, nil, promkit.WriterType{})
	pc.Set(2, nil, writer)

	require.Same(t, reader, pc.GetCli(1))
	require.Equal(t, writer, pc.GetWriterCli(1))
	require.True(t, pc.IsNil(2))
	require.Equal(t, promkit.WriterType{}, pc.GetWriterCli(2))
	pc.ReaderClients[3] = nil
	require.True(t, pc.IsNil(3))
}

func TestPromClientMap_DeleteRemovesBothClients(t *testing.T) {
	pc := &PromClientMap{}
	pc.Reset()
	require.NoError(t, pc.setClientFromPromOption(1, PromOption{Url: "http://127.0.0.1:19090"}))

	pc.Del(1)

	require.True(t, pc.IsNil(1))
	require.Equal(t, promkit.WriterType{}, pc.GetWriterCli(1))
	require.Empty(t, pc.GetDatasourceIds())
}

func TestPromClientMap_ResetClearsBothMaps(t *testing.T) {
	pc := &PromClientMap{}
	pc.Reset()
	require.NoError(t, pc.setClientFromPromOption(1, PromOption{Url: "http://127.0.0.1:19090"}))

	pc.Reset()

	require.Empty(t, pc.ReaderClients)
	require.Empty(t, pc.WriterClients)
	pc.Set(2, nil, promkit.WriterType{})
	require.Empty(t, pc.GetDatasourceIds())
}

func TestSetClientFromPromOption_RejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		id     int64
		option PromOption
		want   string
	}{
		{name: "negative id", id: -1, want: "clusterName"},
		{name: "missing URL", id: 1, want: "url is blank"},
		{name: "invalid reader URL", id: 1, option: PromOption{Url: "://bad"}, want: "newClientFromPromOption"},
		{
			name: "invalid writer URL", id: 1,
			option: PromOption{Url: "http://127.0.0.1:19090", WriteAddr: "://bad"},
			want:   "newClientFromPromOption",
		},
		{
			name: "invalid TLS CA", id: 1,
			option: PromOption{Url: "http://127.0.0.1:19090", TLS: models.TLS{CACert: "invalid PEM"}},
			want:   "TLS config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &PromClientMap{ReaderClients: map[int64]promkit.API{}, WriterClients: map[int64]promkit.WriterType{}}

			err := pc.setClientFromPromOption(tt.id, tt.option)

			require.ErrorContains(t, err, tt.want)
			require.Empty(t, pc.ReaderClients)
			require.Empty(t, pc.WriterClients)
		})
	}
}

func TestPromClients_KeepSeparateReadAndWriteAddresses(t *testing.T) {
	pc := &PromClientMap{}
	option := PromOption{Url: "http://127.0.0.1:19090/read", WriteAddr: "http://127.0.0.1:19091/write"}

	reader, err := pc.newReaderClientFromPromOption(option)
	require.NoError(t, err)
	writer, err := pc.newWriterClientFromPromOption(option)
	require.NoError(t, err)

	require.Equal(t, "http://127.0.0.1:19090/read/api/v1/query", reader.URL("/api/v1/query", nil).String())
	require.Equal(t, "http://127.0.0.1:19091/write/api/v1/write", writer.URL("/api/v1/write", nil).String())
}

func TestPromOption_EqualDetectsConnectionChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(*PromOption)
	}{
		{name: "read address", change: func(p *PromOption) { p.Url = "changed" }},
		{name: "write address", change: func(p *PromOption) { p.WriteAddr = "changed" }},
		{name: "username", change: func(p *PromOption) { p.BasicAuthUser = "changed" }},
		{name: "password", change: func(p *PromOption) { p.BasicAuthPass = "changed" }},
		{name: "timeout", change: func(p *PromOption) { p.Timeout++ }},
		{name: "dial timeout", change: func(p *PromOption) { p.DialTimeout++ }},
		{name: "idle connections", change: func(p *PromOption) { p.MaxIdleConnsPerHost++ }},
		{name: "header count", change: func(p *PromOption) { p.Headers = nil }},
		{name: "header value", change: func(p *PromOption) { p.Headers = []string{"X-Tenant", "other"} }},
		{name: "skip TLS verification", change: func(p *PromOption) { p.TLS.SkipTlsVerify = true }},
		{name: "CA", change: func(p *PromOption) { p.TLS.CACert = "ca" }},
		{name: "client cert", change: func(p *PromOption) { p.TLS.ClientCert = "cert" }},
		{name: "client key", change: func(p *PromOption) { p.TLS.ClientKey = "key" }},
		{name: "server name", change: func(p *PromOption) { p.TLS.ServerName = "server" }},
		{name: "minimum TLS", change: func(p *PromOption) { p.TLS.MinVersion = "1.2" }},
		{name: "maximum TLS", change: func(p *PromOption) { p.TLS.MaxVersion = "1.3" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := PromOption{Url: "http://127.0.0.1:19090", Headers: []string{"X-Tenant", "team"}}
			changed := base
			tt.change(&changed)

			require.False(t, base.Equal(changed))
			require.True(t, base.Equal(base))
		})
	}
}

func TestPromOptions_SetGetAndDelete(t *testing.T) {
	options := &PromOptionsStruct{Data: make(map[int64]PromOption)}
	original := PromOption{Url: "http://127.0.0.1:19090"}
	options.Set(1, original)

	got, found := options.Get(1)

	require.True(t, found)
	require.Equal(t, original, got)
	options.Set(1, PromOption{Url: "http://127.0.0.1:19091"})
	got, found = options.Get(1)
	require.True(t, found)
	require.Equal(t, "http://127.0.0.1:19091", got.Url)
	options.Del(1)
	_, found = options.Get(1)
	require.False(t, found)
}

func TestPromClientMap_LoadFromEmptyDatabaseRemovesStaleClients(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Datasource{}))
	pc := &PromClientMap{ctx: n9ectx.NewContext(context.Background(), db, true)}
	pc.Reset()
	require.NoError(t, pc.setClientFromPromOption(42, PromOption{Url: "http://127.0.0.1:19090"}))
	original := PromOptions
	PromOptions = &PromOptionsStruct{Data: map[int64]PromOption{42: {Url: "http://127.0.0.1:19090"}}}
	t.Cleanup(func() { PromOptions = original })

	pc.loadFromDatabase()

	require.Empty(t, pc.GetDatasourceIds())
	require.Equal(t, promkit.WriterType{}, pc.GetWriterCli(42))
	_, found := PromOptions.Get(42)
	require.False(t, found)
}
