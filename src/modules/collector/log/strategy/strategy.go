package strategy

import (
	"fmt"

	"github.com/didi/nightingale/src/modules/collector/stra"

	"github.com/toolkits/pkg/logger"
)

// 后续开发者切记 : 没有锁，不能修改globalStrategy，更新的时候直接替换，否则会panic
var (
	globalStrategy map[int64]*stra.Strategy
)

func init() {
	globalStrategy = make(map[int64]*stra.Strategy)
}

func Update() error {
	strategies := stra.GetLogCollects()

	err := UpdateGlobalStrategy(strategies)
	if err != nil {
		return err
	}
	logger.Info("Update Strategy end")
	return nil
}

func UpdateGlobalStrategy(sts []*stra.Strategy) error {
	tmpStrategyMap := make(map[int64]*stra.Strategy)
	for _, st := range sts {
		if st.Degree == 0 {
			st.Degree = 6
		}
		tmpStrategyMap[st.ID] = st
	}
	globalStrategy = tmpStrategyMap
	return nil
}

func GetListAll() []*stra.Strategy {
	stmap := GetDeepCopyAll()
	var ret []*stra.Strategy
	for _, st := range stmap {
		ret = append(ret, st)
	}
	return ret
}

func GetDeepCopyAll() map[int64]*stra.Strategy {
	ret := make(map[int64]*stra.Strategy, len(globalStrategy))
	for k, v := range globalStrategy {
		ret[k] = DeepCopyStrategy(v)
	}
	return ret
}

func GetAll() map[int64]*stra.Strategy {
	return globalStrategy
}

func GetByID(id int64) (*stra.Strategy, error) {
	st, ok := globalStrategy[id]

	if !ok {
		return nil, fmt.Errorf("ID: %d is not exists in global Cache", id)
	}
	return st, nil

}

func DeepCopyStrategy(p *stra.Strategy) *stra.Strategy {
	s := stra.Strategy{
		ID:              p.ID,
		Name:            p.Name,
		FilePath:        p.FilePath,
		TimeFormat:      p.TimeFormat,
		Pattern:         p.Pattern,
		MeasurementType: p.MeasurementType,
		Interval:        p.Interval,
		Tags:            stra.DeepCopyStringMap(p.Tags),
		Func:            p.Func,
		Degree:          p.Degree,
		Unit:            p.Unit,
		Comment:         p.Comment,
		Creator:         p.Creator,
		SrvUpdated:      p.SrvUpdated,
		LocalUpdated:    p.LocalUpdated,
	}

	return &s
}
