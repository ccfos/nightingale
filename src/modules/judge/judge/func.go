package judge

import (
	"fmt"
	"math"

	"github.com/didi/nightingale/src/dataobj"
)

type Function interface {
	Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool)
}

type MaxFunction struct {
	Function
	Limit      int
	Operator   string
	RightValue float64
}

func (this MaxFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	max := vs[0].Value
	for i := 1; i < this.Limit; i++ {
		if max < vs[i].Value {
			max = vs[i].Value
		}
	}

	leftValue = max
	isTriggered = checkIsTriggered(leftValue, this.Operator, this.RightValue)
	return
}

type MinFunction struct {
	Function
	Limit      int
	Operator   string
	RightValue float64
}

func (this MinFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	min := vs[0].Value
	for i := 1; i < this.Limit; i++ {
		if min > vs[i].Value {
			min = vs[i].Value
		}
	}

	leftValue = min
	isTriggered = checkIsTriggered(leftValue, this.Operator, this.RightValue)
	return
}

type AllFunction struct {
	Function
	Limit      int
	Operator   string
	RightValue float64
}

func (this AllFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	isTriggered = true
	for i := 0; i < this.Limit; i++ {
		isTriggered = checkIsTriggered(vs[i].Value, this.Operator, this.RightValue)
		if !isTriggered {
			break
		}
	}

	leftValue = vs[0].Value
	return
}

type SumFunction struct {
	Function
	Limit      int
	Operator   string
	RightValue float64
}

func (this SumFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	sum := dataobj.JsonFloat(0.0)
	for i := 0; i < this.Limit; i++ {
		sum += vs[i].Value
	}

	leftValue = sum
	isTriggered = checkIsTriggered(leftValue, this.Operator, this.RightValue)
	return
}

type AvgFunction struct {
	Function
	Limit      int
	Operator   string
	RightValue float64
}

func (this AvgFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	sum := dataobj.JsonFloat(0.0)
	for i := 0; i < this.Limit; i++ {
		sum += vs[i].Value
	}

	leftValue = sum / dataobj.JsonFloat(this.Limit)
	isTriggered = checkIsTriggered(leftValue, this.Operator, this.RightValue)
	return
}

type DiffFunction struct {
	Function
	Limit      int
	Operator   string
	RightValue float64
}

// 只要有一个点的diff触发阈值，就报警
func (this DiffFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	first := vs[0].Value

	isTriggered = false
	for i := 1; i < this.Limit; i++ {
		// diff是当前值减去历史值
		leftValue = first - vs[i].Value
		isTriggered = checkIsTriggered(leftValue, this.Operator, this.RightValue)
		if isTriggered {
			break
		}
	}

	return
}

// pdiff(#3)
type PDiffFunction struct {
	Function
	Limit      int
	Operator   string
	RightValue float64
}

func (this PDiffFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	first := vs[0].Value
	isTriggered = false
	for i := 1; i < this.Limit; i++ {
		if vs[i].Value == 0 {
			continue
		}

		leftValue = (first - vs[i].Value) / vs[i].Value * 100.0
		isTriggered = checkIsTriggered(leftValue, this.Operator, this.RightValue)
		if isTriggered {
			break
		}
	}

	return
}

type HappenFunction struct {
	Function
	Num        int
	Limit      int
	Operator   string
	RightValue float64
}

func (this HappenFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	for n, i := 0, 0; i < len(vs); i++ {
		if checkIsTriggered(vs[i].Value, this.Operator, this.RightValue) {
			n++
			if n == this.Num {
				isTriggered = true
				leftValue = vs[i].Value
				return
			}
		}
	}
	return
}

type NodataFunction struct {
	Function
}

func (this NodataFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	for _, value := range vs {
		if !math.IsNaN(float64(value.Value)) {
			return value.Value, false
		}
	}
	return 0, true
}

type CAvgAbsFunction struct {
	Function
	Limit        int
	Operator     string
	RightValue   float64
	CompareValue float64
}

func (this CAvgAbsFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	sum := dataobj.JsonFloat(0.0)
	for i := 0; i < this.Limit; i++ {
		sum += vs[i].Value
	}

	value := sum / dataobj.JsonFloat(this.Limit)
	leftValue = dataobj.JsonFloat(math.Abs(float64(value) - float64(this.CompareValue)))

	isTriggered = checkIsTriggered(leftValue, this.Operator, this.RightValue)
	return
}

