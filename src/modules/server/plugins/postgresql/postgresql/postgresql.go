package postgresql

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	// register in driver.
	_ "github.com/jackc/pgx/stdlib"

	"github.com/blang/semver"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
)

type OverrideQuery struct {
	versionRange semver.Range
	query        string
}

// ColumnUsage should be one of several enum values which describe how a
// queried row is to be converted to a Prometheus metric.
type ColumnUsage int

// nolint: golint
const (
	DISCARD      ColumnUsage = iota // Ignore this column
	LABEL        ColumnUsage = iota // Use this column as a label
	COUNTER      ColumnUsage = iota // Use this column as a counter
	GAUGE        ColumnUsage = iota // Use this column as a gauge
	MAPPEDMETRIC ColumnUsage = iota // Use this column with the supplied mapping of text values
	DURATION     ColumnUsage = iota // This column should be interpreted as a text duration (and converted to milliseconds)
	HISTOGRAM    ColumnUsage = iota // Use this column as a histogram
)

// ColumnMapping is the user-friendly representation of a prometheus descriptor map
type ColumnMapping struct {
	usage             ColumnUsage  `yaml:"usage"`
	description       string       `yaml:"description"`
	supportedVersions semver.Range `yaml:"pg_version"` // Semantic version ranges which are supported. Unsupported columns are not queried (internally converted to DISCARD).
}

// intermediateMetricMap holds the partially loaded metric map parsing.
// This is mainly so we can parse cacheSeconds around.
type intermediateMetricMap struct {
	columnMappings map[string]ColumnMapping
}

