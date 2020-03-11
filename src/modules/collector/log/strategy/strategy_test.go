package strategy

import (
	"common/scheme"
	"fmt"
	"testing"
)

func TestGetMyStrategy(t *testing.T) {
	fmt.Println("Now Test GetLocalStrategy:")
	data, err := getMyStrategy()
	if err == nil {
		fmt.Println("Result:")
		for _, x := range data {
			fmt.Printf("    %v\n", x)
		}
	} else {
		fmt.Println("Something Error:")
		fmt.Println(err)
	}
}

func TestPatternParse(t *testing.T) {
	fmt.Println("Now Test PatternParse:")
	var a scheme.Strategy
	a.Pattern = "test"
	parsePattern([]*scheme.Strategy{&a})
	fmt.Printf("a.pat:[%s], a.ex[%s]\n", a.Pattern, a.Exclude)
	a.Pattern = "```EXCLUDE```test"
	parsePattern([]*scheme.Strategy{&a})
	fmt.Printf("a.pat:[%s], a.ex[%s]\n", a.Pattern, a.Exclude)
	a.Pattern = "test```EXCLUDE```"
	parsePattern([]*scheme.Strategy{&a})
	fmt.Printf("a.pat:[%s], a.ex[%s]\n", a.Pattern, a.Exclude)
}
