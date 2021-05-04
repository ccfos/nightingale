package rpc

import (
	"fmt"

	"github.com/didi/nightingale/v4/src/models"
)

func (*Server) HeartBeat(rev models.Instance, output *string) error {
	err := models.ReportHeartBeat(rev)
	if err != nil {
		*output = fmt.Sprintf("%v", err)
	}

	return nil
}

func (*Server) InstanceGets(mod string, instancesResp *models.InstancesResp) error {
	var err error
	instancesResp.Data, err = models.GetAllInstances(mod, 1)
	if err != nil {
		instancesResp.Msg = fmt.Sprintf("get %s installs err:%v", mod, err)
	}

	return nil
}
