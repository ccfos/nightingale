package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/toolkits/stack"
)

type AggrCalc struct {
	Id               int64     `xorm:"id pk autoincr" json:"id"`
	Nid              int64     `xorm:"nid" json:"nid"`
	Category         int       `xorm:"category" json:"category"`
	NewMetric        string    `xorm:"new_metric" json:"new_metric"`
	NewStep          int       `xorm:"new_step" json:"new_step"`
	GroupByString    string    `xorm:"groupby" json:"-"`
	RawMetricsString string    `xorm:"raw_metrics" json:"-"`
	GlobalOperator   string    `xorm:"global_operator"json:"global_operator"` //指标聚合方式
	Expression       string    `xorm:"expression" json:"expression"`
	RPN              string    `xorm:"rpn" json:"rpn"`     //用途？
	Status           int       `xorm:"status" json:"-"`    //审核状态
	Quota            int       `xorm:"quota" json:"quota"` //用途？
	Comment          string    `xorm:"comment" json:"comment"`
	Creator          string    `xorm:"creator" json:"creator"`
	Created          time.Time `xorm:"created" json:"created"`
	LastUpdator      string    `xorm:"last_updator" json:"last_updator"`
	LastUpdated      time.Time `xorm:"<-" json:"last_updated"`

	RawMetrics []*dataobj.RawMetric `xorm:"-" json:"raw_metrics"`
	GroupBy    []string             `xorm:"-" json:"groupby"`
	VarNum     int                  `xorm:"-" json:"var_num"`
}

type AggrTagsFilter struct {
	TagK string   `json:"tagk"`
	Opt  string   `json:"opt"`
	TagV []string `json:"tagv"`
}

func (a *AggrCalc) Save() error {
	_, err := DB["mon"].InsertOne(a)
	return err
}

