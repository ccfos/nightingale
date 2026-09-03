// Package embedded 以 embed.FS 形式把内置 skill 资源打进二进制。
// 运行时由 skill.ExtractBuiltin 在启动阶段解压到磁盘，供下游工具按路径访问。
package embedded

import "embed"

//go:embed all:builtin
var FS embed.FS
