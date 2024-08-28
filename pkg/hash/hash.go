package hash

import (
	"sort"
	"strings"

	prommodel "github.com/prometheus/common/model"
	"github.com/spaolacci/murmur3"
)

func GetHash(m prommodel.Metric, ref string) uint64 {
	var str string
	var strs []string
	// get keys from m
	for k, _ := range m {
		strs = append(strs, string(k))
	}

	// sort keys use sort
	sort.Strings(strs)

	for _, k := range strs {
		str += "/"
		str += k
		str += "/"
		str += string(m[prommodel.LabelName(k)])
	}
	str += "/"
	str += ref

	return murmur3.Sum64([]byte(str))
}

func GetTagHash(m prommodel.Metric) uint64 {
	var str string
	var strs []string
	// get keys from m
	for k, _ := range m {
		if k == "__name__" {
			continue
		}
		strs = append(strs, string(k))
	}

	// sort keys use sort
	sort.Strings(strs)

	for _, k := range strs {
		str += "/"
		str += k
		str += "/"
		str += string(m[prommodel.LabelName(k)])
	}

	return murmur3.Sum64([]byte(str))
}

func GetTargetTagHash(m prommodel.Metric, target []string) uint64 {
	builder := strings.Builder{}
	for _, k := range target {
		builder.WriteString("/")
		builder.WriteString(k)
		builder.WriteString("/")
		builder.WriteString(string(m[prommodel.LabelName(k)]))
	}
	return murmur3.Sum64([]byte(builder.String()))
}
