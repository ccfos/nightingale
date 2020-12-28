package report

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
	"github.com/toolkits/pkg/str"
	"github.com/toolkits/pkg/sys"

	"github.com/didi/nightingale/src/common/address"
	"github.com/didi/nightingale/src/common/identity"
	"github.com/didi/nightingale/src/modules/agent/config"
)

var (
	SN    string
	IP    string
	Ident string
)

func LoopReport() {
	duration := time.Duration(config.Config.Report.Interval) * time.Second
	for {
		time.Sleep(duration)
		if err := report(); err != nil {
			logger.Error("report occur error: ", err)
		}
	}
}

func GatherBase() error {
	var err error
	SN, err = exec(config.Config.Report.SN)
	if err != nil {
		return fmt.Errorf("cannot get sn: %s", err)
	}

	IP, err = identity.GetIP()
	if err != nil {
		return fmt.Errorf("cannot get ip: %s", err)
	}

	if !str.IsIP(IP) {
		return fmt.Errorf("'%s' not ip", IP)
	}

	Ident, err = identity.GetIdent()
	if err != nil {
		return fmt.Errorf("cannot get ident: %s", err)
	}

	return nil
}

func gatherFields(m map[string]string) (map[string]string, error) {
	ret := make(map[string]string)
	for k, v := range m {
		output, err := exec(v)
		if err != nil {
			logger.Errorf("get %s by exec %v err:%v", k, v, err)
			continue
		}
		ret[k] = output
	}
	return ret, nil
}

type hostRegisterForm struct {
	SN      string            `json:"sn"`
	IP      string            `json:"ip"`
	Ident   string            `json:"ident"`
	Name    string            `json:"name"`
	Cate    string            `json:"cate"`
	UniqKey string            `json:"uniqkey"`
	Fields  map[string]string `json:"fields"`
	Digest  string            `json:"digest"`
}

type errRes struct {
	Err string `json:"err"`
}

func report() error {
	name, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("cannot get hostname: %s", err)
	}

	fields, err := gatherFields(config.Config.Report.Fields)
	if err != nil {
		return err
	}

	form := hostRegisterForm{
		SN:      SN,
		IP:      IP,
		Ident:   Ident,
		Name:    name,
		Cate:    config.Config.Report.Cate,
		UniqKey: config.Config.Report.UniqKey,
		Fields:  fields,
	}

	content := form.SN + form.IP + form.Ident + form.Name + form.Cate + form.UniqKey
	var keys []string
	for key := range fields {
		keys = append(keys, key, fields[key])
	}
	sort.Strings(keys)

	for _, key := range keys {
		content += fields[key]
	}

	form.Digest = str.MD5(content)

	servers := address.GetHTTPAddresses("ams")
	for _, i := range rand.Perm(len(servers)) {
		url := fmt.Sprintf("http://%s/v1/ams-ce/hosts/register", servers[i])

		logger.Debugf("report: %+v", form)

		var body errRes
		err := httplib.Post(url).JSONBodyQuiet(form).Header("X-Srv-Token", config.Config.Report.Token).SetTimeout(time.Second * 5).ToJSON(&body)
		if err != nil {
			js, _ := json.Marshal(form)
			logger.Errorf("report payload: %s, token: %s", string(js), config.Config.Report.Token)
			return fmt.Errorf("curl %s fail: %v", url, err)
		}

		if body.Err != "" {
			return fmt.Errorf(body.Err)
		}

		return nil
	}

	return fmt.Errorf("all server instance is dead")
}

func exec(shell string) (string, error) {
	out, err := sys.CmdOutTrim("sh", "-c", shell)
	if err != nil {
		return "", fmt.Errorf("cannot exec `%s', error: %v", shell, err)
	}

	return out, nil
}
