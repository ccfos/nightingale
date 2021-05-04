package sw

import (
	"bytes"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func ListIfStatsSnmpWalk(ip, community string, timeout int, ignoreIface []string, retry int, ignorePkt bool, ignoreOperStatus bool, ignoreBroadcastPkt bool, ignoreMulticastPkt bool, ignoreDiscards bool, ignoreErrors bool, ignoreUnknownProtos bool, ignoreOutQLen bool) ([]IfStats, error) {

	var ifStatsList []IfStats
	defer func() {
		if r := recover(); r != nil {
			log.Println(ip+" Recovered in ListIfStats_SnmpWalk", r)
		}
	}()
	chIfInMap := make(chan map[string]string)
	chIfOutMap := make(chan map[string]string)

	chIfNameMap := make(chan map[string]string)
	chIfSpeedMap := make(chan map[string]string)

	go WalkIfIn(ip, community, timeout, chIfInMap, retry)
	go WalkIfOut(ip, community, timeout, chIfOutMap, retry)

	go WalkIfName(ip, community, timeout, chIfNameMap, retry)
	go WalkIfSpeed(ip, community, timeout, chIfSpeedMap, retry)

	ifInMap := <-chIfInMap
	ifOutMap := <-chIfOutMap

	ifNameMap := <-chIfNameMap
	ifSpeedMap := <-chIfSpeedMap

	var ifStatusMap map[string]string
	chIfStatusMap := make(chan map[string]string)
	if ignoreOperStatus == false {
		go WalkIfOperStatus(ip, community, timeout, chIfStatusMap, retry)
		ifStatusMap = <-chIfStatusMap
	}

	chIfInPktMap := make(chan map[string]string)
	chIfOutPktMap := make(chan map[string]string)

	var ifInPktMap, ifOutPktMap map[string]string

	if ignorePkt == false {
		go WalkIfInPkts(ip, community, timeout, chIfInPktMap, retry)
		go WalkIfOutPkts(ip, community, timeout, chIfOutPktMap, retry)
		ifInPktMap = <-chIfInPktMap
		ifOutPktMap = <-chIfOutPktMap
	}

	chIfInBroadcastPktMap := make(chan map[string]string)
	chIfOutBroadcastPktMap := make(chan map[string]string)

	var ifInBroadcastPktMap, ifOutBroadcastPktMap map[string]string

	if ignoreBroadcastPkt == false {
		go WalkIfInBroadcastPkts(ip, community, timeout, chIfInBroadcastPktMap, retry)
		go WalkIfOutBroadcastPkts(ip, community, timeout, chIfOutBroadcastPktMap, retry)
		ifInBroadcastPktMap = <-chIfInBroadcastPktMap
		ifOutBroadcastPktMap = <-chIfOutBroadcastPktMap
	}

	chIfInMulticastPktMap := make(chan map[string]string)
	chIfOutMulticastPktMap := make(chan map[string]string)

	var ifInMulticastPktMap, ifOutMulticastPktMap map[string]string

	if ignoreMulticastPkt == false {
		go WalkIfInMulticastPkts(ip, community, timeout, chIfInMulticastPktMap, retry)
		go WalkIfOutMulticastPkts(ip, community, timeout, chIfOutMulticastPktMap, retry)
		ifInMulticastPktMap = <-chIfInMulticastPktMap
		ifOutMulticastPktMap = <-chIfOutMulticastPktMap
	}

	chIfInDiscardsMap := make(chan map[string]string)
	chIfOutDiscardsMap := make(chan map[string]string)

	var ifInDiscardsMap, ifOutDiscardsMap map[string]string

	if ignoreDiscards == false {
		go WalkIfInDiscards(ip, community, timeout, chIfInDiscardsMap, retry)
		go WalkIfOutDiscards(ip, community, timeout, chIfOutDiscardsMap, retry)
		ifInDiscardsMap = <-chIfInDiscardsMap
		ifOutDiscardsMap = <-chIfOutDiscardsMap
	}

	chIfInErrorsMap := make(chan map[string]string)
	chIfOutErrorsMap := make(chan map[string]string)

	var ifInErrorsMap, ifOutErrorsMap map[string]string

	if ignoreErrors == false {
		go WalkIfInErrors(ip, community, timeout, chIfInErrorsMap, retry)
		go WalkIfOutErrors(ip, community, timeout, chIfOutErrorsMap, retry)
		ifInErrorsMap = <-chIfInErrorsMap
		ifOutErrorsMap = <-chIfOutErrorsMap
	}
	//UnknownProtos
	chIfInUnknownProtosMap := make(chan map[string]string)

	var ifInUnknownProtosMap map[string]string

	if ignoreUnknownProtos == false {
		go WalkIfInUnknownProtos(ip, community, timeout, chIfInUnknownProtosMap, retry)
		ifInUnknownProtosMap = <-chIfInUnknownProtosMap
	}
	//QLen
	chIfOutQLenMap := make(chan map[string]string)

	var ifOutQLenMap map[string]string

	if ignoreOutQLen == false {
		go WalkIfOutQLen(ip, community, timeout, chIfOutQLenMap, retry)
		ifOutQLenMap = <-chIfOutQLenMap
	}

	if len(ifNameMap) > 0 && len(ifInMap) > 0 && len(ifOutMap) > 0 {

		now := time.Now().Unix()

		for ifIndex, ifName := range ifNameMap {

			check := true
			if len(ignoreIface) > 0 {
				for _, ignore := range ignoreIface {
					if strings.Contains(ifName, ignore) {
						check = false
						break
					}
				}
			}

			if check {
				var ifStats IfStats
				var ifstatus_string string
				ifStats.IfIndex, _ = strconv.Atoi(ifIndex)
				ifStats.IfHCInOctets, _ = strconv.ParseUint(ifInMap[ifIndex], 10, 64)
				ifStats.IfHCOutOctets, _ = strconv.ParseUint(ifOutMap[ifIndex], 10, 64)

				if ignorePkt == false {
					ifStats.IfHCInUcastPkts, _ = strconv.ParseUint(ifInPktMap[ifIndex], 10, 64)
					ifStats.IfHCOutUcastPkts, _ = strconv.ParseUint(ifOutPktMap[ifIndex], 10, 64)
				}
				if ignoreBroadcastPkt == false {
					ifStats.IfHCInBroadcastPkts, _ = strconv.ParseUint(ifInBroadcastPktMap[ifIndex], 10, 64)
					ifStats.IfHCOutBroadcastPkts, _ = strconv.ParseUint(ifOutBroadcastPktMap[ifIndex], 10, 64)
				}
				if ignoreMulticastPkt == false {
					ifStats.IfHCInMulticastPkts, _ = strconv.ParseUint(ifInMulticastPktMap[ifIndex], 10, 64)
					ifStats.IfHCOutMulticastPkts, _ = strconv.ParseUint(ifOutMulticastPktMap[ifIndex], 10, 64)
				}
				if ignoreDiscards == false {
					ifStats.IfInDiscards, _ = strconv.Atoi(ifInDiscardsMap[ifIndex])
					ifStats.IfOutDiscards, _ = strconv.Atoi(ifOutDiscardsMap[ifIndex])
				}
				if ignoreErrors == false {
					ifStats.IfInErrors, _ = strconv.Atoi(ifInErrorsMap[ifIndex])
					ifStats.IfOutErrors, _ = strconv.Atoi(ifOutErrorsMap[ifIndex])
				}
				if ignoreUnknownProtos == false {
					ifStats.IfInUnknownProtos, _ = strconv.Atoi(ifInUnknownProtosMap[ifIndex])
				}
				if ignoreOutQLen == false {
					ifStats.IfOutQLen, _ = strconv.Atoi(ifOutQLenMap[ifIndex])
				}

				ifStats.IfSpeed, _ = strconv.Atoi(ifSpeedMap[ifIndex])
				ifStats.IfSpeed = 1000 * 1000 * ifStats.IfSpeed

				if ignoreOperStatus == false {
					ifstatus_string = ifStatusMap[ifIndex]
					ifstatus_string = strings.TrimSpace(ifstatus_string)
					ifstatus := ifstatus_string[(len(ifstatus_string) - 2):(len(ifstatus_string) - 1)]
					ifStats.IfOperStatus, _ = strconv.Atoi(ifstatus)
				}
				ifStats.TS = now

				ifName = strings.Replace(ifName, `"`, "", -1)
				ifStats.IfName = ifName

				ifStatsList = append(ifStatsList, ifStats)
			}
		}
	}

	return ifStatsList, nil
}

func WalkIfOperStatus(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifOperStatusOid, community, timeout, retry, ch)
}

