package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/toolkits/pkg/logger"
)

type WhiteList struct {
	Id         int64  `json:"id"`
	StartIp    string `json:"startIp"`
	StartIpInt int64  `json:"-"`
	EndIp      string `json:"endIp"`
	EndIpInt   int64  `json:"-"`
	StartTime  int64  `json:"startTime"`
	EndTime    int64  `json:"endTime"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updateAt"`
	Creator    string `json:"creator"`
	Updater    string `json:"updater"`
}

func WhiteListAccess(addr string) error {
	ip := parseIPv4(addr)
	if ip == 0 {
		return fmt.Errorf("invalid remote address %s", addr)
	}
	logger.Debugf("WhiteListAccess htol(%s) %d", addr, ip)
	now := time.Now().Unix()
	count, _ := DB["rdb"].Where("start_ip_int<=? and end_ip_int>=? and start_time<=? and end_time>=?", ip, ip, now, now).Count(new(WhiteList))
	if count == 0 {
		return fmt.Errorf("access deny from %s", addr)
	}

	return nil
}

const big = 0xFFFFFF

func dtoi(s string) (n int, i int, ok bool) {
	n = 0
	for i = 0; i < len(s) && '0' <= s[i] && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
		if n >= big {
			return big, i, false
		}
	}
	if i == 0 {
		return 0, 0, false
	}
	return n, i, true
}

func parseIPv4(s string) uint32 {
	var p [4]uint32
	for i := 0; i < 4; i++ {
		if len(s) == 0 {
			// Missing octets.
			return 0
		}
		if i > 0 {
			if s[0] != '.' {
				return 0
			}
			s = s[1:]
		}
		n, c, ok := dtoi(s)
		if !ok || n > 0xFF {
			return 0
		}
		s = s[c:]
		p[i] = uint32(n)
	}
	if len(s) != 0 {
		return 0
	}
	return p[0]<<24 + p[1]<<16 + p[2]<<8 + p[3]
}

func (p *WhiteList) Validate() error {
	if p.StartIpInt = int64(parseIPv4(p.StartIp)); p.StartIpInt == 0 {
		return fmt.Errorf("invalid start ip %s", p.StartIp)
	}
	if p.EndIpInt = int64(parseIPv4(p.EndIp)); p.EndIpInt == 0 {
		return fmt.Errorf("invalid end ip %s", p.EndIp)
	}
	return nil
}

func WhiteListTotal(query string) (int64, error) {
	if query != "" {
		q := "%" + query + "%"
		return DB["rdb"].Where("start_ip like ? or end_ip like ?", q, q).Count(new(WhiteList))
	}

	return DB["rdb"].Count(new(WhiteList))
}

func WhiteListGets(query string, limit, offset int) ([]WhiteList, error) {
	session := DB["rdb"].Desc("id").Limit(limit, offset)
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("start_ip like ? or end_ip like ?", q, q)
	}

	var objs []WhiteList
	err := session.Find(&objs)
	return objs, err
}

func WhiteListGet(where string, args ...interface{}) (*WhiteList, error) {
	var obj WhiteList
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, errors.New("whiteList not found")
	}

	return &obj, nil
}

func (p *WhiteList) Save() error {
	_, err := DB["rdb"].Insert(p)
	return err
}
func (p *WhiteList) Update(cols ...string) error {
	_, err := DB["rdb"].Where("id=?", p.Id).Cols(cols...).Update(p)
	return err
}
func (p *WhiteList) Del() error {
	_, err := DB["rdb"].Where("id=?", p.Id).Delete(new(WhiteList))
	return err
}
