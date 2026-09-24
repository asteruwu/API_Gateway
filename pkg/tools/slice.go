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
