package sw

import (
	"log"
	"regexp"
)

func SysModel(ip, community string, timeout int) (string, error) {
	vendor, err := SysVendor(ip, community, timeout)
	method := "get"
	var oid string

	defer func() {
		if r := recover(); r != nil {
			log.Println("Recovered in sw.modelstat.go SysModel", r)
		}
	}()

	switch vendor {
	case "Cisco_NX":
		oid = "1.3.6.1.2.1.47.1.1.1.1.13.10"
	case "Cisco":
		oid = "1.3.6.1.2.1.47.1.1.1.1.13.1001"
	case "Huawei", "H3C", "H3C_V5", "H3C_V7":
		re := regexp.MustCompile(`\w+-\w+-\w+\S+`)
		sysDescr, err := SysDescr(ip, community, timeout)
		if err != nil {
			return "", err
		} else {
			return re.FindAllString(sysDescr, 1)[0], nil
		}
	default:
		return "", err
	}

	snmpPDUs, err := RunSnmp(ip, community, oid, method, timeout)

	if err == nil {
		for _, pdu := range snmpPDUs {
			return pdu.Value.(string), err
		}
	}

	return "", err

}
