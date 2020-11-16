// The HealthState provides additional information when the
// health endpoint returns !ok.
enum HealthState {
    REFUSING = 0,
    ACCEPTING = 1,
    STOPPING = 2,
    STOPPED = 3,
}

// The HealthRequestType is the type of health check, as a process may want to
// return that it's running, but not ready for traffic.
enum HealthRequestType {
    // PROCESS indicates that the health check is for checking that
    // the process is up. Handlers should always return "ok".
    PROCESS = 0,

    // TRAFFIC indicates that the health check is for checking whether
    // the process wants to receive traffic. The process may want to reject
    // traffic due to warmup, or before shutdown to avoid in-flight requests
    // when the process exits.
    TRAFFIC = 1,
}

struct HealthRequest {
    1: optional HealthRequestType type
}

struct HealthStatus {
    1: required bool ok
    2: optional string message
    3: optional HealthState state
}

typedef string filename

struct ThriftIDLs {
    // map: filename -> contents
    1: required map<filename, string> idls
    // the entry IDL that imports others
    2: required filename entryPoint
}

struct VersionInfo {
  // short string naming the implementation language
  1: required string language
  // language-specific version string representing runtime or build chain
  2: required string language_version
  // semver version indicating the version of the tchannel library
  3: required string version
}

service Meta {
    // All arguments are optional. The default is a PROCESS health request.
    HealthStatus health(1: HealthRequest hr)

    ThriftIDLs thriftIDL()
    VersionInfo versionInfo()
}
