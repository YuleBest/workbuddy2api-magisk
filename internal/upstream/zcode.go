// zcode.go —— ZCode（AI SDK v5 客户端）请求格式适配。
//
// ZCode 的 openai-compatible 模式发送的是 AI SDK v5 内部结构，与上游 WorkBuddy
// 期望的标准 OpenAI 形态差异很大，直接转发会被上游以 400 拒绝：
//
//   - content 为块数组（reasoning / text / tool-call / tool-result）
//     → 11101 "unsupported content type at index 0: reasoning"
//   - tools 为 {工具名: 定义} 字典
//     → 11101 "cannot unmarshal object into Go struct field Request.tools"
//   - tool 消息不带 tool_call_id（配对信息藏在块里）
//     → 11148 "tool calls and tool results do not match"
//   - 输出上限走 maxOutputTokens 字段，上游只认 max_tokens → 输出被截断
//
// 本文件把这些形态就地归一为标准 OpenAI 结构。转换语义无损：块内文本合并进
// content，工具调用与结果保留配对关系；无法配对的 tool 消息降级为 user
// （内容保留），以免触发上游的配对校验。
package upstream

import (
	"encoding/json"
	"strings"
)

// normalizeZCodeFormat 一站式归一 ZCode 请求体（messages / tools / 输出上限）。
func normalizeZCodeFormat(obj map[string]any) {
	normalizeZCodeMessages(obj)
	normalizeZCodeTools(obj)
	normalizeZCodeMaxTokens(obj)
}

// normalizeZCodeMessages 展开 content 块数组为标准 OpenAI 消息。
//
// 单条消息内同时含 tool-call 块时，该消息归一为 assistant + tool_calls；
// 含 tool-result 块时归一为 role=tool + tool_call_id。其余块（text/reasoning）
// 拼接为纯字符串 content。
func normalizeZCodeMessages(obj map[string]any) {
	msgs, ok := obj["messages"].([]any)
	if !ok {
		return
	}

	out := make([]any, 0, len(msgs))
	for _, mv := range msgs {
		msg, ok := mv.(map[string]any)
		if !ok {
			out = append(out, mv)
			continue
		}
		blocks, isBlocks := msg["content"].([]any)
		if !isBlocks {
			out = append(out, msg)
			continue
		}

		var texts []string
		var toolCalls []any
		var toolOutput string
		var toolCallID string
		var parts []any // 图片等非文本 part（image_url 形态）
		hasToolResult := false

		for _, bv := range blocks {
			b, ok := bv.(map[string]any)
			if !ok {
				if s, ok := bv.(string); ok {
					texts = append(texts, s)
				}
				continue
			}
			switch btype, _ := b["type"].(string); btype {
			case "text":
				if s, _ := b["text"].(string); s != "" {
					texts = append(texts, s)
				}
			case "reasoning", "thinking":
				s, _ := b["text"].(string)
				if s == "" {
					s, _ = b["thinking"].(string)
				}
				if s != "" {
					texts = append(texts, s)
				}
			case "tool-call":
				id, _ := b["toolCallId"].(string)
				if id == "" {
					id, _ = b["id"].(string)
				}
				name, _ := b["toolName"].(string)
				if name == "" {
					name, _ = b["name"].(string)
				}
				args := b["input"]
				if args == nil {
					args = b["args"]
				}
				argJSON, err := json.Marshal(args)
				if err != nil || args == nil {
					argJSON = []byte("{}")
				}
				toolCalls = append(toolCalls, map[string]any{
					"id":   id,
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": string(argJSON),
					},
				})
			case "tool-result":
				hasToolResult = true
				if id, ok := b["toolCallId"].(string); ok {
					toolCallID = id
				}
				toolOutput = stringifyToolOutput(b["output"])
			case "image", "file":
				// 图片块 → 上游认的 image_url 形态。ZCode 发
				// {"type":"image","image":"<base64>","mediaType":"image/png"}；
				// 兼容 image 字段为字符串（裸 base64 或 data URI）或对象（{url}）两种。
				if part := toImagePart(b); part != nil {
					parts = append(parts, part)
				}
			case "image_url", "input_image":
				// 已是标准 OpenAI / Anthropic 形态：原样保留（上游直接认）。
				// 缺这一分支会让标准图片块掉进 default 被静默丢弃——模型"看不见图"。
				parts = append(parts, b)
			default:
				// 未知块类型：尽力提取文本，避免整体报错。
				if s, ok := b["text"].(string); ok {
					texts = append(texts, s)
				}
			}
		}

		// 保留原消息上除 content/role 之外的字段（如 name、providerOptions）。
		norm := make(map[string]any, len(msg)+2)
		for k, v := range msg {
			if k == "content" || k == "role" {
				continue
			}
			norm[k] = v
		}
		switch {
		case hasToolResult:
			norm["role"] = "tool"
			norm["content"] = toolOutput
			if toolCallID != "" {
				norm["tool_call_id"] = toolCallID
			}
		case len(parts) > 0:
			// 含图片等非文本 part：content 用多模态数组，文本作为首个 part。
			norm["role"] = msg["role"]
			arr := []any{}
			if t := strings.Join(texts, "\n"); t != "" {
				arr = append(arr, map[string]any{"type": "text", "text": t})
			}
			arr = append(arr, parts...)
			norm["content"] = arr
		case len(toolCalls) > 0:
			norm["role"] = "assistant"
			norm["content"] = strings.Join(texts, "\n")
			norm["tool_calls"] = toolCalls
		default:
			norm["role"] = msg["role"]
			norm["content"] = strings.Join(texts, "\n")
		}
		out = append(out, norm)
	}

	obj["messages"] = demoteOrphanTools(out)
}

