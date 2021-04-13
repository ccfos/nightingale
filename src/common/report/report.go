package report

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/v4/src/common/client"
	"github.com/didi/nightingale/v4/src/common/identity"
	"github.com/didi/nightingale/v4/src/models"

	"github.com/toolkits/pkg/logger"
)

type ReportSection struct {
	Mod      string `yaml:"mod"`
	Enabled  bool   `yaml:"enabled"`
	Interval int    `yaml:"interval"`
	Timeout  int    `yaml:"timeout"`
	HTTPPort string `yaml:"http_port"`
	RPCPort  string `yaml:"rpc_port"`
	Remark   string `yaml:"remark"`
	Region   string `yaml:"region"`
}

var Config ReportSection

func Init(cfg ReportSection) {
	Config = cfg
	for {
		report()
		time.Sleep(time.Duration(Config.Interval) * time.Millisecond)
	}
}

func report() {
	ident, _ := identity.GetIdent()
	instance := models.Instance{
		Module:   Config.Mod,
		Identity: ident,
		RPCPort:  Config.RPCPort,
		HTTPPort: Config.HTTPPort,
		Remark:   Config.Remark,
		Region:   Config.Region,
	}

	var resp string
	err := client.GetCli("server").Call("Server.HeartBeat", instance, &resp)
	if err != nil {
		client.CloseCli()
		return
	}

	if resp != "" {
		logger.Errorf("report instance:%+v err:%s", instance, resp)
	}
}

func GetAlive(wantedMod string) ([]*models.Instance, error) {

	var resp *models.InstancesResp
	err := client.GetCli("server").Call("Server.InstanceGets", wantedMod, &resp)
	if err != nil {
		client.CloseCli()
		return []*models.Instance{}, fmt.Errorf("get %s instances err:%v", wantedMod, err)
	}

	if resp.Msg != "" {
		return []*models.Instance{}, fmt.Errorf("get %s instances err:%s", wantedMod, resp.Msg)
	}

	return resp.Data, err
}
