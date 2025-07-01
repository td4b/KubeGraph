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

func Matches(actual, match map[string]interface{}) bool {
	for key, expected := range match {
		actVal, ok := actual[key]
		if !ok {
			return false
		}
		switch exp := expected.(type) {
		case map[string]interface{}:
			actMap, ok := actVal.(map[string]interface{})
			if !ok {
				return false
			}
			if !Matches(actMap, exp) {
				return false
			}
		default:
			if fmt.Sprintf("%v", actVal) != fmt.Sprintf("%v", exp) {
				return false
			}
		}
	}
	return true
}

func MergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		if existing, ok := dst[k]; ok {
			switch srcVal := v.(type) {
			case map[string]interface{}:
				if dstVal, ok := existing.(map[string]interface{}); ok {
					dst[k] = MergeMaps(dstVal, srcVal)
				} else {
					dst[k] = srcVal
				}
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