// stringifyToolOutput 归一 tool-result 的输出值（字符串 / {type,value} / 其他 JSON）。
func stringifyToolOutput(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case map[string]any:
		if s, ok := t["value"].(string); ok {
			return s
		}
		if s, ok := t["text"].(string); ok {
			return s
		}
		j, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(j)
	default:
		j, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(j)
	}
}

// demoteOrphanTools 把没有对应 tool_call 的 tool 消息降级为 user。
//
// 上游对 tool 结果做严格配对校验，缺失或对不上的 tool_call_id 会报
// 11148 "tool calls and tool results do not match"。降级只改 role、保留内容，
// 信息不丢。
func demoteOrphanTools(msgs []any) []any {
	declared := make(map[string]bool)
	for _, mv := range msgs {
		m, ok := mv.(map[string]any)
		if !ok {
			continue
		}
		tcs, ok := m["tool_calls"].([]any)
		if !ok {
			continue
		}
		for _, tcv := range tcs {
			tc, ok := tcv.(map[string]any)
			if !ok {
				continue
			}
			if id, ok := tc["id"].(string); ok && id != "" {
				declared[id] = true
			}
		}
	}

	for _, mv := range msgs {
		m, ok := mv.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := m["role"].(string); role != "tool" {
			continue
		}
		id, _ := m["tool_call_id"].(string)
		if id == "" || !declared[id] {
			m["role"] = "user"
			delete(m, "tool_call_id")
		}
	}
	return msgs
}

// normalizeZCodeTools 把字典形 tools 转成上游要求的数组。
//
// ZCode 发 {"Bash": {"description":..., "parameters":...}}；上游 Go 端要求
// []*v2.Tools，元素形如 {"type":"function","function":{...}}。数组形态则补齐
// 缺失的 type/function 包装。
func normalizeZCodeTools(obj map[string]any) {
	switch tools := obj["tools"].(type) {
	case map[string]any:
		arr := make([]any, 0, len(tools))
		for name, defv := range tools {
			def, _ := defv.(map[string]any)
			if def != nil {
				if _, hasFn := def["function"]; hasFn {
					item := make(map[string]any, len(def)+1)
					for k, v := range def {
						item[k] = v
					}
					if _, ok := item["type"]; !ok {
						item["type"] = "function"
					}
					arr = append(arr, item)
					continue
				}
			}
			fn := map[string]any{"name": name}
			if def != nil {
				for _, k := range []string{"description", "parameters", "strict"} {
					if v, ok := def[k]; ok {
						fn[k] = v
					}
				}
			}
			if _, ok := fn["parameters"]; !ok {
				fn["parameters"] = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			arr = append(arr, map[string]any{"type": "function", "function": fn})
		}
		obj["tools"] = arr

	case []any:
		for i, tv := range tools {
			item, ok := tv.(map[string]any)
			if !ok {
				continue
			}
			if _, hasFn := item["function"]; hasFn {
				if _, ok := item["type"]; !ok {
					item["type"] = "function"
				}
				continue
			}
			name, ok := item["name"].(string)
			if !ok {
				continue
			}
			fn := map[string]any{"name": name}
			for _, k := range []string{"description", "parameters", "strict"} {
				if v, ok := item[k]; ok {
					fn[k] = v
				}
			}
			if _, ok := fn["parameters"]; !ok {
				fn["parameters"] = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools[i] = map[string]any{"type": "function", "function": fn}
		}
	}
}

// normalizeZCodeMaxTokens 兼容各客户端的输出上限字段名。
//
// OpenAI 标准是 max_tokens；ZCode 用 maxOutputTokens，另有 max_output_tokens 变体。
// 取不到时保持原样（交由上游按默认处理）。
//
// 不处理 max_completion_tokens：那是 OpenAI 官方别名，由 payload.go 的
// translateMaxCompletionTokens 统一翻译（带"非正值/非整数不译、显式 max_tokens
// 优先、别名一律删"的判据）。这里再抄一份会绕过那些判据，把 0/null/负数/小数
// 直接搬进 max_tokens。
func normalizeZCodeMaxTokens(obj map[string]any) {
	if _, ok := obj["max_tokens"]; ok {
		return
	}
	for _, k := range []string{"maxOutputTokens", "max_output_tokens"} {
		if v, ok := obj[k]; ok {
			obj["max_tokens"] = v
			return
		}
	}
}

// toImagePart 把 ZCode 的图片/文件块转成上游认的 image_url part。
//
// 已知形态：{"type":"image","image":"<裸 base64 或 data URI>","mediaType":"image/png"}，
// 以及 image 为对象（{url:...}）的变体。裸 base64 需补 data URI 前缀。
// 提不出图片数据时返回 nil（调用方跳过）。
func toImagePart(b map[string]any) any {
	mediaType, _ := b["mediaType"].(string)
	if mediaType == "" {
		mediaType, _ = b["mimeType"].(string)
	}
	img := b["image"]
	if img == nil {
		img = b["data"]
	}
	var url string
	switch v := img.(type) {
	case string:
		url = v
	case map[string]any:
		if u, ok := v["url"].(string); ok {
			url = u
		}
	}
	if url == "" {
		return nil
	}
	if !strings.HasPrefix(url, "data:") {
		if mediaType == "" {
			mediaType = "image/png"
		}
		url = "data:" + mediaType + ";base64," + url
	}
	return map[string]any{
		"type":      "image_url",
		"image_url": map[string]any{"url": url},
	}
}
