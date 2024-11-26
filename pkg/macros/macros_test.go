package macros

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReplaceMacros(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		start    time.Time
		end      time.Time
		expected string
		wantErr  bool
	}{
		{
			name:     "Simple __timeFilter macro",
			sql:      "SELECT * FROM table WHERE $__timeFilter(column)",
			start:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			end:      time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
			expected: "SELECT * FROM table WHERE column BETWEEN FROM_UNIXTIME(1672531200) AND FROM_UNIXTIME(1672617600)",
			wantErr:  false,
		},
		{
			name:     "__timeFilter with negative Unix timestamp",
			sql:      "SELECT * FROM table WHERE $__timeFilter(column)",
			start:    time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC),
			end:      time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: "SELECT * FROM table WHERE column BETWEEN DATE_ADD(FROM_UNIXTIME(0), INTERVAL -315619200 SECOND) AND FROM_UNIXTIME(0)",
			wantErr:  false,
		},
		{
			name:     "__timeGroup macro",
			sql:      "SELECT $__timeGroup(time_column, '1h') as time_group FROM table",
			start:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			end:      time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
			expected: "SELECT UNIX_TIMESTAMP(time_column) DIV 3600 * 3600 as time_group FROM table",
			wantErr:  false,
		},
		{
			name:     "Multiple macros",
			sql:      "SELECT $__timeGroup(time_column, '1h') as time_group FROM table WHERE $__timeFilter(time_column)",
			start:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			end:      time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
			expected: "SELECT UNIX_TIMESTAMP(time_column) DIV 3600 * 3600 as time_group FROM table WHERE time_column BETWEEN FROM_UNIXTIME(1672531200) AND FROM_UNIXTIME(1672617600)",
			wantErr:  false,
		},
		{
			name:     "Invalid macro",
			sql:      "SELECT $__invalidMacro(column) FROM table",
			start:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			end:      time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
			expected: "",
			wantErr:  true,
		},
		{
			name:     "Restricted query",
			sql:      "SELECT * FROM table; SHOW GRANTS;",
			start:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			end:      time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ReplaceMacros(tt.sql, tt.start.Unix(), tt.end.Unix())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
