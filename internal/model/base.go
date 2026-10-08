// Package model 定义数据库实体与项目公共数据结构。
package model

import (
	"time"

	"gin-quick-start/internal/pkg/snowflake"

	"gorm.io/gorm"
	"gorm.io/plugin/soft_delete"
)

// Entity 所有持久化实体的最小契约，供泛型仓储约束使用。
type Entity interface {
	GetID() ID
}

// Base 通用实体基础字段。
//
// 主键用雪花算法在应用层生成，**不是数据库自增**：
//
//   - tag 必须显式写 autoIncrement:false。GORM 对 int64 类型的主键默认
//     按自增处理（省略该列并用 LastInsertId 回读），不加这个 tag 我们
//     在 BeforeCreate 里赋的值会被丢掉。
//   - BeforeCreate 在 ID 为 0 时分配雪花 ID；调用方显式指定了主键则尊重调用方
//     （数据导入、测试固定 ID 会用到）。
//   - 生成器需要先 snowflake.Setup，由 bootstrap 完成；未初始化时插入会直接报错，
//     不会静默写入 0 号主键。
//
// 时间字段命名与 GORM 默认约定的差异（改动前务必先读）：
//
//   - 本项目统一用 CreatedTime / UpdatedTime / DeletedTime，
//     这是命名约定，**不是 GORM 的默认名**。
//   - GORM 的自动时间戳按「字段名」识别，只认 CreatedAt / UpdatedAt
//     （见 gorm/schema/field.go）。所以这里必须显式写 autoCreateTime /
//     autoUpdateTime 两个 tag，否则两个字段会**静默保持零值且不报错**。
//   - 软删除按「字段类型实现的接口」识别，与字段名无关，
//     所以 DeletedTime 这个命名不影响软删除行为。
//
// 软删除实现：gorm.io/plugin/soft_delete（GORM 官方插件，纯类型实现，无需注册）。
// DeletedTime 存 unix 秒，0 表示未删除：
//
//	查询  SELECT ... WHERE deleted_time = 0
//	删除  UPDATE ... SET deleted_time = <unix 秒> WHERE id = ? AND deleted_time = 0
//
// 需要读取或真删已软删记录时用 db.Unscoped()。
//
// 为什么不用可空时间戳（gorm.DeletedAt）：NULL 在唯一索引中互不冲突，
// 会让 (唯一列, deleted_time) 这类复合唯一索引形同虚设。
// 用 unix 秒（0 而不是 NULL）才能让复合唯一索引真正生效，
// 也便于「软删后可复用唯一键」这类需求。
type Base struct {
	ID          ID                    `gorm:"primaryKey;autoIncrement:false;comment:主键（雪花算法）" json:"id"`
	CreatedTime time.Time             `gorm:"autoCreateTime;comment:创建时间" json:"created_time"`
	UpdatedTime time.Time             `gorm:"autoUpdateTime;comment:更新时间" json:"updated_time"`
	DeletedTime soft_delete.DeletedAt `gorm:"index;comment:软删除时间戳（unix 秒，0 表示未删除）" json:"-"`
}

// GetID 实现 Entity 接口。使用值接收者，保证 T 与 *T 都满足约束。
func (b Base) GetID() ID { return b.ID }

// BeforeCreate GORM 创建钩子：分配雪花主键。
func (b *Base) BeforeCreate(*gorm.DB) error {
	if b.ID != 0 {
		return nil
	}
	id, err := snowflake.Next()
	if err != nil {
		return err
	}
	b.ID = ID(id)
	return nil
}
