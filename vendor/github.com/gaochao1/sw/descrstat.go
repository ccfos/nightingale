package sw

import (
	"log"
	"strconv"
	"strings"
)

func SysDescr(ip, community string, timeout int) (string, error) {
	oid := "1.3.6.1.2.1.1.1.0"
	method := "get"

	snmpPDUs, err := RunSnmp(ip, community, oid, method, timeout)
	defer func() {
		if r := recover(); r != nil {
			log.Println(ip+" Recovered in sysDescr", r)
		}
	}()
	if err == nil {
		for _, pdu := range snmpPDUs {
			return pdu.Value.(string), err
		}
	}

	return "", err
}

func SysVendor(ip, community string, timeout int) (string, error) {
	sysDescr, err := SysDescr(ip, community, timeout)
	sysDescrLower := strings.ToLower(sysDescr)

	if strings.Contains(sysDescrLower, "cisco nx-os") {
		return "Cisco_NX", err
	}

	if strings.Contains(sysDescr, "Cisco Internetwork Operating System Software") {
		return "Cisco_old", err
	}

	if strings.Contains(sysDescrLower, "cisco ios") {
		if strings.Contains(sysDescr, "IOS-XE Software") {
			return "Cisco_IOS_XE", err
		} else if strings.Contains(sysDescr, "Cisco IOS XR") {
			return "Cisco_IOS_XR", err
		} else {
			return "Cisco", err
		}
	}

	if strings.Contains(sysDescrLower, "cisco adaptive security appliance") {
		version_number, err := strconv.ParseFloat(getVersionNumber(sysDescr), 32)
		if err == nil && version_number < 9.2 {
			return "Cisco_ASA_OLD", err
		}
		return "Cisco_ASA", err
	}
	if strings.Contains(sysDescrLower, "h3c") {
		if strings.Contains(sysDescr, "Software Version 5") {
			return "H3C_V5", err
		}

		if strings.Contains(sysDescr, "Software Version 7") {
			return "H3C_V7", err
		}

		if strings.Contains(sysDescr, "Version S9500") {
			return "H3C_S9500", err
		}

		return "H3C", err
	}

	if strings.Contains(sysDescrLower, "huawei") {
		if strings.Contains(sysDescr, "MultiserviceEngine 60") {
			return "Huawei_ME60", err
		}
		if strings.Contains(sysDescr, "Version 5.70") {
			return "Huawei_V5.70", err
		}
		if strings.Contains(sysDescr, "Version 5.130") {
			return "Huawei_V5.130", err
		}
		if strings.Contains(sysDescr, "Version 3.10") {
			return "Huawei_V3.10", err
		}
		return "Huawei", err
	}

	if strings.Contains(sysDescrLower, "ruijie") {
		return "Ruijie", err
	}

	if strings.Contains(sysDescrLower, "juniper networks") {
		return "Juniper", err
	}

	if strings.Contains(sysDescrLower, "dell networking") {
		return "Dell", err
	}

	if strings.Contains(sysDescrLower, "linux") {
		return "Linux", err
	}

	return "", err
}

func getVersionNumber(sysdescr string) string {
	version_number := ""
	s := strings.Fields(sysdescr)
	for index, value := range s {
		if strings.ToLower(value) == "version" {
			version_number = s[index+1]
		}
	}
	version_number = strings.Replace(version_number, "(", "", -1)
	version_number = strings.Replace(version_number, ")", "", -1)
	return version_number
}
