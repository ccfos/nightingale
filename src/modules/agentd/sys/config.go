package sys

type SysSection struct {
	Enable           bool                `yaml:"enable"`
	IfacePrefix      []string            `yaml:"ifacePrefix"`
	MountCollect     MountSection        `yaml:"mountCollect"`
	IgnoreMetrics    []string            `yaml:"ignoreMetrics"`
	IgnoreMetricsMap map[string]struct{} `yaml:"-"`
	NtpServers       []string            `yaml:"ntpServers"`
	Plugin           string              `yaml:"plugin"`
	PluginRemote     bool                `yaml:"pluginRemote"`
	Interval         int                 `yaml:"interval"`
	Timeout          int                 `yaml:"timeout"`
	FsRWEnable       bool                `yaml:"fsRWEnable"`
}

type MountSection struct {
	TypePrefix    string    `yaml:"typePrefix"`
	IgnorePrefix  []string  `yaml:"ignorePrefix"`
	Exclude []string `yaml:"exclude"`
}

var Config SysSection

func Init(s SysSection) {
	Config = s

	l := len(Config.IgnoreMetrics)
	m := make(map[string]struct{}, l)
	for i := 0; i < l; i++ {
		m[Config.IgnoreMetrics[i]] = struct{}{}
	}

	Config.IgnoreMetricsMap = m
}
