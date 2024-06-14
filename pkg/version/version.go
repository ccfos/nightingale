package version

import (
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
)

var Version = "unknown"
var GithubVersion atomic.Value

func CompareVersion(v1, v2 string) (int, error) {
	version1, err := version.NewVersion(v1)
	if err != nil {
		return 0, err
	}
	version2, err := version.NewVersion(v2)
	if err != nil {
		return 0, err
	}

	if version1.LessThan(version2) {
		return -1, nil
	}
	if version1.GreaterThan(version2) {
		return 1, nil
	}
	return 0, nil
}

func GetGithubVersion() {
	for {
		req := httplib.Get("https://api.github.com/repos/ccfos/nightingale/releases/latest")
		var release GithubRelease
		err := req.ToJSON(&release)
		if err != nil {
			logger.Errorf("get github version fail: %v", err)
		}

		GithubVersion.Store(release.TagName)
		time.Sleep(24 * time.Hour)
	}
}

type GithubRelease struct {
	TagName string `json:"tag_name"`
}
