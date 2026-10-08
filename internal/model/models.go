package model

// AllModels 返回需要自动迁移的实体列表。
//
// 新增表后必须在这里登记，否则 database.Migrate 不会为它建表
// （GORM 的 AutoMigrate 只处理显式传入的模型）。
func AllModels() []any {
	return []any{
		&Account{},
		&Org{},
	}
}
