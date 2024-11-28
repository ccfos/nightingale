package macros

var Macro func(sql string, start, end int64) (string, error)

func RegisterMacro(f func(sql string, start, end int64) (string, error)) {
	Macro = f
}

func MacroInVain(sql string, start, end int64) (string, error) {
	return sql, nil
}