func WalkIfName(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifNameOid, community, timeout, retry, ch)
}

func WalkIfIn(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifHCInOid, community, timeout, retry, ch)
}

func WalkIfOut(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifHCOutOid, community, timeout, retry, ch)
}

func WalkIfInPkts(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifHCInPktsOid, community, timeout, retry, ch)
}

func WalkIfOutPkts(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifHCOutPktsOid, community, timeout, retry, ch)
}

func WalkIfInBroadcastPkts(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifHCInBroadcastPktsOid, community, timeout, retry, ch)
}

func WalkIfOutBroadcastPkts(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifHCOutBroadcastPktsOid, community, timeout, retry, ch)
}

func WalkIfInMulticastPkts(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifHCInMulticastPktsOid, community, timeout, retry, ch)
}

func WalkIfOutMulticastPkts(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifHCOutMulticastPktsOid, community, timeout, retry, ch)
}

func WalkIfInDiscards(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifInDiscardsOid, community, timeout, retry, ch)
}

func WalkIfOutDiscards(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifOutDiscardsOid, community, timeout, retry, ch)
}

func WalkIfInErrors(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifInErrorsOid, community, timeout, retry, ch)
}

func WalkIfOutErrors(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifOutErrorsOid, community, timeout, retry, ch)
}

