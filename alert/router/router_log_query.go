package router

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
)

// maxLogQueryTimeout caps what a caller may ask for, so a stray parameter
// cannot pin an engine instance on a multi-GB scan.
const maxLogQueryTimeout = 60 * time.Second

// logGrepOptions builds the search bounds from the query string. Center passes
// timeout (milliseconds) and since (unix seconds) so that the grep on this
// instance stops on the same deadline the caller is waiting on, instead of
// running on after the caller has already given up.
func logGrepOptions(c *gin.Context) loggrep.GrepOptions {
	opts := loggrep.GrepOptions{}

	timeout := time.Duration(ginx.QueryInt64(c, "timeout", 0)) * time.Millisecond
	if timeout <= 0 || timeout > maxLogQueryTimeout {
		timeout = loggrep.DefaultTimeout
	}
	opts.Deadline = time.Now().Add(timeout)

	if since := ginx.QueryInt64(c, "since", 0); since > 0 {
		opts.Since = time.Unix(since, 0)
	}

	return opts
}
