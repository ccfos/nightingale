package macros

// Macro expands SQL macros ($__timeFilter, $__timeGroup, etc) for a given
// datasource. The datasourceType parameter is the same string constant that
// each datasource registers with datasource.RegisterDatasource (for example
// ck.CKType = "ck", es.ESType = "elasticsearch"). Reusing those existing
// constants keeps the "type name" definition single-sourced in each
// datasource package instead of duplicating them here.
var Macro func(sql string, start, end int64, datasourceType string) (string, error)

func RegisterMacro(f func(sql string, start, end int64, datasourceType string) (string, error)) {
	Macro = f
}

func MacroInVain(sql string, start, end int64, _ string) (string, error) {
	return sql, nil
}
