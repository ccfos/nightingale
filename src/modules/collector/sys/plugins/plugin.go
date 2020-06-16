package plugins

type Plugin struct {
	FilePath string
	Params   string
	Env      string
	Stdin    string
	MTime    int64
	Cycle    int
}

var (
	Plugins              = make(map[string]*Plugin)
	PluginsWithScheduler = make(map[string]*PluginScheduler)
)

func DelNoUsePlugins(newPlugins map[string]*Plugin) {
	for currKey, currPlugin := range Plugins {
		newPlugin, ok := newPlugins[currKey]
		if !ok || currPlugin.MTime != newPlugin.MTime {
			deletePlugin(currKey)
		}
	}
}

func AddNewPlugins(newPlugins map[string]*Plugin) {
	for key, newPlugin := range newPlugins {
		if _, ok := Plugins[key]; ok && newPlugin.MTime == Plugins[key].MTime {
			continue
		}

		Plugins[key] = newPlugin
		sch := NewPluginScheduler(newPlugin)
		PluginsWithScheduler[key] = sch
		sch.Schedule()
	}
}

func ClearAllPlugins() {
	for k := range Plugins {
		deletePlugin(k)
	}
}

func deletePlugin(key string) {
	v, ok := PluginsWithScheduler[key]
	if ok {
		v.Stop()
		delete(PluginsWithScheduler, key)
	}
	delete(Plugins, key)
}
