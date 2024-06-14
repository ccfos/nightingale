package hash

import (
	prommodel "github.com/prometheus/common/model"
	"github.com/toolkits/pkg/str"
)

func GetHash2(m prommodel.Metric, ref string) string {
	var s string
	for k, v := range m {
		s += "/"
		s += string(k)
		s += "/"
		s += string(v)
	}
	s += "/"
	s += ref
	return str.MD5(s)
}

func GetTagHash2(m prommodel.Metric) string {
	var s string
	for k, v := range m {
		if k == "__name__" {
			continue
		}

		s += "/"
		s += string(k)
		s += "/"
		s += string(v)
	}
	return str.MD5(s)
}
