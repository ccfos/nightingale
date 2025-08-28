package cfg

import "os"

type scanner struct {
	data []byte
	err  error
}

func NewFileScanner() *scanner {
	return &scanner{}
}

func (s *scanner) Err() error {
	return s.err
}

func (s *scanner) Data() []byte {
	return s.data
}

func (s *scanner) Read(file string) {
	if s.err == nil {
		s.data, s.err = os.ReadFile(file)
	}
}
