package wbapicli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var creditRatePattern = regexp.MustCompile(`(?i)x\s*([0-9]+(?:\.[0-9]+)?)`)

var modelCapabilities = []struct {
	Key   string
	Label string
}{
	{"supports_images", "识图"},
	{"supports_tool_call", "工具调用"},
	{"supports_reasoning", "思考"},
	{"only_reasoning", "仅思考"},
}

var shownModelFields = map[string]bool{
	"id": true, "name": true, "context_length": true, "max_output_tokens": true, "credits": true,
	"supports_images": true, "supports_tool_call": true, "supports_reasoning": true, "only_reasoning": true,
	"reasoning_supported_efforts": true, "reasoning_default_effort": true, "reasoning_effort": true,
	"reasoning_summary": true, "object": true, "owned_by": true, "created": true,
}

func creditRate(model map[string]any) string {
	raw := ""
	if !pyFalsy(model["credits"]) {
		raw = strings.TrimSpace(stringValue(model["credits"]))
	}
	if raw == "" {
		return ""
	}
	hit := creditRatePattern.FindStringSubmatch(raw)
	if len(hit) < 2 {
		return raw
	}
	value, _ := strconv.ParseFloat(hit[1], 64)
	if value == 0 {
		return "x0.00（免费）"
	}
	return fmt.Sprintf("x%.2f", value)
}

func modelName(model map[string]any) string {
	name := ""
	if !pyFalsy(model["name"]) {
		name = strings.TrimSpace(stringValue(model["name"]))
	}
	if name != "" {
		return name
	}
	id := stringValue(model["id"])
	if idx := strings.Index(id, ":"); idx >= 0 {
		return id[idx+1:]
	}
	return id
}

func realmText(id string) string {
	if strings.HasPrefix(id, "global:") {
		return "国际"
	}
	return "国内"
}

func (a *App) cmdModels(args []string) int {
	realm := ""
	more := containsArgument(args, "--more")
	for i, arg := range args {
		if arg == "--realm" {
			if i+1 >= len(args) {
				return a.usageError("models", "", "--realm 缺少值")
			}
			realm = args[i+1]
			if realm != "cn" && realm != "global" {
				return a.usageError("models", "", "--realm 只支持 cn 或 global")
			}
			break
		}
	}
	st, data := a.request("/v1/models", a.gatewayToken(), "GET", nil, 30*time.Second)
	if st != 200 {
		return a.fail("拉模型列表失败：" + errMessage(data, "请求失败"))
	}
	models := sliceValue(data["data"])
	if realm != "" {
		filtered := make([]any, 0, len(models))
		for _, item := range models {
			if strings.HasPrefix(stringValue(mapValue(item)["id"]), realm+":") {
				filtered = append(filtered, item)
			}
		}
		models = filtered
	}
	if len(models) == 0 {
		fmt.Fprintln(a.stdout, "模型列表是空的：网关要至少一个可用账号才能拉到上游模型表（看 wbapi status）")
		return 0
	}
	if more {
		a.printModelsMore(models)
		return 0
	}

	const (
		widthName  = 24
		widthID    = 30
		widthRealm = 4
		widthRate  = 7
		widthCap   = 4
		widthCtx   = 11
		widthOut   = 12
	)
	head := fmt.Sprintf("  %s %s %s %s %s %s %s",
		pad("模型", widthName), pad("ID", widthID), pad("域", widthRealm),
		pad("积分倍率", widthRate), pad("能力", widthCap),
		pad("上下文长度", widthCtx, true), pad("单次最大输出", widthOut, true))
	realmSuffix := ""
	if realm != "" {
		realmSuffix = "，域 " + realm
	}
	fmt.Fprintf(a.stdout, "模型（%d 个%s）:\n", len(models), realmSuffix)
	fmt.Fprintln(a.stdout, head)
	fmt.Fprintln(a.stdout, "  "+strings.Repeat("─", displayWidth(head)-2))
	for _, item := range models {
		model := mapValue(item)
		rate := creditRate(model)
		if rate == "" {
			rate = "—"
		}
		vision := "文字"
		if boolValue(model["supports_images"]) {
			vision = "识图"
		}
		contextLength := formatComma(model["context_length"])
		output := "—"
		if boolValue(model["max_output_tokens"]) {
			output = formatComma(model["max_output_tokens"])
		}
		id := stringValue(model["id"])
		fmt.Fprintf(a.stdout, "  %s %s %s %s %s %s %s\n",
			pad(modelName(model), widthName), pad(id, widthID), pad(realmText(id), widthRealm),
			pad(rate, widthRate), pad(vision, widthCap),
			pad(contextLength, widthCtx, true), pad(output, widthOut, true))
	}
	fmt.Fprintln(a.stdout, "\n  --more 看推理档位与全部字段；--realm cn|global 只看某个域。")
	return 0
}

func (a *App) printModelsMore(models []any) {
	fmt.Fprintf(a.stdout, "模型完整信息（%d 个）:\n\n", len(models))
	for _, item := range models {
		model := mapValue(item)
		id := stringValue(model["id"])
		fmt.Fprintf(a.stdout, "%s — %s\n", id, modelName(model))
		rows := [][2]string{
			{"域", realmText(id)},
			{"积分倍率", fallbackString(creditRate(model), "上游未给")},
			{"上下文长度", formatComma(model["context_length"])},
			{"单次最大输出", modelOutput(model)},
			{"可传推理档位", joinAny(sliceValue(model["reasoning_supported_efforts"]), " / ", "无档位")},
			{"默认档位", fallbackString(model["reasoning_default_effort"], "—")},
			{"上游默认 effort", fallbackString(model["reasoning_effort"], "—")},
			{"思考摘要", fallbackString(model["reasoning_summary"], "—")},
		}
		for _, cap := range modelCapabilities {
			value := "否"
			if boolValue(model[cap.Key]) {
				value = "是"
			}
			rows = append(rows, [2]string{cap.Label, value})
		}
		for _, key := range sortedKeys(model) {
			if shownModelFields[key] {
				continue
			}
			value := model[key]
			if pyFalsy(value) {
				continue
			}
			if list, ok := value.([]any); ok {
				parts := make([]string, 0, len(list))
				for _, v := range list {
					parts = append(parts, pyString(v))
				}
				rows = append(rows, [2]string{key, strings.Join(parts, ", ")})
			} else if b, ok := value.(bool); ok {
				if b {
					rows = append(rows, [2]string{key, "是"})
				}
			} else {
				rows = append(rows, [2]string{key, pyString(value)})
			}
		}
		width := 0
		for _, row := range rows {
			if w := displayWidth(row[0]); w > width {
				width = w
			}
		}
		for _, row := range rows {
			fmt.Fprintf(a.stdout, "  %s  %s\n", pad(row[0], width), row[1])
		}
		fmt.Fprintln(a.stdout)
	}
}

func modelOutput(model map[string]any) string {
	if !boolValue(model["max_output_tokens"]) {
		return "—"
	}
	return formatComma(model["max_output_tokens"])
}

func joinAny(values []any, sep, empty string) string {
	if len(values) == 0 {
		return empty
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, stringValue(value))
	}
	return strings.Join(out, sep)
}
