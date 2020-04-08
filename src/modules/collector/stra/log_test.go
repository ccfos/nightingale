package stra

import (
	"fmt"
	"testing"
)

func TestPatternParse(t *testing.T) {
	fmt.Println("Now Test PatternParse:")
	var a Strategy
	a.Pattern = "test"
	parsePattern([]*Strategy{&a})
	fmt.Printf("a.pat:[%s], a.ex[%s]\n", a.Pattern, a.Exclude)
	a.Pattern = "```EXCLUDE```test"
	parsePattern([]*Strategy{&a})
	fmt.Printf("a.pat:[%s], a.ex[%s]\n", a.Pattern, a.Exclude)
	a.Pattern = "test```EXCLUDE```"
	parsePattern([]*Strategy{&a})
	fmt.Printf("a.pat:[%s], a.ex[%s]\n", a.Pattern, a.Exclude)
}
