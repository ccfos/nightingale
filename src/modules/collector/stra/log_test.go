package stra

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPatternParse(t *testing.T) {
	s := Strategy{
		Pattern: "test-stra",
	}

	parsePattern([]*Strategy{&s})
	assert.Equal(t, s.Pattern, "test-stra")
	assert.Equal(t, s.Exclude, "")

	s.Pattern = "```EXCLUDE```test"
	parsePattern([]*Strategy{&s})
	assert.Equal(t, s.Pattern, "")
	assert.Equal(t, s.Exclude, "test")

	s.Pattern = "test```EXCLUDE```"
	parsePattern([]*Strategy{&s})
	assert.Equal(t, s.Pattern, "test")
	assert.Equal(t, s.Exclude, "")
}
