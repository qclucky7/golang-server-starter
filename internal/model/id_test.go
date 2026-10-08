package model_test

import (
	"encoding/json"
	"testing"

	"gin-quick-start/internal/model"
)

// TestIDMarshalsAsString 雪花 ID 必须序列化成 JSON 字符串。
//
// 回归点：19 位的雪花 ID 超过 JS 的 Number.MAX_SAFE_INTEGER（16 位），
// 以 number 输出会在前端被静默舍入 —— 不同 ID 可能舍入成同一个值。
func TestIDMarshalsAsString(t *testing.T) {
	type payload struct {
		ID      model.ID `json:"id"`
		OwnerID model.ID `json:"owner_id"`
	}

	const raw int64 = 1859123456789012480

	encoded, err := json.Marshal(payload{ID: model.ID(raw), OwnerID: 42})
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	want := `{"id":"1859123456789012480","owner_id":"42"}`
	if string(encoded) != want {
		t.Fatalf("序列化结果不符\n期望: %s\n实际: %s", want, encoded)
	}
}

// TestIDUnmarshalAcceptsStringAndNumber 反序列化兼容字符串与数字。
func TestIDUnmarshalAcceptsStringAndNumber(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int64
	}{
		{"字符串", `{"id":"1859123456789012480"}`, 1859123456789012480},
		{"数字", `{"id":1859123456789012480}`, 1859123456789012480},
		{"null", `{"id":null}`, 0},
		{"空字符串", `{"id":""}`, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got struct {
				ID model.ID `json:"id"`
			}
			if err := json.Unmarshal([]byte(tc.input), &got); err != nil {
				t.Fatalf("反序列化失败: %v", err)
			}
			if got.ID.Int64() != tc.want {
				t.Fatalf("期望 %d，实际 %d", tc.want, got.ID.Int64())
			}
		})
	}
}

// TestIDUnmarshalRejectsGarbage 非法输入必须报错，而不是静默变成 0。
func TestIDUnmarshalRejectsGarbage(t *testing.T) {
	for _, input := range []string{`{"id":"abc"}`, `{"id":"1.5"}`, `{"id":{}}`} {
		var got struct {
			ID model.ID `json:"id"`
		}
		if err := json.Unmarshal([]byte(input), &got); err == nil {
			t.Fatalf("输入 %s 应报错，实际得到 %d", input, got.ID)
		}
	}
}

// TestIDHelpers 辅助方法行为。
func TestIDHelpers(t *testing.T) {
	var zero model.ID
	if !zero.IsZero() {
		t.Fatal("零值应判定为未分配")
	}
	if model.ID(7).String() != "7" {
		t.Fatalf("String() 不符: %s", model.ID(7).String())
	}
	if model.ID(7).Int64() != 7 {
		t.Fatal("Int64() 不符")
	}
}
