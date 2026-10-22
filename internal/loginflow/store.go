package loginflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Credentials is the plugin-compatible account document written after OAuth.
type Credentials struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
	Domain       string
	Realm        string
	UID          string
	EnterpriseID string
	Nickname     string
}

type authDocument struct {
	Account struct {
		UID          string `json:"uid"`
		EnterpriseID string `json:"enterpriseId"`
		Nickname     string `json:"nickname"`
	} `json:"account"`
	Auth struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
		Domain       string `json:"domain"`
		Realm        string `json:"realm"`
	} `json:"auth"`
}

// AuthFilePath returns the filename used by both the plugin and gateway loader.
func AuthFilePath(dir, uid string) string {
	return filepath.Join(dir, "workbuddy-"+uid+".json")
}

// MarshalAuthFile builds the nested plugin format with one-space indentation and
// no trailing newline.
func MarshalAuthFile(creds Credentials) ([]byte, error) {
	if strings.TrimSpace(creds.UID) == "" {
		return nil, errors.New("save refused: empty uid")
	}
	if strings.TrimSpace(creds.AccessToken) == "" {
		return nil, errors.New("save refused: empty accessToken")
	}
	var doc authDocument
	doc.Account.UID = creds.UID
	doc.Account.EnterpriseID = creds.EnterpriseID
	doc.Account.Nickname = creds.Nickname
	doc.Auth.AccessToken = creds.AccessToken
	doc.Auth.RefreshToken = creds.RefreshToken
	doc.Auth.ExpiresAt = creds.ExpiresAt
	doc.Auth.Domain = creds.Domain
	doc.Auth.Realm = creds.Realm
	return json.MarshalIndent(doc, "", " ")
}

// WriteAuthFile writes a plugin-readable credential file through a same-directory
// temporary file and rename. The final file and temporary file are both 0600.
func WriteAuthFile(dir string, creds Credentials) (string, error) {
	raw, err := MarshalAuthFile(creds)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".workbuddy-auth-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	finalPath := AuthFilePath(dir, creds.UID)
	if err := os.Rename(tmpName, finalPath); err != nil {
		return "", err
	}
	removeTemp = false
	if err := os.Chmod(finalPath, 0o600); err != nil {
		return "", err
	}
	if directory, err := os.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return finalPath, nil
}

// PrepareAuthDir fails before OAuth starts when the destination cannot receive a
// temporary credential file.
func PrepareAuthDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".wb2api-write-test-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

// PermissionAdvice returns deployment-specific repair instructions.
func PermissionAdvice(path string, expectedUID int, lookupName func(int) (string, error)) []string {
	if expectedUID < 0 {
		return []string{"    请检查当前用户对该目录的读写权限"}
	}
	name := fmt.Sprintf("%d", expectedUID)
	if lookupName != nil {
		if got, err := lookupName(expectedUID); err == nil && strings.TrimSpace(got) != "" {
			name = got
		}
	}
	return []string{fmt.Sprintf("    请执行：sudo chown -R %s:%s %s", name, name, path)}
}
