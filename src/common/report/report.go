package report

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/src/common/address"
	"github.com/didi/nightingale/src/common/identity"
	"github.com/didi/nightingale/src/models"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
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

func Init(cfg ReportSection, mod string) {
	Config = cfg

	addrs := address.GetHTTPAddresses(mod)

	t1 := time.NewTicker(time.Duration(Config.Interval) * time.Millisecond)
	report(addrs)
	for {
		<-t1.C
		report(addrs)
	}
}

type reportRes struct {
	Err string `json:"err"`
	Dat string `json:"dat"`
}

func report(addrs []string) {
	perm := rand.Perm(len(addrs))
	for i := range perm {
		url := fmt.Sprintf("http://%s/api/hbs/heartbeat", addrs[perm[i]])

		ident, _ := identity.GetIdent()
		m := map[string]string{
			"module":    Config.Mod,
			"identity":  ident,
			"rpc_port":  Config.RPCPort,
			"http_port": Config.HTTPPort,
			"remark":    Config.Remark,
			"region":    Config.Region,
		}

		var body reportRes
		err := httplib.Post(url).JSONBodyQuiet(m).SetTimeout(3 * time.Second).ToJSON(&body)
		if err != nil {
			logger.Errorf("curl %s fail: %v", url, err)
			continue
		}

		if body.Err != "" {
			logger.Error(body.Err)
			continue
		}

		return
	}
}

type instanceRes struct {
	Err string             `json:"err"`
	Dat []*models.Instance `json:"dat"`
}

func GetAlive(wantedMod, serverMod string) ([]*models.Instance, error) {
	addrs := address.GetHTTPAddresses(serverMod)
	perm := rand.Perm(len(addrs))

	timeout := 3000
	if Config.Timeout != 0 {
		timeout = Config.Timeout
	}

	var body instanceRes
	var err error
	for i := range perm {
		url := fmt.Sprintf("http://%s/api/hbs/instances?mod=%s&alive=1", addrs[perm[i]], wantedMod)
		err = httplib.Get(url).SetTimeout(time.Duration(timeout) * time.Millisecond).ToJSON(&body)

		if err != nil {
			logger.Warningf("curl %s fail: %v", url, err)
			continue
		}

		if body.Err != "" {
			err = fmt.Errorf("curl %s fail: %v", url, body.Err)
			logger.Warning(err)
			continue
		}
	}
	return body.Dat, err
}
