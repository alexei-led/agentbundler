package model

import "sort"

// SortHookDescriptors sorts hooks by declared order, identity, then source location.
func SortHookDescriptors(hooks []HookDescriptor) {
	sort.SliceStable(hooks, func(left, right int) bool {
		if hooks[left].Order != hooks[right].Order {
			return hooks[left].Order < hooks[right].Order
		}
		if hooks[left].Identity != hooks[right].Identity {
			return hooks[left].Identity < hooks[right].Identity
		}
		if hooks[left].Location.Path != hooks[right].Location.Path {
			return hooks[left].Location.Path < hooks[right].Location.Path
		}
		if compareOptionalInt(hooks[left].Location.Line, hooks[right].Location.Line) != 0 {
			return compareOptionalInt(hooks[left].Location.Line, hooks[right].Location.Line) < 0
		}
		return compareOptionalInt(hooks[left].Location.Column, hooks[right].Location.Column) < 0
	})
}

func compareOptionalInt(left, right *int) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if *left < *right {
		return -1
	}
	if *left > *right {
		return 1
	}
	return 0
}
