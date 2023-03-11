package upgrade

import (
	"context"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/toolkits/pkg/logger"
)

func Upgrade(configFile string) error {
	var config Config
	Parse(configFile, &config)

	db, err := storage.New(config.DB)
	if err != nil {
		return err
	}

	ctx := ctx.NewContext(context.Background(), db)
	for _, cluster := range config.Clusters {
		count, err := models.GetDatasourcesCountBy(ctx, "", "", cluster.Name)
		if err != nil {
			logger.Errorf("get datasource %s count error: %v", cluster.Name, err)
			continue
		}
		if count > 0 {
			continue
		}

		header := make(map[string]string)
		headerCount := len(cluster.Headers)
		if headerCount > 0 && headerCount%2 == 0 {
			for i := 0; i < len(cluster.Headers); i += 2 {
				header[cluster.Headers[i]] = cluster.Headers[i+1]
			}
		}

		authJosn := models.Auth{
			BasicAuthUser:     cluster.BasicAuthUser,
			BasicAuthPassword: cluster.BasicAuthPass,
		}

		httpJson := models.HTTP{
			Timeout:     cluster.Timeout,
			DialTimeout: cluster.DialTimeout,
			UseTLS: models.TLS{
				SkipTlsVerify: cluster.UseTLS,
			},
			MaxIdleConnsPerHost: cluster.MaxIdleConnsPerHost,
			Url:                 cluster.Prom,
			Headers:             header,
		}

		datasrouce := models.Datasource{
			PluginId:       1,
			PluginType:     "prometheus",
			PluginTypeName: "Prometheus Like",
			Name:           cluster.Name,
			HTTPJson:       httpJson,
			AuthJson:       authJosn,
			ClusterName:    "default",
		}

		err = datasrouce.Add(ctx)
		if err != nil {
			logger.Errorf("add datasource %s error: %v", cluster.Name, err)
		}
	}

	datasources, err := models.GetDatasources(ctx)
	if err != nil {
		return err
	}

	m := make(map[string]models.Datasource)
	for i := 0; i < len(datasources); i++ {
		m[datasources[i].Name] = datasources[i]
	}

	err = models.AlertRuleUpgradeToV6(ctx, m)
	if err != nil {
		return err
	}

	// alert mute
	err = models.AlertMuteUpgradeToV6(ctx, m)
	if err != nil {
		return err
	}
	// alert subscribe
	err = models.AlertSubscribeUpgradeToV6(ctx, m)
	if err != nil {
		return err
	}

	// recoding rule
	err = models.RecordingRuleUpgradeToV6(ctx, m)
	if err != nil {
		return err
	}

	// alert cur event
	err = models.AlertCurEventUpgradeToV6(ctx, m)
	if err != nil {
		return err
	}

	// alert his event
	err = models.AlertHisEventUpgradeToV6(ctx, m)
	if err != nil {
		return err
	}
	return nil
}
