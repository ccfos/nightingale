package all

import (
	// remote
	// _ "github.com/didi/nightingale/src/modules/monapi/plugins/api"
	// telegraf style
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/github"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/mongodb"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/mysql"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/prometheus"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/redis"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/nginx"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/elasticsearch"

	// local
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/log"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/plugin"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/port"
	_ "github.com/didi/nightingale/src/modules/monapi/plugins/proc"
)
