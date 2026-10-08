// Package convert 提供泛型转换工具。
package convert

// AnySlice 将 []T 转为 []any，便于构造通用响应体。
func AnySlice[T any](in []T) []any {
	out := make([]any, len(in))
	for i := range in {
		out[i] = in[i]
	}
	return out
}

// Map 将 []T 映射为 []R
func Map[T any, R any](in []T, fn func(T) R) []R {
	out := make([]R, 0, len(in))
	for _, item := range in {
		out = append(out, fn(item))
	}
	return out
}
