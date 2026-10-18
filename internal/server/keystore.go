// API key 管理：多 key 校验 + 文件热加载。
//
// 设计要点：
//   - key 存独立文件 keys.json（与 config.json 解耦），可由 wbapi key 系列命令增删
//   - 每次鉴权检查文件 mtime，变了就重载 —— 吊销/新增无需重启服务
//   - 向后兼容：keys.json 不存在时回退到 config.json 的 api_key（老配置照常работа）
//   - 空文件/空列表 = 不鉴权（与 config.json 里 api_key 为空的语义一致）
//
// 管理后台（internal/admin）经 List/Add/Remove 读写同一文件，写入后立即 reload，
// 与 wbapi CLI 双写同一份 keys.json（note/added 字段格式与 CLI 保持一致）。
package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// KeyStore 管理可用的调用 key 集合，支持热重载。
type KeyStore struct {
	mu       sync.RWMutex
	path     string
	fallback string // keys.json 缺失时用的 key（来自 config.json）
	keys     map[string]bool
	order    []string // key 原文件顺序（供 First/List 稳定排序）
	entries  []keyEntry
	mtime    time.Time
	lastErr  string
}

type keysFile struct {
	Keys []keyEntry `json:"keys"`
}

// keyEntry keys.json 单条记录。note 是 wbapi CLI/手写用字段，comment 为历史字段，
// 两者都读；写回统一用 note。
type keyEntry struct {
	Key     string `json:"key"`
	Note    string `json:"note,omitempty"`
	Comment string `json:"comment,omitempty"`
	Added   string `json:"added,omitempty"`
}

// label 返回该条目的备注（note 优先，回落 comment）。
func (e keyEntry) label() string {
	if e.Note != "" {
		return e.Note
	}
	return e.Comment
}

// KeyInfo 管理后台展示用的 key 条目（key 值掩码，不暴露完整值）。
type KeyInfo struct {
	Prefix string `json:"prefix"`
	Masked string `json:"masked"`
	Note   string `json:"note,omitempty"`
	Added  string `json:"added,omitempty"`
}

// NewKeyStore 创建 KeyStore。path 为 keys.json 路径（可为空表示只启用 fallback）。
func NewKeyStore(path, fallback string) *KeyStore {
	ks := &KeyStore{path: path, fallback: fallback, keys: map[string]bool{}}
	ks.reload()
	return ks
}

// Valid 判断 key 是否可用（会按需热重载）。
func (ks *KeyStore) Valid(key string) bool {
	if key == "" {
		return false
	}
	ks.maybeReload()
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	// 常量时间比较（与 handler.withAuth 同口径，见上游"发现 7"）：逐条 ConstantTimeCompare
	// 且不提前 return，避免用比较耗时探测 key 前缀。keys 数量是个位数，O(n) 无成本压力。
	if len(ks.order) > 0 {
		var ok bool
		for _, k := range ks.order {
			if subtle.ConstantTimeCompare([]byte(key), []byte(k)) == 1 {
				ok = true
			}
		}
		return ok
	}
	return ks.fallback != "" && subtle.ConstantTimeCompare([]byte(key), []byte(ks.fallback)) == 1
}

// AuthRequired 是否需要鉴权（两处都空则放行，保持"留空=不鉴权"的既有语义）。
func (ks *KeyStore) AuthRequired() bool {
	ks.maybeReload()
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return len(ks.keys) > 0 || ks.fallback != ""
}

// Keys 返回当前所有 key（供 /status 统计条数，不暴露具体值）。
func (ks *KeyStore) Count() int {
	ks.maybeReload()
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	if len(ks.keys) > 0 {
		return len(ks.keys)
	}
	if ks.fallback != "" {
		return 1
	}
	return 0
}

// maybeReload 检查文件 mtime，变化则重载（最多每秒一次 stat，避免频繁 IO）。
func (ks *KeyStore) maybeReload() {
	if ks.path == "" {
		return
	}
	st, err := os.Stat(ks.path)
	if err != nil {
		return // 文件不存在：保持现状（回退 fallback）
	}
	ks.mu.RLock()
	unchanged := st.ModTime().Equal(ks.mtime)
	ks.mu.RUnlock()
	if unchanged {
		return
	}
	ks.reload()
}

// reload 从磁盘读取 keys.json。
func (ks *KeyStore) reload() {
	if ks.path == "" {
		return
	}
	st, err := os.Stat(ks.path)
	if err != nil {
		return
	}
	raw, err := os.ReadFile(ks.path)
	if err != nil {
		ks.setErr(err.Error())
		return
	}
	var f keysFile
	if err := json.Unmarshal(raw, &f); err != nil {
		// 解析失败保留旧集合，避免写坏文件时全体失联。
		ks.setErr("parse keys.json: " + err.Error())
		return
	}
	next := make(map[string]bool, len(f.Keys))
	order := make([]string, 0, len(f.Keys))
	for _, e := range f.Keys {
		if e.Key != "" {
			next[e.Key] = true
			order = append(order, e.Key)
		}
	}
	ks.mu.Lock()
	ks.keys = next
	ks.order = order
	ks.entries = f.Keys
	ks.mtime = st.ModTime()
	ks.lastErr = ""
	ks.mu.Unlock()
}

