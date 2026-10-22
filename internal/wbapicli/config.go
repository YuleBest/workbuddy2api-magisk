package wbapicli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func newApp(opts Options) *App {
	if opts.Stdout == nil {
		opts.Stdout = discardWriter{}
	}
	if opts.Stderr == nil {
		opts.Stderr = discardWriter{}
	}
	if opts.Stdin == nil {
		opts.Stdin = strings.NewReader("")
	}
	deps := opts.Deps.withDefaults()
	project := opts.Project
	if v, ok := opts.Env.get("WB2A_HOME"); ok && v != "" {
		project = v
	}
	if project == "" {
		project = "."
	}
	cfgPath := filepath.Join(project, "config.json")
	if v := opts.Env["WB2A_CONFIG"]; v != "" {
		cfgPath = v
	}
	publicHost := "aiapi.yule.best"
	if v, ok := opts.Env.get("WB2A_PUBLIC_HOST"); ok {
		publicHost = v
	}
	return &App{
		env:        opts.Env,
		project:    project,
		configPath: cfgPath,
		publicHost: publicHost,
		deps:       deps,
		stdout:     opts.Stdout,
		stderr:     opts.Stderr,
		stdin:      opts.Stdin,
		osName:     deps.Platform.GOOS(),
		now:        deps.Now(),
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func decodeJSON(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON value")
		}
		return nil, err
	}
	return v, nil
}

func decodeObject(raw []byte) (map[string]any, error) {
	v, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return map[string]any{}, nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected JSON object")
	}
	return obj, nil
}

func (a *App) loadConfig() {
	if a.cfgLoaded {
		return
	}
	a.cfgLoaded = true
	raw, err := a.deps.FS.ReadFile(a.configPath)
	if err != nil {
		a.cfg = map[string]any{}
		a.cfgRaw = []byte("{}")
		if !isNotExist(err) {
			a.cfgWarn = err
			if !a.quietConfig {
				fmt.Fprintf(a.stdout, "[!] 配置文件解析失败（%s）: %v\n", a.configPath, err)
			}
		}
		return
	}
	a.cfgRaw = append([]byte(nil), raw...)
	cfg, err := decodeObject(raw)
	if err != nil {
		a.cfg = map[string]any{}
		a.cfgWarn = err
		if !a.quietConfig {
			fmt.Fprintf(a.stdout, "[!] 配置文件解析失败（%s）: %v\n", a.configPath, err)
		}
		return
	}
	a.cfg = cfg
}

func isNotExist(err error) bool {
	return osIsNotExist(err)
}

func (a *App) baseURL() string {
	a.loadConfig()
	if v := a.env["WB2A_GATEWAY"]; v != "" {
		return strings.TrimRight(v, "/")
	}
	listen := fallbackString(a.cfg["listen"], ":7863")
	host, port := "127.0.0.1", listen
	if idx := strings.LastIndex(listen, ":"); idx >= 0 {
		host, port = listen[:idx], listen[idx+1:]
		switch host {
		case "", "0.0.0.0", "::", "[::]":
			host = "127.0.0.1"
		}
	}
	return "http://" + host + ":" + port
}

func (a *App) keysPath() string {
	a.loadConfig()
	kf := fallbackString(a.cfg["keys_file"], "keys.json")
	if !filepath.IsAbs(kf) {
		kf = filepath.Join(filepath.Dir(resolvedPath(a.configPath)), kf)
	}
	return filepath.Clean(kf)
}

func (a *App) loadKeys() map[string]any {
	raw, err := a.deps.FS.ReadFile(a.keysPath())
	if err != nil {
		return map[string]any{"keys": []any{}}
	}
	obj, err := decodeObject(raw)
	if err != nil {
		return map[string]any{"keys": []any{}}
	}
	return obj
}

func (a *App) saveKeys(doc map[string]any) error {
	raw, err := marshalKeysDocument(doc)
	if err != nil {
		return err
	}
	path := a.keysPath()
	if err := a.deps.FS.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return a.deps.FS.Chmod(path, 0o600)
}

func marshalKeysDocument(doc map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeOrderedJSON(&buf, doc, 0, []string{"keys"}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeOrderedJSON(buf *bytes.Buffer, value any, depth int, preferred []string) error {
	switch typed := value.(type) {
	case map[string]any:
		keys := orderedObjectKeys(typed, preferred)
		if len(keys) == 0 {
			buf.WriteString("{}")
			return nil
		}
		buf.WriteString("{\n")
		for i, key := range keys {
			buf.WriteString(strings.Repeat("  ", depth+1))
			name, err := marshalJSONScalar(key)
			if err != nil {
				return err
			}
			buf.Write(name)
			buf.WriteString(": ")
			childPreferred := []string(nil)
			if key == "keys" {
				childPreferred = []string{"key", "note", "added", "comment"}
			}
			if err := writeOrderedJSON(buf, typed[key], depth+1, childPreferred); err != nil {
				return err
			}
			if i+1 < len(keys) {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteString(strings.Repeat("  ", depth))
		buf.WriteByte('}')
		return nil
	case []any:
		if len(typed) == 0 {
			buf.WriteString("[]")
			return nil
		}
		buf.WriteString("[\n")
		for i, item := range typed {
			buf.WriteString(strings.Repeat("  ", depth+1))
			if err := writeOrderedJSON(buf, item, depth+1, preferred); err != nil {
				return err
			}
			if i+1 < len(typed) {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteString(strings.Repeat("  ", depth))
		buf.WriteByte(']')
		return nil
	default:
		raw, err := marshalJSONScalar(value)
		if err != nil {
			return err
		}
		buf.Write(raw)
		return nil
	}
}

func orderedObjectKeys(obj map[string]any, preferred []string) []string {
	seen := make(map[string]bool, len(obj))
	keys := make([]string, 0, len(obj))
	for _, key := range preferred {
		if _, ok := obj[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	var rest []string
	for key := range obj {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func marshalJSONScalar(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	raw := buf.Bytes()
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	return raw, nil
}

func (a *App) gatewayToken() string {
	a.loadConfig()
	ks, _ := a.loadKeys()["keys"].([]any)
	if len(ks) > 0 {
		if first, ok := ks[0].(map[string]any); ok {
			return tokenString(first["key"])
		}
		return ""
	}
	return tokenString(a.cfg["api_key"])
}

func (a *App) adminToken() string {
	a.loadConfig()
	admin, _ := a.cfg["admin"].(map[string]any)
	if tok := tokenString(admin["token"]); tok != "" {
		return tok
	}
	return a.gatewayToken()
}

func tokenString(v any) string {
	if pyFalsy(v) {
		return ""
	}
	return stringValue(v)
}

func (a *App) authDir() string {
	a.loadConfig()
	dir := fallbackString(a.cfg["auth_dir"], "./auths")
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(filepath.Dir(resolvedPath(a.configPath)), dir)
	}
	return filepath.Clean(dir)
}

func resolvedPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved
	}
	return absolute
}

func stringValue(v any) string {
	switch x := v.(type) {
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
		return strconv.FormatFloat(x, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}
