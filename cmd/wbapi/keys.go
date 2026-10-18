package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type keyEntry struct {
	Key   string `json:"key"`
	Note  string `json:"note,omitempty"`
	Added string `json:"added,omitempty"`
}

type keyDocument struct {
	Keys []keyEntry `json:"keys"`
}

func loadKeys(path string) (keyDocument, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return keyDocument{}, nil
	}
	if err != nil {
		return keyDocument{}, err
	}
	var doc keyDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return keyDocument{}, err
	}
	return doc, nil
}

func saveKeys(path string, doc keyDocument) error {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeFileAtomic(path, raw, 0o600)
}

func newKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sk-" + hex.EncodeToString(buf), nil
}

func (e *moduleEnv) gatewayToken() string {
	doc, err := loadKeys(e.KeysFile)
	if err == nil && len(doc.Keys) > 0 {
		return doc.Keys[0].Key
	}
	return e.Config.APIKey
}

func (e *moduleEnv) adminToken() string {
	if e.Config.Admin.Token != "" {
		return e.Config.Admin.Token
	}
	return e.gatewayToken()
}

func (e *moduleEnv) keyList(show bool) error {
	doc, err := loadKeys(e.KeysFile)
	if err != nil {
		return err
	}
	if len(doc.Keys) == 0 {
		fmt.Printf("keys.json 为空: %s\n", e.KeysFile)
		return nil
	}
	fmt.Printf("共 %d 个 key:\n", len(doc.Keys))
	for _, entry := range doc.Keys {
		value := maskKey(entry.Key)
		if show {
			value = entry.Key
		}
		fmt.Printf("  %s  %s  %s\n", value, entry.Note, entry.Added)
	}
	if !show {
		fmt.Println("使用 --show 显示完整值")
	}
	return nil
}

func (e *moduleEnv) keyNew(note string) error {
	doc, err := loadKeys(e.KeysFile)
	if err != nil {
		return err
	}
	value, err := newKey()
	if err != nil {
		return err
	}
	if strings.TrimSpace(note) == "" {
		note = "未命名"
	}
	doc.Keys = append(doc.Keys, keyEntry{Key: value, Note: note, Added: time.Now().Format("2006-01-02")})
	if err := saveKeys(e.KeysFile, doc); err != nil {
		return err
	}
	fmt.Printf("已添加 %s\n\n%s\n\n立即生效，无需重启\n", note, value)
	return nil
}

func (e *moduleEnv) keyRevoke(prefix string) error {
	doc, err := loadKeys(e.KeysFile)
	if err != nil {
		return err
	}
	var hits []int
	for i, entry := range doc.Keys {
		if strings.HasPrefix(entry.Key, prefix) || strings.HasPrefix(strings.TrimPrefix(entry.Key, "sk-"), prefix) {
			hits = append(hits, i)
		}
	}
	if len(hits) != 1 {
		return fmt.Errorf("前缀匹配到 %d 个 key，请写得更具体", len(hits))
	}
	removed := doc.Keys[hits[0]]
	doc.Keys = append(doc.Keys[:hits[0]], doc.Keys[hits[0]+1:]...)
	if err := saveKeys(e.KeysFile, doc); err != nil {
		return err
	}
	fmt.Printf("已吊销 %s，剩余 %d 个\n", maskKey(removed.Key), len(doc.Keys))
	return nil
}

func maskKey(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:8] + "..." + value[len(value)-4:]
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".wbapi-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
