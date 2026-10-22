package wbapicli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeDisplayWidth(r)
	}
	return n
}

func pad(s string, width int, right ...bool) string {
	space := width - displayWidth(s)
	if space < 0 {
		space = 0
	}
	padding := strings.Repeat(" ", space)
	if len(right) > 0 && right[0] {
		return padding + s
	}
	return s + padding
}

// runeDisplayWidth approximates Python's unicodedata.east_asian_width(c) in "WF".
// Go's standard library intentionally has no East Asian Width property, so the
// ranges below cover the wide/fullwidth blocks used by the CLI and common emoji.
func runeDisplayWidth(r rune) int {
	if r == 0 {
		return 1
	}
	ranges := [...]struct{ lo, hi rune }{
		{0x1100, 0x115F}, {0x231A, 0x231B}, {0x2329, 0x232A},
		{0x23E9, 0x23EC}, {0x23F0, 0x23F0}, {0x23F3, 0x23F3},
		{0x25FD, 0x25FE}, {0x2614, 0x2615}, {0x2648, 0x2653},
		{0x267F, 0x267F}, {0x2693, 0x2693}, {0x26A1, 0x26A1},
		{0x26AA, 0x26AB}, {0x26BD, 0x26BE}, {0x26C4, 0x26C5},
		{0x26CE, 0x26CE}, {0x26D4, 0x26D4}, {0x26EA, 0x26EA},
		{0x26F2, 0x26F3}, {0x26F5, 0x26F5}, {0x26FA, 0x26FA},
		{0x26FD, 0x26FD}, {0x2705, 0x2705}, {0x270A, 0x270B},
		{0x2728, 0x2728}, {0x274C, 0x274C}, {0x274E, 0x274E},
		{0x2753, 0x2755}, {0x2757, 0x2757}, {0x2795, 0x2797},
		{0x27B0, 0x27B0}, {0x27BF, 0x27BF}, {0x2B1B, 0x2B1C},
		{0x2B50, 0x2B50}, {0x2B55, 0x2B55}, {0x2E80, 0x303E},
		{0x3041, 0x33FF}, {0x3400, 0x4DBF}, {0x4E00, 0x9FFF},
		{0xA000, 0xA4CF}, {0xA960, 0xA97F}, {0xAC00, 0xD7A3},
		{0xF900, 0xFAFF}, {0xFE10, 0xFE19}, {0xFE30, 0xFE6F},
		{0xFF01, 0xFF60}, {0xFFE0, 0xFFE6}, {0x1F004, 0x1F004},
		{0x1F0CF, 0x1F0CF}, {0x1F18E, 0x1F18E}, {0x1F191, 0x1F19A},
		{0x1F200, 0x1F320}, {0x1F32D, 0x1F335}, {0x1F337, 0x1F37C},
		{0x1F37E, 0x1F393}, {0x1F3A0, 0x1F3CA}, {0x1F3CF, 0x1F3D3},
		{0x1F3E0, 0x1F3F0}, {0x1F3F4, 0x1F3F4}, {0x1F3F8, 0x1F43E},
		{0x1F440, 0x1F440}, {0x1F442, 0x1F4FC}, {0x1F4FF, 0x1F53D},
		{0x1F54B, 0x1F54E}, {0x1F550, 0x1F567}, {0x1F57A, 0x1F57A},
		{0x1F595, 0x1F596}, {0x1F5A4, 0x1F5A4}, {0x1F5FB, 0x1F64F},
		{0x1F680, 0x1F6C5}, {0x1F6CC, 0x1F6CC}, {0x1F6D0, 0x1F6D2},
		{0x1F6D5, 0x1F6D7}, {0x1F6DC, 0x1F6DF}, {0x1F6EB, 0x1F6EC},
		{0x1F6F4, 0x1F6FC}, {0x1F7E0, 0x1F7EB}, {0x1F7F0, 0x1F7F0},
		{0x1F90C, 0x1F93A}, {0x1F93C, 0x1F945}, {0x1F947, 0x1F9FF},
		{0x1FA70, 0x1FAFF}, {0x20000, 0x2FFFD}, {0x30000, 0x3FFFD},
	}
	for _, rg := range ranges {
		if r < rg.lo {
			return 1
		}
		if r <= rg.hi {
			return 2
		}
	}
	return 1
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func fmtSecs(v any) string {
	sec := intValue(v)
	if sec <= 0 {
		return "0s"
	}
	h, m, s := sec/3600, (sec%3600)/60, sec%60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func timestamp(v any) string {
	s := ""
	if !pyFalsy(v) {
		s = stringValue(v)
	}
	if len(s) > 19 {
		s = s[:19]
	}
	s = strings.Replace(s, "T", " ", 1)
	if s == "" {
		return "—"
	}
	return s
}

func boolValue(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case nil:
		return false
	case string:
		return x != ""
	case json.Number:
		n, _ := x.Float64()
		return n != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}

func intValue(v any) int {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return int(n)
		}
		n, _ := x.Float64()
		return int(n)
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	default:
		return 0
	}
}

func pyFalsy(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case bool:
		return !x
	case string:
		return x == ""
	case json.Number:
		n, _ := x.Float64()
		return n == 0
	case float64:
		return x == 0
	case int:
		return x == 0
	case int64:
		return x == 0
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	default:
		return false
	}
}

func pyValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case string:
		return x
	case json.Number:
		return x.String()
	default:
		return fmt.Sprint(v)
	}
}

func floatValue(v any) float64 {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Float64()
		return n
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		n, _ := strconv.ParseFloat(x, 64)
		return n
	default:
		return 0
	}
}

func pyFloat(v any) float64 {
	return floatValue(v)
}

func formatFloat(v float64, digits int) string {
	return strconv.FormatFloat(v, 'f', digits, 64)
}

func formatComma(v any) string {
	n := int64(intValue(v))
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}

func mapValue(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func sliceValue(v any) []any {
	s, _ := v.([]any)
	return s
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func pyString(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			parts = append(parts, pyString(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		parts := make([]string, 0, len(x))
		for _, k := range sortedKeys(x) {
			parts = append(parts, fmt.Sprintf("'%s': %s", k, pyString(x[k])))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return fmt.Sprint(v)
	}
}
