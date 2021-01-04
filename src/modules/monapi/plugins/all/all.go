package all

import (
	// remote
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/api"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/github"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/mysql"
	// _ "github.com/didi/nightingale/src/modules/monapi/plugins/prometheus"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/redis"

	// local
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/log"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/plugin"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/port"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/proc"
)
