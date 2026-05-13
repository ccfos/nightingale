package sqlbase

import (
	"fmt"
	"strings"
)

// ValidateIdentifier checks that name is safe to embed as a SQL identifier
// (database / schema / table / column name). Most database drivers cannot
// parameterize identifiers, so any code path that has to interpolate one
// into a query MUST validate it first.
func ValidateIdentifier(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("identifier is empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("identifier too long: %d", len(name))
	}
	for _, c := range name {
		switch c {
		case ';', '\'', '"', '`', '\\', 0, ' ', '\t', '\n', '\r':
			return fmt.Errorf("invalid character %q in identifier", c)
		}
	}
	if strings.Contains(name, "/*") || strings.Contains(name, "*/") || strings.Contains(name, "--") {
		return fmt.Errorf("invalid sequence in identifier: %s", name)
	}
	return nil
}

// QuoteBacktick wraps an identifier with backticks (MySQL / Doris / ClickHouse).
// Callers must run ValidateIdentifier first; the backtick escaping here is a
// defense-in-depth fallback.
func QuoteBacktick(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// QuoteDouble wraps an identifier with double quotes (PostgreSQL / standard SQL).
func QuoteDouble(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
