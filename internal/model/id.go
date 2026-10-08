package model

import (
	"errors"
	"strconv"
)

// ID 雪花算法主键。
//
// 为什么单独定义类型而不是直接用 int64：
//
//	雪花 ID 是 19 位十进制数，超过 JavaScript 的 Number.MAX_SAFE_INTEGER
//	（2^53-1，16 位）。直接以 JSON number 返回，前端解析时末尾几位会被静默
//	四舍五入成 0 —— 更糟的是不同 ID 可能被舍入成同一个值，前端拿着错的 ID
//	去请求详情接口，表现为「查到了别人的数据」或莫名其妙的 404。
//
//	这里统一序列化成 JSON **字符串**（"1859...  "），前端当字符串透传即可。
//	用独立类型而不是在每个字段上写 `json:",string"`，是为了不可能忘记。
//
// 数据库层面仍然是 BIGINT，由 Base 的 gorm tag 指定。
type ID int64

// Int64 转回原生类型，用于与不感知 model 包的组件（如 JWT claims）交互
func (i ID) Int64() int64 { return int64(i) }

// String 十进制字符串
func (i ID) String() string { return strconv.FormatInt(int64(i), 10) }

// IsZero 是否尚未分配
func (i ID) IsZero() bool { return i == 0 }

// MarshalJSON 输出为 JSON 字符串，避免 JS 精度丢失
func (i ID) MarshalJSON() ([]byte, error) {
	// 用引号包裹的十进制字面量，无需转义
	return []byte(`"` + strconv.FormatInt(int64(i), 10) + `"`), nil
}

// UnmarshalJSON 同时接受字符串与数字。
//
// 接受数字是为了兼容外部系统直接传 number 的场景；字符串是推荐形式。
func (i *ID) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*i = 0
		return nil
	}

	text := string(data)
	if text[0] == '"' {
		unquoted, err := strconv.Unquote(text)
		if err != nil {
			return errors.New("id 不是合法的 JSON 字符串: " + text)
		}
		text = unquoted
	}
	if text == "" {
		*i = 0
		return nil
	}

	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return errors.New("id 需为十进制整数或数字字符串，实际: " + string(data))
	}
	*i = ID(value)
	return nil
}
