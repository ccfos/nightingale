package rpc

// Ping return string 'pong', just for test
func (*Scheduler) Ping(input string, output *string) error {
	*output = "pong"
	return nil
}
