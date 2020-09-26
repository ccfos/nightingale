package logger

type multiBackend struct {
	bes []Backend
}

func NewMultiBackend(bes ...Backend) (*multiBackend, error) {
	var b multiBackend
	b.bes = bes
	return &b, nil
}

func (self *multiBackend) Log(s Severity, msg []byte) {
	for _, be := range self.bes {
		be.Log(s, msg)
	}
}

func (self *multiBackend) close() {
	for _, be := range self.bes {
		be.close()
	}
}