var builtinMetricMaps = map[string]intermediateMetricMap{
	"pg_stat_bgwriter": {
		map[string]ColumnMapping{
			"checkpoints_timed":     {COUNTER, "Number of scheduled checkpoints that have been performed", nil},
			"checkpoints_req":       {COUNTER, "Number of requested checkpoints that have been performed", nil},
			"checkpoint_write_time": {COUNTER, "Total amount of time that has been spent in the portion of checkpoint processing where files are written to disk, in milliseconds", nil},
			"checkpoint_sync_time":  {COUNTER, "Total amount of time that has been spent in the portion of checkpoint processing where files are synchronized to disk, in milliseconds", nil},
			"buffers_checkpoint":    {COUNTER, "Number of buffers written during checkpoints", nil},
			"buffers_clean":         {COUNTER, "Number of buffers written by the background writer", nil},
			"maxwritten_clean":      {COUNTER, "Number of times the background writer stopped a cleaning scan because it had written too many buffers", nil},
			"buffers_backend":       {COUNTER, "Number of buffers written directly by a backend", nil},
			"buffers_backend_fsync": {COUNTER, "Number of times a backend had to execute its own fsync call (normally the background writer handles those even when the backend does its own write)", nil},
			"buffers_alloc":         {COUNTER, "Number of buffers allocated", nil},
			"stats_reset":           {COUNTER, "Time at which these statistics were last reset", nil},
		},
	},
	"pg_stat_database": {
		map[string]ColumnMapping{
			"datid":          {LABEL, "OID of a database", nil},
			"datname":        {LABEL, "Name of this database", nil},
			"numbackends":    {GAUGE, "Number of backends currently connected to this database. This is the only column in this view that returns a value reflecting current state; all other columns return the accumulated values since the last reset.", nil},
			"xact_commit":    {COUNTER, "Number of transactions in this database that have been committed", nil},
			"xact_rollback":  {COUNTER, "Number of transactions in this database that have been rolled back", nil},
			"blks_read":      {COUNTER, "Number of disk blocks read in this database", nil},
			"blks_hit":       {COUNTER, "Number of times disk blocks were found already in the buffer cache, so that a read was not necessary (this only includes hits in the PostgreSQL buffer cache, not the operating system's file system cache)", nil},
			"tup_returned":   {COUNTER, "Number of rows returned by queries in this database", nil},
			"tup_fetched":    {COUNTER, "Number of rows fetched by queries in this database", nil},
			"tup_inserted":   {COUNTER, "Number of rows inserted by queries in this database", nil},
			"tup_updated":    {COUNTER, "Number of rows updated by queries in this database", nil},
			"tup_deleted":    {COUNTER, "Number of rows deleted by queries in this database", nil},
			"conflicts":      {COUNTER, "Number of queries canceled due to conflicts with recovery in this database. (Conflicts occur only on standby servers; see pg_stat_database_conflicts for details.)", nil},
			"temp_files":     {COUNTER, "Number of temporary files created by queries in this database. All temporary files are counted, regardless of why the temporary file was created (e.g., sorting or hashing), and regardless of the log_temp_files setting.", nil},
			"temp_bytes":     {COUNTER, "Total amount of data written to temporary files by queries in this database. All temporary files are counted, regardless of why the temporary file was created, and regardless of the log_temp_files setting.", nil},
			"deadlocks":      {COUNTER, "Number of deadlocks detected in this database", nil},
			"blk_read_time":  {COUNTER, "Time spent reading data file blocks by backends in this database, in milliseconds", nil},
			"blk_write_time": {COUNTER, "Time spent writing data file blocks by backends in this database, in milliseconds", nil},
			"stats_reset":    {COUNTER, "Time at which these statistics were last reset", nil},
		},
	},
	"pg_stat_database_count": {
		map[string]ColumnMapping{
			"datid":          {LABEL, "OID of a database", nil},
			"datname":        {LABEL, "Name of this database", nil},
			"dml_data_count": {COUNTER, "", nil},
			"tps":            {COUNTER, "", nil},
		},
	},
	"pg_stat_database_conflicts": {
		map[string]ColumnMapping{
			"datid":            {LABEL, "OID of a database", nil},
			"datname":          {LABEL, "Name of this database", nil},
			"confl_tablespace": {COUNTER, "Number of queries in this database that have been canceled due to dropped tablespaces", nil},
			"confl_lock":       {COUNTER, "Number of queries in this database that have been canceled due to lock timeouts", nil},
			"confl_snapshot":   {COUNTER, "Number of queries in this database that have been canceled due to old snapshots", nil},
			"confl_bufferpin":  {COUNTER, "Number of queries in this database that have been canceled due to pinned buffers", nil},
			"confl_deadlock":   {COUNTER, "Number of queries in this database that have been canceled due to deadlocks", nil},
		},
	},
	"pg_locks": {
		map[string]ColumnMapping{
			"datname": {LABEL, "Name of this database", nil},
			"mode":    {LABEL, "Type of Lock", nil},
			"count":   {GAUGE, "Number of locks", nil},
		},
	},
	"pg_stat_replication": {
		map[string]ColumnMapping{
			"procpid":                  {DISCARD, "Process ID of a WAL sender process", semver.MustParseRange("<9.2.0")},
			"pid":                      {DISCARD, "Process ID of a WAL sender process", semver.MustParseRange(">=9.2.0")},
			"usesysid":                 {DISCARD, "OID of the user logged into this WAL sender process", nil},
			"usename":                  {DISCARD, "Name of the user logged into this WAL sender process", nil},
			"application_name":         {LABEL, "Name of the application that is connected to this WAL sender", nil},
			"client_addr":              {LABEL, "IP address of the client connected to this WAL sender. If this field is null, it indicates that the client is connected via a Unix socket on the server machine.", nil},
			"client_hostname":          {DISCARD, "Host name of the connected client, as reported by a reverse DNS lookup of client_addr. This field will only be non-null for IP connections, and only when log_hostname is enabled.", nil},
			"client_port":              {DISCARD, "TCP port number that the client is using for communication with this WAL sender, or -1 if a Unix socket is used", nil},
			"backend_start":            {DISCARD, "with time zone	Time when this process was started, i.e., when the client connected to this WAL sender", nil},
			"backend_xmin":             {DISCARD, "The current backend's xmin horizon.", nil},
			"state":                    {LABEL, "Current WAL sender state", nil},
			"sent_location":            {DISCARD, "Last transaction log position sent on this connection", semver.MustParseRange("<10.0.0")},
			"write_location":           {DISCARD, "Last transaction log position written to disk by this standby server", semver.MustParseRange("<10.0.0")},
			"flush_location":           {DISCARD, "Last transaction log position flushed to disk by this standby server", semver.MustParseRange("<10.0.0")},
			"replay_location":          {DISCARD, "Last transaction log position replayed into the database on this standby server", semver.MustParseRange("<10.0.0")},
			"sent_lsn":                 {DISCARD, "Last transaction log position sent on this connection", semver.MustParseRange(">=10.0.0")},
			"write_lsn":                {DISCARD, "Last transaction log position written to disk by this standby server", semver.MustParseRange(">=10.0.0")},
			"flush_lsn":                {DISCARD, "Last transaction log position flushed to disk by this standby server", semver.MustParseRange(">=10.0.0")},
			"replay_lsn":               {DISCARD, "Last transaction log position replayed into the database on this standby server", semver.MustParseRange(">=10.0.0")},
			"sync_priority":            {DISCARD, "Priority of this standby server for being chosen as the synchronous standby", nil},
			"sync_state":               {DISCARD, "Synchronous state of this standby server", nil},
			"slot_name":                {LABEL, "A unique, cluster-wide identifier for the replication slot", semver.MustParseRange(">=9.2.0")},
			"plugin":                   {DISCARD, "The base name of the shared object containing the output plugin this logical slot is using, or null for physical slots", nil},
			"slot_type":                {DISCARD, "The slot type - physical or logical", nil},
			"datoid":                   {DISCARD, "The OID of the database this slot is associated with, or null. Only logical slots have an associated database", nil},
			"database":                 {DISCARD, "The name of the database this slot is associated with, or null. Only logical slots have an associated database", nil},
			"active":                   {DISCARD, "True if this slot is currently actively being used", nil},
			"active_pid":               {DISCARD, "Process ID of a WAL sender process", nil},
			"xmin":                     {DISCARD, "The oldest transaction that this slot needs the database to retain. VACUUM cannot remove tuples deleted by any later transaction", nil},
			"catalog_xmin":             {DISCARD, "The oldest transaction affecting the system catalogs that this slot needs the database to retain. VACUUM cannot remove catalog tuples deleted by any later transaction", nil},
			"restart_lsn":              {DISCARD, "The address (LSN) of oldest WAL which still might be required by the consumer of this slot and thus won't be automatically removed during checkpoints", nil},
			"pg_current_xlog_location": {DISCARD, "pg_current_xlog_location", nil},
			"pg_current_wal_lsn":       {DISCARD, "pg_current_xlog_location", semver.MustParseRange(">=10.0.0")},
			"pg_current_wal_lsn_bytes": {GAUGE, "WAL position in bytes", semver.MustParseRange(">=10.0.0")},
			"pg_xlog_location_diff":    {GAUGE, "Lag in bytes between master and slave", semver.MustParseRange(">=9.2.0 <10.0.0")},
			"pg_wal_lsn_diff":          {GAUGE, "Lag in bytes between master and slave", semver.MustParseRange(">=10.0.0")},
			"confirmed_flush_lsn":      {DISCARD, "LSN position a consumer of a slot has confirmed flushing the data received", nil},
			"write_lag":                {DISCARD, "Time elapsed between flushing recent WAL locally and receiving notification that this standby server has written it (but not yet flushed it or applied it). This can be used to gauge the delay that synchronous_commit level remote_write incurred while committing if this server was configured as a synchronous standby.", semver.MustParseRange(">=10.0.0")},
			"flush_lag":                {DISCARD, "Time elapsed between flushing recent WAL locally and receiving notification that this standby server has written and flushed it (but not yet applied it). This can be used to gauge the delay that synchronous_commit level remote_flush incurred while committing if this server was configured as a synchronous standby.", semver.MustParseRange(">=10.0.0")},
			"replay_lag":               {DISCARD, "Time elapsed between flushing recent WAL locally and receiving notification that this standby server has written, flushed and applied it. This can be used to gauge the delay that synchronous_commit level remote_apply incurred while committing if this server was configured as a synchronous standby.", semver.MustParseRange(">=10.0.0")},
		},
	},
	"pg_replication_slots": {
		map[string]ColumnMapping{
			"slot_name":       {LABEL, "Name of the replication slot", nil},
			"database":        {LABEL, "Name of the database", nil},
			"active":          {GAUGE, "Flag indicating if the slot is active", nil},
			"pg_wal_lsn_diff": {GAUGE, "Replication lag in bytes", nil},
		},
	},
	"pg_stat_archiver": {
		map[string]ColumnMapping{
			"archived_count":     {COUNTER, "Number of WAL files that have been successfully archived", nil},
			"last_archived_wal":  {DISCARD, "Name of the last WAL file successfully archived", nil},
			"last_archived_time": {DISCARD, "Time of the last successful archive operation", nil},
			"failed_count":       {COUNTER, "Number of failed attempts for archiving WAL files", nil},
			"last_failed_wal":    {DISCARD, "Name of the WAL file of the last failed archival operation", nil},
			"last_failed_time":   {DISCARD, "Time of the last failed archival operation", nil},
			"stats_reset":        {DISCARD, "Time at which these statistics were last reset", nil},
			"last_archive_age":   {GAUGE, "Time in seconds since last WAL segment was successfully archived", nil},
		},
	},
	"pg_stat_activity": {
		map[string]ColumnMapping{
			"datname":         {LABEL, "Name of this database", nil},
			"state":           {LABEL, "connection state", semver.MustParseRange(">=9.2.0")},
			"count":           {GAUGE, "number of connections in this state", nil},
			"max_tx_duration": {GAUGE, "max duration in seconds any active transaction has been running", nil},
		},
	},
}

