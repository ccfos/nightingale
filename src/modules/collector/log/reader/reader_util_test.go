package reader

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	path = "/home/${%Y%m}/log/${%Y%m%d}/application.log.${%Y-%m-%d-%H}"
)

func TestGetCurrentPath(t *testing.T) {
	now := time.Now()
	dir1 := now.Format("200601")
	dir2 := now.Format("20060102")
	suffix := now.Format("2006-01-02-15")
	expected := fmt.Sprintf("/home/%s/log/%s/application.log.%s", dir1, dir2, suffix)
	assert.Equal(t, expected, GetCurrentPath(path))
}
