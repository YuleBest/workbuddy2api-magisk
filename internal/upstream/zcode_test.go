package upstream

import (
	"encoding/json"
	"testing"
)

// TestNormalizeZCodeFormat 覆盖 ZCode 的三类畸形：块数组 content、字典形 tools、
// 缺配对 ID 的 tool 消息，外加 maxOutputTokens 字段。
func TestNormalizeZCodeFormat(t *testing.T) {
	src := []byte(`{
		"model": "glm-5.3",
		"messages": [
			{"role": "system", "content": "sys"},
			{"role": "user", "content": [{"type": "text", "text": "hi"}]},
			{"role": "assistant", "content": [
				{"type": "reasoning", "text": "let me think"},
				{"type": "text", "text": "running"},
				{"type": "tool-call", "toolCallId": "call_1", "toolName": "Bash", "input": {"command": "ls"}}
			]},
			{"role": "user", "content": [
				{"type": "tool-result", "toolCallId": "call_1", "toolName": "Bash", "output": {"type": "text", "value": "file1 file2"}}
			]},
			{"role": "user", "content": "go on"}
		],
		"tools": {"Bash": {"description": "run a command", "parameters": {"type": "object", "properties": {}}}},
		"maxOutputTokens": 4096
	}`)

	out := PrepareBodyOpt(src, false)

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("结果不是合法 JSON: %v", err)
	}

	// 1. 输出上限字段归一
	if got["max_tokens"] != float64(4096) {
		t.Errorf("max_tokens 未归一: got %v", got["max_tokens"])
	}

	// 2. tools 字典 → 数组
	tools, ok := got["tools"].([]any)
	if !ok {
		t.Fatalf("tools 未转成数组: %T", got["tools"])
	}
	if len(tools) != 1 {
		t.Fatalf("tools 元素数 = %d, want 1", len(tools))
	}
	tool, _ := tools[0].(map[string]any)
	if tool["type"] != "function" {
		t.Errorf("tool.type = %v, want function", tool["type"])
	}
	fn, _ := tool["function"].(map[string]any)
	if fn["name"] != "Bash" {
		t.Errorf("tool.function.name = %v, want Bash", fn["name"])
	}

	// 3. 块数组展开
	msgs, _ := got["messages"].([]any)
	if len(msgs) != 5 {
		t.Fatalf("messages 数 = %d, want 5", len(msgs))
	}

	// assistant：reasoning + text 合并，tool-call 提升为 tool_calls
	asst, _ := msgs[2].(map[string]any)
	if asst["role"] != "assistant" {
		t.Errorf("msgs[2].role = %v, want assistant", asst["role"])
	}
	if asst["content"] != "let me think\nrunning" {
		t.Errorf("msgs[2].content = %q", asst["content"])
	}
	tcs, ok := asst["tool_calls"].([]any)
	if !ok || len(tcs) != 1 {
		t.Fatalf("msgs[2].tool_calls 缺失或数量不对: %v", asst["tool_calls"])
	}
	tc, _ := tcs[0].(map[string]any)
	if tc["id"] != "call_1" {
		t.Errorf("tool_call.id = %v, want call_1", tc["id"])
	}
	tfn, _ := tc["function"].(map[string]any)
	if tfn["name"] != "Bash" {
		t.Errorf("tool_call.function.name = %v, want Bash", tfn["name"])
	}
	if tfn["arguments"] != `{"command":"ls"}` {
		t.Errorf("tool_call.function.arguments = %v", tfn["arguments"])
	}

	// tool 结果：role=tool + 配对 ID + 输出文本
	tr, _ := msgs[3].(map[string]any)
	if tr["role"] != "tool" {
		t.Errorf("msgs[3].role = %v, want tool", tr["role"])
	}
	if tr["tool_call_id"] != "call_1" {
		t.Errorf("msgs[3].tool_call_id = %v, want call_1", tr["tool_call_id"])
	}
	if tr["content"] != "file1 file2" {
		t.Errorf("msgs[3].content = %v", tr["content"])
	}
}

// TestDemoteOrphanToolMessages 验证无法配对的 tool 消息被降级为 user。
func TestDemoteOrphanToolMessages(t *testing.T) {
	msgs := []any{
		map[string]any{"role": "user", "content": "hi"},
		map[string]any{"role": "tool", "content": "orphan output", "tool_call_id": "missing_id"},
	}
	got := demoteOrphanTools(msgs)
	second, _ := got[1].(map[string]any)
	if second["role"] != "user" {
		t.Errorf("孤立 tool 未降级: role=%v", second["role"])
	}
	if _, exists := second["tool_call_id"]; exists {
		t.Error("降级后仍残留 tool_call_id")
	}
	if second["content"] != "orphan output" {
		t.Errorf("降级丢失内容: %v", second["content"])
	}
}

// TestStandardRequestUntouched 确认标准 OpenAI 请求不被本转换改写。
func TestStandardRequestUntouched(t *testing.T) {
	src := []byte(`{
		"model": "glm-5.3",
		"messages": [
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "hi"},
			{"role": "tool", "content": "out", "tool_call_id": "x"}
		],
		"tools": [{"type": "function", "function": {"name": "F", "parameters": {"type": "object"}}}],
		"max_tokens": 128
	}`)
	out := PrepareBodyOpt(src, false)
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("结果不是合法 JSON: %v", err)
	}
	msgs, _ := got["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages 数 = %d, want 3", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["content"] != "hello" {
		t.Errorf("字符串 content 被改动: %v", first["content"])
	}
	if got["max_tokens"] != float64(128) {
		t.Errorf("max_tokens 被改动: %v", got["max_tokens"])
	}
}