// Overriding queries for namespaces above.
// TODO: validate this is a closed set in tests, and there are no overlaps
var queryOverrides = map[string][]OverrideQuery{
	"pg_locks": {
		{
			semver.MustParseRange(">0.0.0"),
			`SELECT pg_database.datname,tmp.mode,COALESCE(count,0) as count
			FROM
				(
				  VALUES ('accesssharelock'),
				         ('rowsharelock'),
				         ('rowexclusivelock'),
				         ('shareupdateexclusivelock'),
				         ('sharelock'),
				         ('sharerowexclusivelock'),
				         ('exclusivelock'),
				         ('accessexclusivelock'),
					 ('sireadlock')
				) AS tmp(mode) CROSS JOIN pg_database
			LEFT JOIN
			  (SELECT database, lower(mode) AS mode,count(*) AS count
			  FROM pg_locks WHERE database IS NOT NULL
			  GROUP BY database, lower(mode)
			) AS tmp2
			ON tmp.mode=tmp2.mode and pg_database.oid = tmp2.database ORDER BY 1`,
		},
	},

	"pg_stat_replication": {
		{
			semver.MustParseRange(">=10.0.0"),
			`
			SELECT *,
				(case pg_is_in_recovery() when 't' then null else pg_current_wal_lsn() end) AS pg_current_wal_lsn,
				(case pg_is_in_recovery() when 't' then null else pg_wal_lsn_diff(pg_current_wal_lsn(), pg_lsn('0/0'))::float end) AS pg_current_wal_lsn_bytes,
				(case pg_is_in_recovery() when 't' then null else pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn)::float end) AS pg_wal_lsn_diff
			FROM pg_stat_replication
			`,
		},
		{
			semver.MustParseRange(">=9.2.0 <10.0.0"),
			`
			SELECT *,
				(case pg_is_in_recovery() when 't' then null else pg_current_xlog_location() end) AS pg_current_xlog_location,
				(case pg_is_in_recovery() when 't' then null else pg_xlog_location_diff(pg_current_xlog_location(), replay_location)::float end) AS pg_xlog_location_diff
			FROM pg_stat_replication
			`,
		},
		{
			semver.MustParseRange("<9.2.0"),
			`
			SELECT *,
				(case pg_is_in_recovery() when 't' then null else pg_current_xlog_location() end) AS pg_current_xlog_location
			FROM pg_stat_replication
			`,
		},
	},

	"pg_replication_slots": {
		{
			semver.MustParseRange(">=9.4.0"),
			`
			SELECT slot_name, database, active, pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)
			FROM pg_replication_slots
			`,
		},
	},
	"pg_stat_database": {
		{
			semver.MustParseRange(">=0.0.0"),
			`
			SELECT * FROM pg_stat_database
			`,
		},
	},
	"pg_stat_database_count": {
		{
			semver.MustParseRange(">=0.0.0"),
			`
			SELECT
				datid,
				datname,
				tup_inserted + tup_updated + tup_deleted AS dml_data_count,
				xact_rollback + xact_commit AS tps 
			FROM
				pg_stat_database
			`,
		},
	},
	"pg_stat_archiver": {
		{
			semver.MustParseRange(">=0.0.0"),
			`
			SELECT *,
				extract(epoch from now() - last_archived_time) AS last_archive_age
			FROM pg_stat_archiver
			`,
		},
	},
	"pg_stat_bgwriter": {
		{
			semver.MustParseRange(">=0.0.0"),
			`
			SELECT * FROM pg_stat_bgwriter
			`,
		},
	},
	"pg_stat_activity": {
		// This query only works
		{
			semver.MustParseRange(">=9.2.0"),
			`
			SELECT
				pg_database.datname,
				tmp.state,
				COALESCE(count,0) as count,
				COALESCE(max_tx_duration,0) as max_tx_duration
			FROM
				(
				  VALUES ('active'),
				  		 ('idle'),
				  		 ('idle in transaction'),
				  		 ('idle in transaction (aborted)'),
				  		 ('fastpath function call'),
				  		 ('disabled')
				) AS tmp(state) CROSS JOIN pg_database
			LEFT JOIN
			(
				SELECT
					datname,
					state,
					count(*) AS count,
					MAX(EXTRACT(EPOCH FROM now() - xact_start))::float AS max_tx_duration
				FROM pg_stat_activity GROUP BY datname,state) AS tmp2
				ON tmp.state = tmp2.state AND pg_database.datname = tmp2.datname
			`,
		},
		{
			semver.MustParseRange("<9.2.0"),
			`
			SELECT
				datname,
				'unknown' AS state,
				COALESCE(count(*),0) AS count,
				COALESCE(MAX(EXTRACT(EPOCH FROM now() - xact_start))::float,0) AS max_tx_duration
			FROM pg_stat_activity GROUP BY datname
			`,
		},
	},
}

