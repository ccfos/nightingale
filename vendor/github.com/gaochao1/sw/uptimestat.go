package sw

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
)

func SysUpTime(ip, community string, timeout int) (string, error) {
	oid := "1.3.6.1.2.1.1.3.0"
	method := "get"
	defer func() {
		if r := recover(); r != nil {
			log.Println(ip+" Recovered in Uptime", r)
		}
	}()
	snmpPDUs, err := RunSnmp(ip, community, oid, method, timeout)

	if err == nil {
		for _, pdu := range snmpPDUs {
			durationStr := parseTime(pdu.Value.(int))
			return durationStr, err
		}
	}

	return "", err
}

func parseTime(d int) string {
	timestr := strconv.Itoa(d / 100)
	duration, _ := time.ParseDuration(timestr + "s")

	totalHour := duration.Hours()
	day := int(totalHour / 24)

	modTime := math.Mod(totalHour, 24)
	modTimeStr := strconv.FormatFloat(modTime, 'f', 3, 64)
	modDuration, _ := time.ParseDuration(modTimeStr + "h")

	modDurationStr := modDuration.String()
	if strings.Contains(modDurationStr, ".") {
		modDurationStr = strings.Split(modDurationStr, ".")[0] + "s"
	}

	return fmt.Sprintf("%dday %s", day, modDurationStr)

}