func AggrCalcGet(where string, args ...interface{}) (*AggrCalc, error) {
	var obj AggrCalc
	has, err := DB["mon"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func AggrCalcsList(name string, nid int64) ([]*AggrCalc, error) {
	session := DB["mon"].NewSession()
	defer session.Close()

	objs := make([]*AggrCalc, 0)

	whereClause := "1 = 1"
	params := []interface{}{}

	if name != "" {
		whereClause += " AND name LIKE ?"
		params = append(params, "%"+name+"%")
	}

	if nid != 0 {
		whereClause += " AND nid = ?"
		params = append(params, nid)
	}

	err := session.Where(whereClause, params...).Find(&objs)
	if err != nil {
		return objs, err
	}

	stras := make([]*AggrCalc, 0)
	for _, obj := range objs {
		err = obj.Decode()
		if err != nil {
			return stras, err
		}
		stras = append(stras, obj)
	}
	return stras, err
}

func (a *AggrCalc) Update(cols ...string) error {
	session := DB["mon"].NewSession()
	defer session.Close()

	err := session.Begin()
	if err != nil {
		session.Rollback()
		return err
	}

	var obj AggrCalc
	exists, err := session.Id(a.Id).Get(&obj)
	if err != nil {
		session.Rollback()
		return err
	}

	if !exists {
		session.Rollback()
		return fmt.Errorf("%d not exists", a.Id)
	}

	_, err = session.Id(a.Id).Cols(cols...).Update(a)
	if err != nil {
		session.Rollback()
		return err
	}

	straByte, err := json.Marshal(a)
	if err != nil {
		session.Rollback()
		return err
	}

	err = saveHistory(a.Id, "calc", "update", a.Creator, string(straByte), session)
	if err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func AggrCalcDel(id int64) error {
	session := DB["mon"].NewSession()
	defer session.Close()
	var obj AggrCalc

	if err := session.Begin(); err != nil {
		return err
	}

	exists, err := session.Id(id).Get(&obj)
	if err != nil {
		session.Rollback()
		return err
	}

	if !exists {
		session.Rollback()
		return fmt.Errorf("%d not exists", obj.Id)
	}

	if _, err := session.Id(id).Delete(new(AggrCalc)); err != nil {
		session.Rollback()
		return err
	}

	straByte, err := json.Marshal(obj)
	if err != nil {
		session.Rollback()
		return err
	}

	err = saveHistory(obj.Id, "calc", "delete", obj.Creator, string(straByte), session)
	if err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func (a *AggrCalc) Encode() error {

	groupByBytes, err := json.Marshal(a.GroupBy)
	if err != nil {
		return fmt.Errorf("encode GroupBy err:%v", err)
	}
	a.GroupByString = string(groupByBytes)
	a.GlobalOperator = "none"

	if !strings.HasPrefix(a.NewMetric, "aggr.") {
		return fmt.Errorf("新指标名必需以aggr.开头")
	}

	if len(a.RawMetrics) > 100 {
		return fmt.Errorf("待聚合指标过多 > 100")
	}

	for _, metric := range a.RawMetrics {
		if metric.Name == "" {
			return fmt.Errorf("待聚合指标名不能为空")
		}

		if metric.VarID == "" {
			return fmt.Errorf("待聚合指标ID不能为空")
		}

		if !strings.HasPrefix(metric.VarID, "$") && !strings.HasPrefix(metric.VarID, "#") {
			metric.VarID = "$" + metric.VarID
		}

		// check operator
		if !isInList(metric.Opt, []string{"sum", "max", "avg", "min", "count"}) {
			return fmt.Errorf("聚合方法为空或不支持：" + metric.Opt)
		}

		// check tag filter
		for _, filter := range metric.Filters {
			if filter.Opt != "=" && filter.Opt != "!=" {
				fmt.Errorf("过滤操作符无效")
			}

			if filter.TagK == "" {
				fmt.Errorf("tagk不能为空")
			}
			for _, tagv := range filter.TagV {
				if tagv == "" {
					fmt.Errorf("tagv不能为空")
				}
			}
		}

	}

	rawMetricsBytes, err := json.Marshal(a.RawMetrics)
	if err != nil {
		return fmt.Errorf("encode RawMetrics err:%v", err)
	}
	a.RawMetricsString = string(rawMetricsBytes)

	// 把中缀表达式转化为后缀表达式，并check表达式合法性
	if a.Expression == "" { // 单指标聚合，没有expression，则默认为"$a"
		a.Expression = "$a"
	}
	checker := expChecker{
		state:  stateNull,
		number: "",
		RPNs:   make([]string, 0),
	}
	s := stack.New()

	if len(a.Expression) > 100 {
		return fmt.Errorf("计算表达式不合法：表达式过长")
	}

	for idx, c := range []rune("(" + a.Expression + ")") {
		ePrefix := "计算表达式不合法：第" + strconv.Itoa(idx) + "个字符："
		switch {
		case unicode.IsSpace(c):
			continue // 忽略所有空白字符
		case c == '$':
			if !checker.toNext(stateDollar, c) {
				return fmt.Errorf(ePrefix + "$位置不合法")
			}
		case c == '#':
			if !checker.toNext(stateSharp, c) {
				return fmt.Errorf(ePrefix + "#位置不合法")
			}
		case c >= '0' && c <= '9':
			if !checker.toNext(stateNumber, c) {
				return fmt.Errorf(ePrefix + "数字位置不合法")
			}
			if len(checker.number) > 9 { // 常量不超过9位数
				return fmt.Errorf(ePrefix + "数字过长")
			}
		case c >= 'a' && c <= rune('a'+len(a.RawMetrics)-1):
			if !checker.toNext(stateLetter, c) {
				return fmt.Errorf(ePrefix + "变量位置不合法")
			}
			checker.append("$" + string(c))
		case c == '+' || c == '-' || c == '*' || c == '/':
			if !checker.toNext(stateMark, c) {
				return fmt.Errorf(ePrefix + "运算符号位置不合法")
			}
			for s.Len() > 0 {
				top := s.Peek().(string)
				if isLessPriority(top, string(c)) {
					s.Push(string(c))
					break
				}
				checker.append(top)
				s.Pop()
			}
		case c == '(':
			if !checker.toNext(stateLB, c) {
				return fmt.Errorf(ePrefix + "左括号位置不合法")
			}
			s.Push(string(c))
		case c == ')':
			if !checker.toNext(stateRB, c) {
				return fmt.Errorf(ePrefix + "右括号位置不合法")
			}
			found := false
			for s.Len() > 0 {
				top := s.Pop().(string)
				if top == "(" {
					found = true
					break
				}
				checker.append(top)
			}
			if !found {
				return fmt.Errorf(ePrefix + "括号不匹配，请检查")
			}
		default:
			return fmt.Errorf(ePrefix + "不支持的字符")
		}
	}
	if s.Len() != 0 { // 已处理完，但符号栈没清空，说明左右括号不匹配
		return fmt.Errorf("计算表达式不合法：括号不匹配，请检查")
	}

	a.RPN = checker.print()

	return nil
}

func (a *AggrCalc) Decode() error {
	err := json.Unmarshal([]byte(a.GroupByString), &a.GroupBy)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(a.RawMetricsString), &a.RawMetrics)
	return err
}

/* expChecker 用于校验计算表达式的合法性 */
const (
	stateNull   = iota // 空状态
	stateLB            // (
	stateRB            // )
	stateNumber        // 0-9
	stateDollar        // $
	stateLetter        // a-z
	stateMark          // +-*/
	stateSharp         // #
)

type expChecker struct {
	state   int
	number  string
	isNumID bool
	RPNs    []string
}

func (e *expChecker) append(val string) {
	e.RPNs = append(e.RPNs, val)
}
func (e *expChecker) print() string {
	return strings.Join(e.RPNs, " ")
}
func (e *expChecker) toNext(targetState int, next rune) bool {
	// 检测状态转移是否合法，如果合法则转移至targetState
	valid := false
	switch e.state {
	case stateNull:
		valid = true
	case stateLB:
		switch targetState {
		case stateLB, stateDollar, stateSharp, stateNumber:
			valid = true
		}
	case stateRB:
		switch targetState {
		case stateRB, stateMark:
			valid = true
		}
	case stateLetter:
		switch targetState {
		case stateMark, stateRB:
			valid = true
		}
	case stateMark:
		switch targetState {
		case stateDollar, stateSharp, stateLB, stateNumber:
			valid = true
		}
	case stateDollar:
		switch targetState {
		case stateLetter:
			valid = true
		}
	case stateNumber:
		switch targetState {
		case stateNumber:
			valid = true
		case stateMark, stateRB:
			valid = true
			if e.isNumID {
				e.append("#" + e.number)
			} else {
				e.append(e.number)
			}
			e.isNumID = false
			e.number = ""
		}
	case stateSharp:
		switch targetState {
		case stateNumber:
			e.isNumID = true
			valid = true
		}
	}
	if valid {
		e.state = targetState
		if targetState == stateNumber {
			e.number += string(next)
		}
	}
	return valid
}

/* helper func */
func isInList(source string, validList []string) bool {
	for _, target := range validList {
		if source == target {
			return true
		}
	}
	return false
}

func isLessPriority(a, b string) bool { // check 符号a的优先级 < 符号b (严格小于）
	if a == "(" {
		return true
	}
	if a == "+" || a == "-" {
		if b == "*" || b == "/" {
			return true
		}
	}
	return false
}