func (ks *KeyStore) setErr(msg string) {
	ks.mu.Lock()
	ks.lastErr = msg
	ks.mu.Unlock()
}

// First 返回文件里的第一个 key（文件为空时回退 fallback）。
// 仅供 main 推导管理后台默认 token：让"没单独配 admin.token"的部署也能直接登录。
func (ks *KeyStore) First() string {
	ks.maybeReload()
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	if len(ks.order) > 0 {
		return ks.order[0]
	}
	return ks.fallback
}

// Path 返回 keys.json 路径（管理后台展示用，空表示未启用文件）。
func (ks *KeyStore) Path() string { return ks.path }

// List 返回全部 key（掩码形式），文件顺序稳定排序。
func (ks *KeyStore) List() []KeyInfo {
	ks.maybeReload()
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	out := make([]KeyInfo, 0, len(ks.entries))
	for _, e := range ks.entries {
		if e.Key == "" {
			continue
		}
		out = append(out, KeyInfo{
			Prefix: prefixOf(e.Key),
			Masked: mask(e.Key),
			Note:   e.label(),
			Added:  e.Added,
		})
	}
	// keys.json 缺失时列表为空但 fallback 有效：把 config 的 key 也列出来，
	// 否则后台会显示"无 key"而实际鉴权仍在生效。
	if len(out) == 0 && ks.fallback != "" {
		out = append(out, KeyInfo{Prefix: prefixOf(ks.fallback), Masked: mask(ks.fallback), Note: "来自 config.json"})
	}
	return out
}

// Add 生成并追加一个新 key，写回 keys.json（原子替换）后立即生效。
// 返回完整 key 值——只在创建时返回这一次，之后一律掩码。
func (ks *KeyStore) Add(note string) (string, error) {
	if ks.path == "" {
		return "", fmt.Errorf("未配置 keys_file，无法管理 key")
	}
	key, err := newKey()
	if err != nil {
		return "", err
	}
	entries, err := ks.loadEntries()
	if err != nil {
		return "", err
	}
	entries = append(entries, keyEntry{Key: key, Note: note, Added: time.Now().Format("2006-01-02")})
	if err := ks.writeEntries(entries); err != nil {
		return "", err
	}
	ks.reload()
	return key, nil
}

// Remove 按前缀吊销一个 key（前缀必须唯一匹配，防止误删）。返回剩余数量。
func (ks *KeyStore) Remove(prefix string) (int, error) {
	if ks.path == "" {
		return 0, fmt.Errorf("未配置 keys_file，无法管理 key")
	}
	if strings.TrimSpace(prefix) == "" {
		return 0, fmt.Errorf("前缀不能为空")
	}
	entries, err := ks.loadEntries()
	if err != nil {
		return 0, err
	}
	var hits []int
	for i, e := range entries {
		// 两种前缀都认：后台展示的"去 sk- 前缀"形，以及 CLI 习惯的原样前缀（sk-73b8…）。
		if strings.HasPrefix(e.Key, prefix) || strings.HasPrefix(strings.TrimPrefix(e.Key, "sk-"), prefix) {
			hits = append(hits, i)
		}
	}
	switch {
	case len(hits) == 0:
		return len(entries), fmt.Errorf("没有匹配的 key")
	case len(hits) > 1:
		return len(entries), fmt.Errorf("前缀匹配到 %d 个 key，请用更长前缀", len(hits))
	}
	kept := append(append([]keyEntry{}, entries[:hits[0]]...), entries[hits[0]+1:]...)
	if err := ks.writeEntries(kept); err != nil {
		return len(entries), err
	}
	ks.reload()
	return len(kept), nil
}

// loadEntries 读当前文件条目；文件不存在按空列表处理。
func (ks *KeyStore) loadEntries() ([]keyEntry, error) {
	raw, err := os.ReadFile(ks.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f keysFile
	if err := json.Unmarshal(raw, &f); err != nil {
		// 解析失败不覆盖：写坏的文件比不写更糟。
		return nil, fmt.Errorf("解析 keys.json 失败，已放弃写入：%w", err)
	}
	return f.Keys, nil
}

// writeEntries 原子写回 keys.json（临时文件 + rename），并更新 mtime 触发重载。
func (ks *KeyStore) writeEntries(entries []keyEntry) error {
	raw, err := json.MarshalIndent(keysFile{Keys: entries}, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := filepath.Join(filepath.Dir(ks.path), "."+filepath.Base(ks.path)+".tmp")
	// 0600：key 是凭证，不给同组/其他用户读。
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, ks.path)
}

// newKey 生成 sk-<32 hex> 形态的 key（与 wbapi CLI 一致）。
func newKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sk-" + hex.EncodeToString(buf), nil
}

// prefixOf 取 key 前 8 位作为展示前缀（也是吊销时的匹配前缀）。
func prefixOf(key string) string {
	key = strings.TrimPrefix(key, "sk-")
	if len(key) > 8 {
		return key[:8]
	}
	return key
}

// mask 生成掩码展示形：sk-xxxx…xxxx（保留头尾便于人工核对）。
func mask(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:11] + "…" + key[len(key)-4:]
}