type CAvgFunction struct {
	Function
	Limit        int
	Operator     string
	RightValue   float64
	CompareValue float64
}

func (this CAvgFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	sum := dataobj.JsonFloat(0.0)
	for i := 0; i < this.Limit; i++ {
		sum += vs[i].Value
	}

	leftValue = sum/dataobj.JsonFloat(this.Limit) - dataobj.JsonFloat(this.CompareValue)

	isTriggered = checkIsTriggered(leftValue, this.Operator, this.RightValue)
	return
}

type CAvgRateAbsFunction struct {
	Function
	Limit        int
	Operator     string
	RightValue   float64
	CompareValue float64
}

func (this CAvgRateAbsFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	sum := dataobj.JsonFloat(0.0)
	for i := 0; i < this.Limit; i++ {
		sum += vs[i].Value
	}

	value := sum / dataobj.JsonFloat(this.Limit)
	leftValue = dataobj.JsonFloat(math.Abs(float64(value) - float64(this.CompareValue)))

	isTriggered = checkIsTriggered(leftValue, this.Operator, this.RightValue)
	return
}

type CAvgRateFunction struct {
	Function
	Limit        int
	Operator     string
	RightValue   float64
	CompareValue float64
}

func (this CAvgRateFunction) Compute(vs []*dataobj.RRDData) (leftValue dataobj.JsonFloat, isTriggered bool) {
	if len(vs) < this.Limit {
		return
	}

	sum := dataobj.JsonFloat(0.0)
	for i := 0; i < this.Limit; i++ {
		sum += vs[i].Value
	}

	value := sum / dataobj.JsonFloat(this.Limit)
	leftValue = (value - dataobj.JsonFloat(this.CompareValue)) / dataobj.JsonFloat(math.Abs(this.CompareValue))

	isTriggered = checkIsTriggered(leftValue, this.Operator, this.RightValue)
	return
}

func ParseFuncFromString(str string, span []interface{}, operator string, rightValue float64) (fn Function, err error) {
	if str == "" {
		return nil, fmt.Errorf("func can not be null!")
	}
	limit := span[0].(int)

	switch str {
	case "max":
		fn = &MaxFunction{Limit: limit, Operator: operator, RightValue: rightValue}
	case "min":
		fn = &MinFunction{Limit: limit, Operator: operator, RightValue: rightValue}
	case "all":
		fn = &AllFunction{Limit: limit, Operator: operator, RightValue: rightValue}
	case "sum":
		fn = &SumFunction{Limit: limit, Operator: operator, RightValue: rightValue}
	case "avg":
		fn = &AvgFunction{Limit: limit, Operator: operator, RightValue: rightValue}
	case "diff":
		fn = &DiffFunction{Limit: limit, Operator: operator, RightValue: rightValue}
	case "pdiff":
		fn = &PDiffFunction{Limit: limit, Operator: operator, RightValue: rightValue}
	case "happen":
		fn = &HappenFunction{Limit: limit, Num: span[1].(int), Operator: operator, RightValue: rightValue}
	case "nodata":
		fn = &NodataFunction{}
	case "c_avg":
		fn = &CAvgFunction{Limit: limit, CompareValue: span[1].(float64), Operator: operator, RightValue: rightValue}
	case "c_avg_abs":
		fn = &CAvgAbsFunction{Limit: limit, CompareValue: span[1].(float64), Operator: operator, RightValue: rightValue}
	case "c_avg_rate":
		fn = &CAvgRateFunction{Limit: limit, CompareValue: span[1].(float64), Operator: operator, RightValue: rightValue}
	case "c_avg_rate_abs":
		fn = &CAvgRateAbsFunction{Limit: limit, CompareValue: span[1].(float64), Operator: operator, RightValue: rightValue}
	default:
		err = fmt.Errorf("not_supported_method")
	}

	return
}

func checkIsTriggered(leftValue dataobj.JsonFloat, operator string, rightValue float64) (isTriggered bool) {
	switch operator {
	case "=", "==":
		isTriggered = math.Abs(float64(leftValue)-rightValue) < 0.0001
	case "!=":
		isTriggered = math.Abs(float64(leftValue)-rightValue) > 0.0001
	case "<":
		isTriggered = float64(leftValue) < rightValue
	case "<=":
		isTriggered = float64(leftValue) <= rightValue
	case ">":
		isTriggered = float64(leftValue) > rightValue
	case ">=":
		isTriggered = float64(leftValue) >= rightValue
	}

	return
}
