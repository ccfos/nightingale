module github.com/didi/nightingale

go 1.12

require (
	github.com/Shopify/sarama v1.27.1
	github.com/cespare/xxhash v1.1.0
	github.com/codegangsta/negroni v1.0.0
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/dgryski/go-tsz v0.0.0-20180227144327-03b7d791f4fe
	github.com/garyburd/redigo v1.6.2
	github.com/gin-contrib/pprof v1.3.0
	github.com/gin-gonic/gin v1.6.3
	github.com/go-sql-driver/mysql v1.5.0
	github.com/google/go-github/v32 v32.1.0
	github.com/google/uuid v1.1.2-0.20190416172445-c2e93f3ae59f
	github.com/gorilla/mux v1.7.3
	github.com/hashicorp/golang-lru v0.5.4
	github.com/hpcloud/tail v1.0.0
	github.com/influxdata/influxdb v1.8.0
	github.com/influxdata/telegraf v1.16.2
	github.com/m3db/m3 v0.15.17
	github.com/mattn/go-isatty v0.0.12
	github.com/mattn/go-sqlite3 v1.14.0 // indirect
	github.com/mojocn/base64Captcha v1.3.1
	github.com/open-falcon/rrdlite v0.0.0-20200214140804-bf5829f786ad
	github.com/pquerna/cachecontrol v0.0.0-20200819021114-67c6ae64274f // indirect
	github.com/robfig/go-cache v0.0.0-20130306151617-9fc39e0dbf62 // indirect
	github.com/shirou/gopsutil v3.20.11+incompatible
	github.com/spaolacci/murmur3 v1.1.0
	github.com/spf13/viper v1.7.1
	github.com/streadway/amqp v1.0.0
	github.com/stretchr/testify v1.6.1
	github.com/toolkits/pkg v1.1.3
	github.com/ugorji/go/codec v1.1.7
	github.com/unrolled/render v1.0.3
	go.uber.org/automaxprocs v1.3.0 // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/text v0.3.3
	google.golang.org/protobuf v1.25.0 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/ldap.v3 v3.1.0
	gopkg.in/mgo.v2 v2.0.0-20180705113604-9856a29383ce
	gopkg.in/square/go-jose.v2 v2.5.1 // indirect
	gopkg.in/yaml.v2 v2.3.0
	xorm.io/core v0.7.3
	xorm.io/xorm v0.8.1
)

replace github.com/satori/go.uuid => github.com/satori/go.uuid v1.2.0

// branch 0.9.3-pool-read-binary-3
replace github.com/apache/thrift => github.com/m3db/thrift v0.0.0-20190820191926-05b5a2227fe4

// NB(nate): upgrading to the latest msgpack is not backwards compatibile as msgpack will no longer attempt to automatically
// write an integer into the smallest number of bytes it will fit in. We rely on this behavior by having helper methods
// in at least two encoders (see below) take int64s and expect that msgpack will size them down accordingly. We'll have
// to make integer sizing explicit before attempting to upgrade.
//
// Encoders:
// src/metrics/encoding/msgpack/base_encoder.go
// src/dbnode/persist/fs/msgpack/encoder.go
replace gopkg.in/vmihailenco/msgpack.v2 => github.com/vmihailenco/msgpack v2.8.3+incompatible

replace github.com/stretchr/testify => github.com/stretchr/testify v1.1.4-0.20160305165446-6fe211e49392

replace github.com/prometheus/common => github.com/prometheus/common v0.9.1

// Fix legacy import path - https://github.com/uber-go/atomic/pull/60
replace github.com/uber-go/atomic => github.com/uber-go/atomic v1.4.0

// Pull in https://github.com/etcd-io/bbolt/pull/220, required for go 1.14 compatibility
//
// etcd 3.14.13 depends on v1.3.3, but everything before v1.3.5 has unsafe misuses, and fails hard on go 1.14
// TODO: remove after etcd pulls in the change to a new release on 3.4 branch
replace go.etcd.io/bbolt => go.etcd.io/bbolt v1.3.5

// https://github.com/ory/dockertest/issues/212
replace golang.org/x/sys => golang.org/x/sys v0.0.0-20200826173525-f9321e4c35a6
