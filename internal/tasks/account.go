package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"workbuddy2api/internal/auth"
)

// Account is a read-only credential snapshot for task automation.
type Account struct {
	Token    string
	UID      string
	Nickname string
	Domain   string
	Realm    string
	File     string
}

func accountFromAuth(a *auth.Auth) Account {
	return Account{
		Token:    a.AccessTokenValue(),
		UID:      a.UID,
		Nickname: a.Nickname,
		Domain:   a.DomainValue(),
		Realm:    a.RealmStored(),
		File:     filepath.Base(a.FilePath),
	}
}

// IsGlobal deliberately ignores auth.SetGlobalEnabled: task scripts operate on
// the identity of the credential, not on the gateway's CN-only routing escape.
func (a Account) IsGlobal() bool {
	if strings.EqualFold(strings.TrimSpace(a.Realm), "global") {
		return true
	}
	return auth.ResolveRealm("", a.Domain) == "global"
}

func (a Account) UID8() string {
	runes := []rune(a.UID)
	if len(runes) > 8 {
		return string(runes[:8])
	}
	if len(runes) > 0 {
		return string(runes)
	}
	return "?"
}

func resolveAuthsDir() string {
	if dir := os.Getenv("WB2A_AUTHS"); dir != "" {
		return dir
	}
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "auths"))
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "auths"))
	}
	for _, dir := range candidates {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return "/root/workbuddy2api/auths"
}

func listAllAuthFiles(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "workbuddy-*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func loadAccountFile(path string) (Account, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Account{}, err
	}
	a, err := auth.Parse(raw)
	if err != nil {
		return Account{}, err
	}
	a.FilePath = path
	return accountFromAuth(a), nil
}

func (s *Service) loadAccount(selector string) (Account, error) {
	var path string
	if strings.ContainsRune(selector, filepath.Separator) || strings.HasSuffix(selector, ".json") {
		path = selector
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.AuthsDir, path)
		}
	} else {
		hits, err := filepath.Glob(filepath.Join(s.AuthsDir, "workbuddy-"+selector+"*.json"))
		if err != nil {
			return Account{}, err
		}
		if len(hits) == 0 {
			return Account{}, fmt.Errorf("no auth for %s", selector)
		}
		path = hits[0]
	}
	return loadAccountFile(path)
}

func (s *Service) allAccounts() ([]Account, error) {
	out, _, err := s.allAccountResults()
	return out, err
}

func (s *Service) allAccountResults() ([]Account, []error, error) {
	prefixes, err := s.allAuthPrefixes()
	if err != nil {
		return nil, nil, err
	}
	prefixes = dedupStrings(prefixes)
	out := make([]Account, 0, len(prefixes))
	errs := []error{}
	for _, prefix := range prefixes {
		a, err := s.loadAccount(prefix)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", prefix, err))
			continue
		}
		out = append(out, a)
	}
	return out, errs, nil
}

func (s *Service) allAuthPrefixes() ([]string, error) {
	files, err := listAllAuthFiles(s.AuthsDir)
	if err != nil {
		return nil, err
	}
	prefixes := make([]string, 0, len(files))
	for _, path := range files {
		name := filepath.Base(path)
		if len(name) <= 10 {
			continue
		}
		prefix := name[10:]
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}
