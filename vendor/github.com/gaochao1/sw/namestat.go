package sw

import (
	"log"
)

func SysName(ip, community string, timeout int) (string, error) {
	oid := "1.3.6.1.2.1.1.5.0"
	method := "get"
	defer func() {
		if r := recover(); r != nil {
			log.Println(ip+" Recovered in SysName", r)
		}
	}()
	snmpPDUs, err := RunSnmp(ip, community, oid, method, timeout)

	if err == nil {
		for _, pdu := range snmpPDUs {
			return pdu.Value.(string), err
		}
	}

	return "", err
}
