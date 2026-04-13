package migrations

import "embed"

// FS 保存数据库迁移文件，供启动阶段自动执行。
//
//go:embed *.sql
var FS embed.FS