func WalkIfInUnknownProtos(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifInUnknownProtosOid, community, timeout, retry, ch)
}

func WalkIfOutQLen(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifOutQLenOid, community, timeout, retry, ch)
}

func WalkIfSpeed(ip, community string, timeout int, ch chan map[string]string, retry int) {
	WalkIf(ip, ifSpeedOid, community, timeout, retry, ch)
}

func WalkIf(ip, oid, community string, timeout, retry int, ch chan map[string]string) {
	result := make(map[string]string)

	for i := 0; i < retry; i++ {
		out, err := CmdTimeout(timeout, "snmpwalk", "-v", "2c", "-c", community, ip, oid)

		var list []string
		if strings.Contains(out, "IF-MIB") {
			list = strings.Split(out, "IF-MIB")
		} else {
			list = strings.Split(out, "iso")
		}

		for _, v := range list {

			defer func() {
				if r := recover(); r != nil {
					log.Println("Recovered in WalkIf", r)
				}
			}()

			if len(v) > 0 && strings.Contains(v, "=") {
				vt := strings.Split(v, "=")

				var ifIndex, ifValue string
				if strings.Contains(vt[0], ".") {
					leftList := strings.Split(vt[0], ".")
					ifIndex = leftList[len(leftList)-1]
					ifIndex = strings.TrimSpace(ifIndex)
				}

				if strings.Contains(vt[1], ":") {
					ifValue = strings.Split(vt[1], ":")[1]
					ifValue = strings.TrimSpace(ifValue)
				}

				result[ifIndex] = ifValue
			}
		}

		if len(result) > 0 {
			ch <- result
			return
		}
		if err != nil && i == (retry-1) {
			log.Println(ip, oid, err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	ch <- result
	return
}

func CmdTimeout(timeout int, name string, arg ...string) (string, error) {
	cmd := exec.Command(name, arg...)

	var out bytes.Buffer
	cmd.Stdout = &out

	cmd.Start()
	timer := time.AfterFunc(time.Duration(timeout)*time.Millisecond, func() {
		err := cmd.Process.Kill()
		if err != nil {
			log.Println("failed to kill: ", err)
		}
	})
	err := cmd.Wait()
	timer.Stop()

	return out.String(), err
}
