package flashduty

import (
	"fmt"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"net/url"
	"time"
)

func PostFlashDuty(fdUrl, path string, timeout time.Duration, appKey string, body interface{}) (response []byte, code int, err error) {
	urlParams := url.Values{}
	urlParams.Add("app_key", appKey)
	var url string
	if fdUrl != "" {
		url = fmt.Sprintf("%s%s?%s", fdUrl, path, urlParams.Encode())
	} else {
		url = fmt.Sprintf("%s%s?%s", "https://jira.flashcat.cloud/api", path, urlParams.Encode())
	}
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return poster.PostJSON(url, timeout, body)
}
