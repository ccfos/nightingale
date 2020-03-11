package reader

import (
	"fmt"
	"testing"
	"time"
)

func TestGetcurrentpath(t *testing.T) {
	path := "/home/${%Y%m}/log/${%Y%m%d}/application.log.${%Y-%m-%d-%H}"
	now := time.Now()
	dir1 := now.Format("200601")
	dir2 := now.Format("20060102")
	suffix := now.Format("2006-01-02-15")
	shouldbe := fmt.Sprintf("/home/%s/log/%s/application.log.%s", dir1, dir2, suffix)
	if GetCurrentPath(path) != shouldbe {
		t.Error("getcurrentpath failed")
	}
}
func TestGetnextpath(t *testing.T) {
	path := "/home/${%Y%m}/log/${%Y%m%d}/application.log.${%Y-%m-%d-%H}"
	now := time.Now().Add(time.Hour)
	dir1 := now.Format("200601")
	dir2 := now.Format("20060102")
	suffix := now.Format("2006-01-02-15")
	shouldbe := fmt.Sprintf("/home/%s/log/%s/application.log.%s", dir1, dir2, suffix)
	if GetNextPath(path) != shouldbe {
		t.Error("getcurrentpath failed")
	}
}
