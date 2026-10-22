package wbapicli

import (
	"encoding/hex"
	"fmt"
	"io"
)

func (a *App) cmdKeyList(show bool) int {
	doc := a.loadKeys()
	keys := sliceValue(doc["keys"])
	if len(keys) == 0 {
		fmt.Fprintf(a.stdout, "keys.json 里没有 key（%s）——网关当前不鉴权，建议建一个：wbapi key new\n", a.keysPath())
		return 0
	}
	fmt.Fprintf(a.stdout, "共 %d 个 key:\n", len(keys))
	for _, item := range keys {
		entry := mapValue(item)
		key := stringValue(entry["key"])
		display := key
		if !show {
			display = keyPrefix(key, 8) + "…" + keySuffix(key, 4)
		}
		fmt.Fprintf(a.stdout, "  %s  %s  %s\n",
			display, stringValue(entry["note"]), stringValue(entry["added"]))
	}
	if !show {
		fmt.Fprintln(a.stdout, "  （加 --show 显示完整值）")
	}
	return 0
}

func keyPrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func keySuffix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func (a *App) cmdKeyNew(note string) int {
	raw := make([]byte, 16)
	if _, err := io.ReadFull(a.deps.Random, raw); err != nil {
		return a.fail("生成 key 失败：" + err.Error())
	}
	key := "sk-" + hex.EncodeToString(raw)
	doc := a.loadKeys()
	keys := sliceValue(doc["keys"])
	displayNote := note
	if displayNote == "" {
		displayNote = "未命名"
	}
	keys = append(keys, map[string]any{
		"key":   key,
		"note":  displayNote,
		"added": a.deps.Now().Format("2006-01-02"),
	})
	doc["keys"] = keys
	if err := a.saveKeys(doc); err != nil {
		return a.fail("保存 keys.json 失败：" + err.Error())
	}
	fmt.Fprintf(a.stdout, "[✓] 已添加（%s）\n\n%s\n\n[*] 立即生效（网关热重载），无需重启\n", displayNote, key)
	return 0
}

func (a *App) cmdKeyRevoke(prefix string) int {
	doc := a.loadKeys()
	keys := sliceValue(doc["keys"])
	hitIndex := -1
	for i, item := range keys {
		if hasStringPrefix(stringValue(mapValue(item)["key"]), prefix) {
			if hitIndex >= 0 {
				hitIndex = -2
				break
			}
			hitIndex = i
		}
	}
	if hitIndex < 0 {
		count := 0
		if hitIndex == -2 {
			for _, item := range keys {
				if hasStringPrefix(stringValue(mapValue(item)["key"]), prefix) {
					count++
				}
			}
		}
		return a.fail(fmt.Sprintf("匹配到 %d 个 key，请用更长的前缀", count))
	}
	hit := mapValue(keys[hitIndex])
	remaining := append([]any(nil), keys[:hitIndex]...)
	remaining = append(remaining, keys[hitIndex+1:]...)
	doc["keys"] = remaining
	if err := a.saveKeys(doc); err != nil {
		return a.fail("保存 keys.json 失败：" + err.Error())
	}
	key := stringValue(hit["key"])
	fmt.Fprintf(a.stdout, "[✓] 已吊销 %s…（%s），剩余 %d 个，立即生效\n",
		keyPrefix(key, 8), stringValue(hit["note"]), len(remaining))
	return 0
}

func hasStringPrefix(s, prefix string) bool {
	return len(prefix) <= len(s) && s[:len(prefix)] == prefix
}
