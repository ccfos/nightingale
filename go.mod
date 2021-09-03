module github.com/didi/nightingale/v5

go 1.14

require (
	github.com/armon/go-metrics v0.3.4 // indirect
	github.com/gin-contrib/gzip v0.0.3
	github.com/gin-contrib/pprof v1.3.0
	github.com/gin-contrib/sessions v0.0.3
	github.com/gin-gonic/gin v1.7.0
	github.com/go-kit/kit v0.10.0
	github.com/go-ldap/ldap/v3 v3.2.4
	github.com/go-sql-driver/mysql v1.5.0
	github.com/gogo/protobuf v1.3.2
	github.com/golang/snappy v0.0.3
	github.com/gopherjs/gopherjs v0.0.0-20190910122728-9d188e94fb99 // indirect
	github.com/gorilla/sessions v1.2.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.2.0 // indirect
	github.com/hashicorp/go-msgpack v0.5.5 // indirect
	github.com/hashicorp/go-uuid v1.0.2 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.1-0.20190611123218-cf7d376da96d // indirect
	github.com/magiconair/properties v1.8.2 // indirect
	github.com/mattn/go-isatty v0.0.12
	github.com/n9e/agent-payload v0.0.0-20210619031503-b72325474651
	github.com/opentracing-contrib/go-stdlib v1.0.0
	github.com/opentracing/opentracing-go v1.2.0
	github.com/orcaman/concurrent-map v0.0.0-20210106121528-16402b402231
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.9.0
	github.com/prometheus/common v0.17.0
	github.com/prometheus/prometheus v1.8.2-0.20210220213500-8c8de46003d1
	github.com/smartystreets/assertions v1.0.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/spf13/cast v1.3.1-0.20190531151931-f31dc0aaab5a // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/viper v1.7.1
	github.com/subosito/gotenv v1.2.1-0.20190917103637-de67a6614a4d // indirect
	github.com/toolkits/pkg v1.1.3
	github.com/ugorji/go/codec v1.1.7
	go.uber.org/atomic v1.7.0
	go.uber.org/automaxprocs v1.4.0 // indirect
	golang.org/x/text v0.3.5
	gopkg.in/ini.v1 v1.51.1 // indirect
	xorm.io/builder v0.3.7
	xorm.io/xorm v1.0.7
)

// branch 0.9.3-pool-read-binary-3
replace github.com/apache/thrift => github.com/m3db/thrift v0.0.0-20190820191926-05b5a2227fe4

// Fix legacy import path - https://github.com/uber-go/atomic/pull/60
replace github.com/uber-go/atomic => github.com/uber-go/atomic v1.4.0

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0