type Postgresql struct {
	Dsn                      string
	server                   *Server
	Version                  semver.Version
	Namespace                []string
	queryOverrides           map[string]string
	ExcludeDatabases         []string
	GatherPgSetting          bool
	GatherPgStatReplication  bool
	GatherPgReplicationSlots bool
	GatherPgStatArchiver     bool
}

var ignoredColumns = map[string]bool{"stats_reset": true}

var sampleConfig = `
  ## specify address via a url matching:
  ##   postgres://[pqgotest[:password]]@localhost[/dbname]\
  ##       ?sslmode=[disable|verify-ca|verify-full]
  ## or a simple string:
  ##   host=localhost user=pqgotest password=... sslmode=... dbname=app_production
  ##
  ## All connection parameters are optional.
  ##
  ## Without the dbname parameter, the driver will default to a database
  ## with the same name as the user. This dbname is just for instantiating a
  ## connection with the server and doesn't restrict the databases we are trying
  ## to grab metrics for.
  ##
  address = "host=localhost user=postgres sslmode=disable"
  ## A custom name for the database that will be used as the "server" tag in the
  ## measurement output. If not specified, a default one generated from
  ## the connection address is used.
  # outputaddress = "db01"
  ## connection configuration.
  ## maxlifetime - specify the maximum lifetime of a connection.
  ## default is forever (0s)
  max_lifetime = "0s"
  ## A  list of databases to explicitly ignore.  If not specified, metrics for all
  ## databases are gathered.  Do NOT use with the 'databases' option.
  # ignored_databases = ["postgres", "template0", "template1"]
  ## A list of databases to pull metrics about. If not specified, metrics for all
  ## databases are gathered.  Do NOT use with the 'ignored_databases' option.
  # databases = ["app_production", "testing"]
`

