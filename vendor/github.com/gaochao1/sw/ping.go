package sw

func PingRtt(ip string, timeout int, fastPingMode bool) (float64, error) {
	var rtt float64
	var err error
	if fastPingMode == true {
		rtt, err = fastPingRtt(ip, timeout)
	} else {
		rtt, err = goPingRtt(ip, timeout)
	}
	return rtt, err
}

func Ping(ip string, timeout int, fastPingMode bool) bool {
	rtt, _ := PingRtt(ip, timeout, fastPingMode)
	if rtt == -1 {
		return false
	}
	return true
}
