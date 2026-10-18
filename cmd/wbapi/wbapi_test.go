package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagedPathStaysInsideModule(t *testing.T) {
	root := t.TempDir()
	got, err := managedPath(root, "auths", "auths")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "auths")
	if got != want {
		t.Fatalf("managedPath() = %q, want %q", got, want)
	}
	if _, err := managedPath(root, "/data/adb/elsewhere", "auths"); err == nil {
		t.Fatal("managedPath accepted a path outside the module")
	}
}

func TestKeyRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.json")
	doc := keyDocument{Keys: []keyEntry{{Key: "sk-1234567890abcdef", Note: "test"}}}
	if err := saveKeys(path, doc); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("keys mode = %o, want 600", info.Mode().Perm())
	}
	got, err := loadKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Keys) != 1 || got.Keys[0].Key != doc.Keys[0].Key {
		t.Fatalf("round trip mismatch: %#v", got)
	}
}

func TestEnvironmentWithReplacesValue(t *testing.T) {
	t.Setenv("WBAPI_TEST_VALUE", "old")
	env := environmentWith(map[string]string{"WBAPI_TEST_VALUE": "new"})
	count := 0
	for _, entry := range env {
		if entry == "WBAPI_TEST_VALUE=new" {
			count++
		}
		if entry == "WBAPI_TEST_VALUE=old" {
			t.Fatal("old environment value was retained")
		}
	}
	if count != 1 {
		t.Fatalf("replacement count = %d, want 1", count)
	}
}

func TestAPIError(t *testing.T) {
	got := apiError([]byte(`{"error":{"code":"bad","message":"broken"}}`))
	if got != "broken" {
		t.Fatalf("apiError() = %q", got)
	}
}