// Regex used to get the "short-version" from the postgres version field.
var versionRegex = regexp.MustCompile(`^\w+ ((\d+)(\.\d+)?(\.\d+)?)`)
var lowestSupportedVersion = semver.MustParse("9.1.0")

// Parses the version of postgres into the short version string we can use to
// match behaviors.
func parseVersion(versionString string) (semver.Version, error) {
	submatches := versionRegex.FindStringSubmatch(versionString)
	if len(submatches) > 1 {
		return semver.ParseTolerant(submatches[1])
	}
	return semver.Version{},
		errors.New(fmt.Sprintln("Could not find a postgres version in string:", versionString))
}

func makeQueryOverrideMap(pgVersion semver.Version, queryOverrides map[string][]OverrideQuery) map[string]string {
	resultMap := make(map[string]string)
	for name, overrideDef := range queryOverrides {
		// Find a matching semver. We make it an error to have overlapping
		// ranges at test-time, so only 1 should ever match.
		matched := false
		for _, queryDef := range overrideDef {
			if queryDef.versionRange(pgVersion) {
				resultMap[name] = queryDef.query
				matched = true
				break
			}
		}
		if !matched {
			resultMap[name] = ""
		}
	}
	return resultMap
}

// 检查版本号以及使用的sql
func (p *Postgresql) checkMapVersions(server *Server) error {
	var versionString string
	versionRow := server.db.QueryRow("SELECT version();")
	err := versionRow.Scan(&versionString)
	if err != nil {
		return err
	}
	p.Version, err = parseVersion(versionString)
	if err != nil {
		return err
	}

	if p.Version.LT(lowestSupportedVersion) {
		return errors.New(fmt.Sprintf("database version too low, version:%s, lowest version:%s", p.Version.String(), lowestSupportedVersion.String()))
	}

	// 确定版本号与SQL
	p.queryOverrides = makeQueryOverrideMap(p.Version, queryOverrides)
	return nil
}

