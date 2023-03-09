package upgrade

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/go-sql-driver/mysql"
)

func Upgrade(configFile string, sqlFile string) error {
	var config Config
	Parse(configFile, &config)
	dsnConf, _ := mysql.ParseDSN(config.DB.DSN)
	passwd := dsnConf.Passwd
	user := dsnConf.User

	cmd := exec.Command("mysql", fmt.Sprintf("-u%s", user), fmt.Sprintf("-p%s", passwd), "<", sqlFile)
	if err := cmd.Run(); err != nil {
		return err
	}

	db, err := storage.New(config.DB)
	if err != nil {
		return err
	}
	ctx := ctx.NewContext(context.Background(), db)

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

	// target
	err = models.TargetUpgradeToV6(ctx, m)
	if err != nil {
		return err
	}

	// recoding rule
	err = models.RecordingRuleUpgradeToV6(ctx, m)
	if err != nil {
		return err
	}

	return nil
}
