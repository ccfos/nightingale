package models

// 0 0 0 0 0 0 处理中
// 0 0 0 0 x 1 已发送
// 0 0 0 0 1 x 已回调
// 0 0 0 1 0 0 已屏蔽
// 0 0 1 0 0 0 被收敛
// 0 1 0 0 x 0 无接收人
// 1 0 0 0 x 0 升级发送
const (
	FLAG_SEND = iota
	FLAG_CALLBACK
	FLAG_MASK
	FLAG_CONVERGE
	FLAG_NONEUSER
	FLAG_UPGRADE
)

const (
	STATUS_DOING    = "doing"     // 处理中
	STATUS_SEND     = "send"      // 已发送
	STATUS_NONEUSER = "none-user" // 无接收人
	STATUS_CALLBACK = "callback"  // 已回调
	STATUS_MASK     = "mask"      // 已屏蔽
	STATUS_CONVERGE = "converge"  // 频率限制
	STATUS_UPGRADE  = "upgrade"   // 升级报警
)

func StatusConvert(s []string) []string {
	status := []string{}

	for i := 0; i < len(s); i++ {
		switch s[i] {
		case STATUS_DOING:
			status = append(status, "处理中")
		case STATUS_SEND:
			status = append(status, "已发送")
		case STATUS_NONEUSER:
			status = append(status, "无接收人")
		case STATUS_CALLBACK:
			status = append(status, "已回调")
		case STATUS_MASK:
			status = append(status, "已屏蔽")
		case STATUS_CONVERGE:
			status = append(status, "已收敛")
		case STATUS_UPGRADE:
			status = append(status, "已升级")
		}
	}

	return status
}

func GetStatus(status string) int {
	switch status {
	case STATUS_SEND:
		return 1 << FLAG_SEND
	case STATUS_NONEUSER:
		return 1 << FLAG_NONEUSER
	case STATUS_CALLBACK:
		return 1 << FLAG_CALLBACK
	case STATUS_MASK:
		return 1 << FLAG_MASK
	case STATUS_CONVERGE:
		return 1 << FLAG_CONVERGE
	case STATUS_UPGRADE:
		return 1 << FLAG_UPGRADE
	}

	return 0
}

func GetFlagsByStatus(ss []string) []uint16 {
	if len(ss) == 0 {
		return []uint16{}
	}

	flags := make(map[string][]uint16)
	for _, s := range ss {
		switch s {
		case STATUS_DOING:
			flags[s] = getDoing()
		case STATUS_SEND:
			flags[s] = getSend()
		case STATUS_NONEUSER:
			flags[s] = getNoneUser()
		case STATUS_CALLBACK:
			flags[s] = getCallback()
		case STATUS_MASK:
			flags[s] = getMask()
		case STATUS_CONVERGE:
			flags[s] = getConverge()
		case STATUS_UPGRADE:
			flags[s] = getUpgrade()
		}
	}
	uss := make([][]uint16, 0)
	for _, s := range flags {
		uss = append(uss, s)
	}
	return interSection(uss)
}

func GetStatusByFlag(flag uint16) []string {
	ret := make([]string, 0)

	if flag == 0 {
		ret = append(ret, STATUS_DOING)
		return ret
	}

	if (flag>>FLAG_UPGRADE)&0x01 == 1 {
		ret = append(ret, STATUS_UPGRADE)
	}

	if (flag>>FLAG_CONVERGE)&0x01 == 1 {
		ret = append(ret, STATUS_CONVERGE)
		return ret
	}

	if (flag>>FLAG_MASK)&0x01 == 1 {
		ret = append(ret, STATUS_MASK)
		return ret
	}

	if (flag>>FLAG_SEND)&0x01 == 1 {
		ret = append(ret, STATUS_SEND)
	}

	if (flag>>FLAG_CALLBACK)&0x01 == 1 {
		ret = append(ret, STATUS_CALLBACK)
	}

	if (flag>>FLAG_NONEUSER)&0x01 == 1 {
		ret = append(ret, STATUS_NONEUSER)
	}

	return ret
}

// 0 0 0 0 0 0 正在处理
func getDoing() []uint16 {
	return []uint16{0}
}

// x 0 0 0 x 1 已发送
func getSend() []uint16 {
	return []uint16{1, 3, 33, 35}
}

// x 0 0 0 1 x 已回调
func getCallback() []uint16 {
	return []uint16{2, 3, 34, 35}
}

// 0 0 0 1 0 0 已屏蔽
func getMask() []uint16 {
	return []uint16{4}
}

// x 0 1 0 0 0 被收敛
func getConverge() []uint16 {
	return []uint16{8, 40}
}

// x 1 0 0 x 0 无接收人
func getNoneUser() []uint16 {
	return []uint16{16, 18, 48, 50}
}

// 1 x x 0 x x 已升级
func getUpgrade() []uint16 {
	return []uint16{32, 33, 34, 35, 40, 41, 42, 43, 48, 49, 50, 51, 56, 57, 58, 59}
}

func interSection(ss [][]uint16) []uint16 {
	if len(ss) == 0 {
		return []uint16{}
	}
	umap := make(map[uint16]int)
	for _, s := range ss {
		for _, su := range s {
			if _, found := umap[su]; found {
				umap[su] += 1
			} else {
				umap[su] = 1
			}
		}
	}
	ret := []uint16{}
	for su, cnt := range umap {
		if cnt == len(ss) {
			ret = append(ret, su)
		}
	}
	return ret
}
