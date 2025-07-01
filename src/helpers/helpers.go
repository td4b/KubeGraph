package helpers

import (
	"fmt"
	"strconv"
)

func Walk(data interface{}, path []string) interface{} {
	target := data
	for _, p := range path {
		switch node := target.(type) {
		case map[string]interface{}:
			v, ok := node[p]
			if !ok {
				return fmt.Sprintf("<missing: %s>", p)
			}
			target = v
		case []interface{}:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(node) {
				return fmt.Sprintf("<invalid index: %s>", p)
			}
			target = node[idx]
		default:
			return fmt.Sprintf("<unexpected node at: %s>", p)
		}
	}
	return target
}

func Matches(actual, match interface{}) bool {
	actMap := toMap(actual)
	matchMap := toMap(match)

	for key, expected := range matchMap {
		actVal, ok := actMap[key]
		if !ok {
			return false
		}
		switch expected.(type) {
		case map[string]interface{}, map[interface{}]interface{}:
			if !Matches(actVal, expected) {
				return false
			}
		default:
			if fmt.Sprintf("%v", actVal) != fmt.Sprintf("%v", expected) {
				return false
			}
		}
	}
	return true
}

func toMap(i interface{}) map[string]interface{} {
	switch m := i.(type) {
	case map[string]interface{}:
		return m
	case map[interface{}]interface{}:
		out := make(map[string]interface{})
		for k, v := range m {
			out[fmt.Sprintf("%v", k)] = v
		}
		return out
	default:
		return map[string]interface{}{}
	}
}

func MergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		if existing, ok := dst[k]; ok {
			srcMap := toMap(v)
			dstMap := toMap(existing)

			if len(srcMap) > 0 && len(dstMap) > 0 {
				dst[k] = MergeMaps(dstMap, srcMap)
				continue
			}

			switch srcVal := v.(type) {
			case []interface{}:
				if dstVal, ok := existing.([]interface{}); ok {
					dst[k] = append(dstVal, srcVal...)
				} else {
					dst[k] = srcVal
				}
			default:
				dst[k] = srcVal
			}
		} else {
			dst[k] = v
		}
	}
	return dst
}