func (p *Postgresql) SampleConfig() string {
	return sampleConfig
}

func (p *Postgresql) Description() string {
	return "Read metrics from one or many postgresql servers"
}

func (p *Postgresql) IgnoredColumns() map[string]bool {
	return ignoredColumns
}

func (p *Postgresql) Gather(acc telegraf.Accumulator) error {
	var err error
	p.server, err = NewServer(p.Dsn)
	if err != nil {
		acc.AddGauge("postgresql", map[string]interface{}{"up": false}, nil)
		return fmt.Errorf("Error opening connection to database (%s): %s", loggableDSN(p.Dsn), err.Error())
	} else {
		defer p.server.Close()
	}
	// 测试连通性
	err = p.server.Ping()
	if err != nil {
		acc.AddGauge("postgresql", map[string]interface{}{"up": false}, nil)
		return fmt.Errorf("error opening connection to database (%s): %s", loggableDSN(p.Dsn), err.Error())
	}
	acc.AddGauge("postgresql", map[string]interface{}{"up": true}, nil)
	dsnURI, err := url.Parse(p.Dsn)
	if err != nil {
		errObj := fmt.Errorf("parse dsn:%s failed,plz recheck", p.Dsn)
		acc.AddError(errObj)
		return errObj
	}
	host := dsnURI.Host
	// 检查当前版本，并提取出对应的sql
	err = p.checkMapVersions(p.server)
	if err != nil {
		acc.AddGauge("postgresql", map[string]interface{}{"collect_success": false}, nil)
		return err
	}
	// 获取该server的数据
	err = queryNamespaceMapping(p, p.server, acc, host)
	if err != nil {
		acc.AddGauge("postgresql", map[string]interface{}{"collect_success": false}, nil)
		return err
	}
	// 查询pg_settings
	if p.GatherPgSetting {
		err = querySettings(host, p.server, acc)
		if err != nil {
			acc.AddGauge("postgresql", map[string]interface{}{"collect_success": false}, nil)
			return err
		}
	}
	acc.AddGauge("postgresql", map[string]interface{}{"collect_success": true}, nil)
	return nil
}

