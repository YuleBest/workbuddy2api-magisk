package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeKeys(t *testing.T, path string, keys []string) {
	t.Helper()
	var f keysFile
	for _, k := range keys {
		f.Keys = append(f.Keys, keyEntry{Key: k})
	}
	raw, _ := json.Marshal(f)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	// 确保 mtime 变化（文件系统时间精度可能秒级）
	time.Sleep(10 * time.Millisecond)
}

func TestKeyStoreFallbackToConfig(t *testing.T) {
	dir := t.TempDir()
	ks := NewKeyStore(filepath.Join(dir, "keys.json"), "cfg-key")

	if !ks.Valid("cfg-key") {
		t.Error("keys.json 不存在时应回退到 config 的 key")
	}
	if ks.Valid("other") {
		t.Error("非配置 key 应被拒绝")
	}
	if !ks.AuthRequired() {
		t.Error("有 fallback key 时应要求鉴权")
	}
	if ks.Count() != 1 {
		t.Errorf("Count = %d, want 1", ks.Count())
	}
}

func TestKeyStoreMultiAndHotReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	writeKeys(t, path, []string{"k1", "k2"})

	ks := NewKeyStore(path, "cfg-key")
	if !ks.Valid("k1") || !ks.Valid("k2") {
		t.Error("keys.json 中的 key 应通过")
	}
	if ks.Valid("cfg-key") {
		t.Error("keys.json 有内容时应以它为准，不再接受 config 的 key")
	}

	// 热重载：追加 k3、移除 k1
	writeKeys(t, path, []string{"k2", "k3"})
	if !ks.Valid("k3") {
		t.Error("新增 key 应无需重启即可生效")
	}
	if ks.Valid("k1") {
		t.Error("已移除的 key 应立即失效")
	}
	if !ks.Valid("k2") {
		t.Error("保留的 key 应仍然有效")
	}
	if ks.Count() != 2 {
		t.Errorf("Count = %d, want 2", ks.Count())
	}
}

func TestKeyStoreEmptyMeansNoAuth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	writeKeys(t, path, nil)

	ks := NewKeyStore(path, "")
	if ks.AuthRequired() {
		t.Error("keys.json 为空且无 fallback 时不应要求鉴权")
	}
}

func TestKeyStoreBadJSONKeepsOldKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	writeKeys(t, path, []string{"good"})

	ks := NewKeyStore(path, "")
	if !ks.Valid("good") {
		t.Fatal("初始 key 应有效")
	}

	// 写入坏 JSON：应保留旧集合，避免写坏文件把所有人挡在门外
	os.WriteFile(path, []byte("{not json"), 0o600)
	time.Sleep(10 * time.Millisecond)
	if !ks.Valid("good") {
		t.Error("keys.json 解析失败时应保留原有 key 集合")
	}
}

// TestKeyStoreAddListRemove 管理后台路径：新增/列出/前缀吊销，且都无需重启即生效。
func TestKeyStoreAddListRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	writeKeys(t, path, []string{"sk-existing0000000000000000000000000000"})

	ks := NewKeyStore(path, "")
	created, err := ks.Add("后台新建")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !strings.HasPrefix(created, "sk-") || len(created) != 35 {
		t.Errorf("新 key 形态异常：%q", created)
	}
	if !ks.Valid(created) {
		t.Error("新增的 key 应立即可用（热重载）")
	}

	list := ks.List()
	if len(list) != 2 {
		t.Fatalf("List 应返回 2 条，得到 %d", len(list))
	}
	var created_ *KeyInfo
	for i := range list {
		if !strings.Contains(list[i].Masked, "…") {
			t.Errorf("key 必须掩码展示：%q", list[i].Masked)
		}
		if list[i].Note == "后台新建" {
			created_ = &list[i]
		}
	}
	if created_ == nil {
		t.Fatal("未在列表里找到新建的 key")
	}
	if created_.Note != "后台新建" {
		t.Errorf("备注 = %q, want 后台新建", created_.Note)
	}

	// 前缀必须唯一：太短的前缀匹配多条时应拒绝。
	if _, err := ks.Remove("sk-"); err == nil {
		t.Error("歧义前缀应被拒绝")
	}
	remaining, err := ks.Remove(created_.Prefix)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if remaining != 1 {
		t.Errorf("剩余 = %d, want 1", remaining)
	}
	if ks.Valid(created) {
		t.Error("吊销的 key 应立即失效")
	}
}

// TestKeyStoreAddRejectsBadFile 文件被写坏时不得覆盖（宁可写不进，也不能清空所有人的 key）。
func TestKeyStoreAddRejectsBadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	os.WriteFile(path, []byte("{broken"), 0o600)

	ks := NewKeyStore(path, "")
	if _, err := ks.Add("x"); err == nil {
		t.Fatal("坏文件应导致 Add 报错")
	}
	if got, _ := os.ReadFile(path); string(got) != "{broken" {
		t.Errorf("坏文件不应被覆盖，得到 %q", got)
	}
}

// TestKeyStoreListFallsBackToConfig 未启用 keys.json 文件时，列表仍应体现 config 里的 key。
func TestKeyStoreListFallsBackToConfig(t *testing.T) {
	ks := NewKeyStore("", "cfg-key-1234567890")
	list := ks.List()
	if len(list) != 1 {
		t.Fatalf("应回退列出 config key，得到 %d 条", len(list))
	}
	if !ks.AuthRequired() {
		t.Error("有 fallback key 时应要求鉴权")
	}
	if _, err := ks.Add("x"); err == nil {
		t.Error("未配置 keys_file 时应拒绝写入")
	}
}
