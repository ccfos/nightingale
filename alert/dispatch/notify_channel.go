package dispatch

// NotifyChannels channelKey -> bool
type NotifyChannels map[string]bool

func NewNotifyChannels(channels []string) NotifyChannels {
	nc := make(NotifyChannels)
	for _, ch := range channels {
		nc[ch] = true
	}
	return nc
}

func (nc NotifyChannels) OrMerge(other NotifyChannels) {
	nc.merge(other, func(a, b bool) bool { return a || b })
}

func (nc NotifyChannels) AndMerge(other NotifyChannels) {
	nc.merge(other, func(a, b bool) bool { return a && b })
}

func (nc NotifyChannels) merge(other NotifyChannels, f func(bool, bool) bool) {
	if other == nil {
		return
	}
	for k, v := range other {
		if curV, has := nc[k]; has {
			nc[k] = f(curV, v)
		} else {
			nc[k] = v
		}
	}
}