// Query within a namespace mapping and emit metrics. Returns fatal errors if
// the scrape fails, and a slice of errors if they were non-fatal.
func queryNamespaceMapping(p *Postgresql, server *Server, acc telegraf.Accumulator, host string) error {
	var namespaces = []string{"pg_stat_bgwriter", "pg_stat_database", "pg_stat_database_count", "pg_locks", "pg_stat_activity"}
	if p.GatherPgReplicationSlots {
		namespaces = append(namespaces, "pg_replication_slots")
	}
	if p.GatherPgStatArchiver {
		namespaces = append(namespaces, "pg_stat_archiver")
	}
	if p.GatherPgStatReplication {
		namespaces = append(namespaces, "pg_stat_replication")
	}
	var rows *sql.Rows
	var err error
	for _, namespace := range namespaces {
		mapping := builtinMetricMaps[namespace]
		query := p.queryOverrides[namespace]
		rows, err = server.db.Query(query)
		if err != nil {

			return err
		}
		var columnNames []string
		columnNames, err = rows.Columns()
		if err != nil {
			return fmt.Errorf("get namespace:%s column failed, detail:%s", namespace, err.Error())
		}
		// Make a lookup map for the column indices
		var columnIdx = make(map[string]int, len(columnNames))
		for i, n := range columnNames {
			columnIdx[n] = i
		}
		var columnData = make([]interface{}, len(columnNames))
		var scanArgs = make([]interface{}, len(columnNames))
		for i := range columnData {
			scanArgs[i] = &columnData[i]
		}

		for rows.Next() {
			var tags = make(map[string]string, 0)
			tags["server"] = host
			var counterFields []map[string]interface{}
			var gaugeFields []map[string]interface{}
			err = rows.Scan(scanArgs...)
			if err != nil {
				return errors.New(fmt.Sprintln("Error retrieving rows:", namespace, err))
			}
			// Loop over column names, and match to scan data. Unknown columns
			// will be filled with an untyped metric number *if* they can be
			// converted to float64s. NULLs are allowed and treated as NaN.
			for idx, columnName := range columnNames {
				if metricMapping, ok := mapping.columnMappings[columnName]; ok {
					switch metricMapping.usage {
					case DISCARD:
						continue
					case LABEL:
						v, ok := dbToString(columnData[idx])
						if !ok {
							acc.AddError(fmt.Errorf("columnName: %s value:%v, %s", columnName, columnData[idx], "类型错误"))
						}

						tags[columnName] = v
					case COUNTER:
						counterFields = append(counterFields, map[string]interface{}{columnName: columnData[idx]})
					case GAUGE:
						gaugeFields = append(gaugeFields, map[string]interface{}{columnName: columnData[idx]})
					}
				}
			}
			// 处理数据
			if len(counterFields) > 0 {
				for _, counterField := range counterFields {
					acc.AddCounter("postgresql_"+namespace, counterField, tags)
				}
			}
			if len(gaugeFields) > 0 {
				for _, gaugeField := range gaugeFields {
					acc.AddGauge("postgresql_"+namespace, gaugeField, tags)
				}
			}
		}

		if rows != nil {
			_ = rows.Close()
		}
	}

	return nil
}

func init() {
	inputs.Add("postgresql", func() telegraf.Input {
		return &Postgresql{
			GatherPgSetting:          false,
			GatherPgStatReplication:  false,
			GatherPgReplicationSlots: false,
			GatherPgStatArchiver:     false,
			ExcludeDatabases:         []string{},
		}
	})
}
