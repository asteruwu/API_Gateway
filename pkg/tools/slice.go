package tools

import "slices"

// StringsOverlap 判断两个字符串切片是否有交集；空切片视为通配，必然重叠
func StringsOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	set := make(map[string]bool, len(a))
	for _, m := range a {
		set[m] = true
	}
	for _, m := range b {
		if set[m] {
			return true
		}
	}
	return false
}

// StringsContain 判断 v 是否在 list 中；空 list 视为通配，匹配任意值
func StringsContain(list []string, v string) bool {
	if len(list) == 0 {
		return true
	}
	return slices.Contains(list, v)
}

// DuplicateStrings 返回 names 中重复出现的值，按首次重复出现的顺序去重
func DuplicateStrings[T ~string](names []T) []T {
	seen := make(map[T]bool, len(names))
	duplicated := make(map[T]bool)
	out := make([]T, 0)
	for _, name := range names {
		if !seen[name] {
			seen[name] = true
			continue
		}
		if duplicated[name] {
			continue
		}
		duplicated[name] = true
		out = append(out, name)
	}
	return out
}

// UniqueStrings 按首次出现顺序对字符串切片去重
func UniqueStrings(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// CloneOrDefault 返回 src 的副本；src 为 nil 时改为返回 fallback 的副本
func CloneOrDefault[T any](src, fallback []T) []T {
	if src == nil {
		src = fallback
	}
	return append(make([]T, 0, len(src)), src...)
}
