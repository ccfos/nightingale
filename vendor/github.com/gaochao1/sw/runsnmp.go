package sw

import (
	"strings"
	"time"

	"github.com/gaochao1/gosnmp"
)

func RunSnmp(ip, community, oid, method string, timeout int) (snmpPDUs []gosnmp.SnmpPDU, err error) {
	cur_gosnmp, err := gosnmp.NewGoSNMP(ip, community, gosnmp.Version2c, int64(timeout))

	if err != nil {
		return nil, err
	} else {
		cur_gosnmp.SetTimeout(int64(timeout))
		snmpPDUs, err := ParseSnmpMethod(oid, method, cur_gosnmp)
		if err != nil {
			return nil, err
		} else {
			return snmpPDUs, err
		}
	}

	return
}

func ParseSnmpMethod(oid, method string, cur_gosnmp *gosnmp.GoSNMP) (snmpPDUs []gosnmp.SnmpPDU, err error) {
	var snmpPacket *gosnmp.SnmpPacket

	switch method {
	case "get":
		snmpPacket, err = cur_gosnmp.Get(oid)
		if err != nil {
			return nil, err
		} else {
			snmpPDUs = snmpPacket.Variables
			return snmpPDUs, err
		}
	case "getnext":
		snmpPacket, err = cur_gosnmp.GetNext(oid)
		if err != nil {
			return nil, err
		} else {
			snmpPDUs = snmpPacket.Variables
			return snmpPDUs, err
		}
	default:
		snmpPDUs, err = cur_gosnmp.Walk(oid)
		return snmpPDUs, err
	}

	return
}

func snmpPDUNameToIfIndex(snmpPDUName string) string {
	oidSplit := strings.Split(snmpPDUName, ".")
	curIfIndex := oidSplit[len(oidSplit)-1]
	return curIfIndex
}

func RunSnmpwalk(ip, community, oid string, retry int, timeout int) ([]gosnmp.SnmpPDU, error) {
	method := "getnext"
	oidnext := oid
	var snmpPDUs = []gosnmp.SnmpPDU{}
	var snmpPDU []gosnmp.SnmpPDU
	var err error

	for {
		for i := 0; i < retry; i++ {
			snmpPDU, err = RunSnmp(ip, community, oidnext, method, timeout)
			if len(snmpPDU) > 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			break
		}
		oidnext = snmpPDU[0].Name
		if strings.Contains(oidnext, oid) {
			snmpPDUs = append(snmpPDUs, snmpPDU[0])
		} else {
			break
		}
	}
	return snmpPDUs, err
}
