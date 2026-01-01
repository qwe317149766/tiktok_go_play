package main

import (
	"fmt"
	"strings"
)

func deviceIDFromAny(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	if v, ok := m["device_id"]; ok {
		switch t := v.(type) {
		case string:
			return strings.TrimSpace(t)
		case float64:
			return fmt.Sprintf("%.0f", t)
		case int:
			return fmt.Sprintf("%d", t)
		case int64:
			return fmt.Sprintf("%d", t)
		default:
			return strings.TrimSpace(fmt.Sprintf("%v", t))
		}
	}
	return ""
}


