package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeURLAddsSchemeAndSDKPath(t *testing.T) {
	got, err := NormalizeURL("vcsa.example.com")
	if err != nil {
		t.Fatalf("NormalizeURL returned error: %v", err)
	}

	want := "https://vcsa.example.com/sdk"
	if got != want {
		t.Fatalf("NormalizeURL = %q, want %q", got, want)
	}
}

func TestNormalizeURLPreservesExplicitPath(t *testing.T) {
	got, err := NormalizeURL("https://vcsa.example.com/custom")
	if err != nil {
		t.Fatalf("NormalizeURL returned error: %v", err)
	}

	want := "https://vcsa.example.com/custom"
	if got != want {
		t.Fatalf("NormalizeURL = %q, want %q", got, want)
	}
}

func TestFromLookupLoadsVCMEnvironment(t *testing.T) {
	env := map[string]string{
		EnvURL:              " vcsa.example.com ",
		EnvUsername:         " administrator@vsphere.local ",
		EnvPassword:         "secret",
		EnvDatacenter:       " Lab ",
		EnvInsecure:         "true",
		EnvDefaultFolder:    " Lab VMs ",
		EnvDefaultDatastore: " datastore1 ",
		EnvDefaultPool:      " Resources ",
	}

	cfg := FromLookup(func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	})

	if cfg.URL != "vcsa.example.com" {
		t.Fatalf("URL = %q", cfg.URL)
	}
	if cfg.Username != "administrator@vsphere.local" {
		t.Fatalf("Username = %q", cfg.Username)
	}
	if cfg.Password != "secret" {
		t.Fatalf("Password = %q", cfg.Password)
	}
	if cfg.Datacenter != "Lab" {
		t.Fatalf("Datacenter = %q", cfg.Datacenter)
	}
	if !cfg.Insecure {
		t.Fatal("Insecure = false, want true")
	}
	if cfg.DefaultFolder != "Lab VMs" {
		t.Fatalf("DefaultFolder = %q", cfg.DefaultFolder)
	}
	if cfg.DefaultDatastore != "datastore1" {
		t.Fatalf("DefaultDatastore = %q", cfg.DefaultDatastore)
	}
	if cfg.DefaultPool != "Resources" {
		t.Fatalf("DefaultPool = %q", cfg.DefaultPool)
	}
}

func TestFromFileLoadsYAMLConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
url: vcenter.spectrocloud.dev
username: ram
datacenter: Datacenter
insecure: true
defaultFolder: sp-ramganesh.senthilkumar
defaultDatastore: vsanDatastore1
defaultPool: /Datacenter/host/Cluster1/Resources
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := FromFile(path)
	if err != nil {
		t.Fatalf("FromFile returned error: %v", err)
	}

	if cfg.URL != "vcenter.spectrocloud.dev" {
		t.Fatalf("URL = %q", cfg.URL)
	}
	if cfg.DefaultPool != "/Datacenter/host/Cluster1/Resources" {
		t.Fatalf("DefaultPool = %q", cfg.DefaultPool)
	}
	if !cfg.Insecure {
		t.Fatal("Insecure = false, want true")
	}
}

func TestLoadWithLookupOverlaysEnvOverFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
url: file.example.com
username: file-user
password: file-password
datacenter: FileDC
insecure: true
defaultFolder: file-folder
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	env := map[string]string{
		EnvURL:      "env.example.com",
		EnvInsecure: "false",
	}
	cfg, err := LoadWithLookup(path, func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("LoadWithLookup returned error: %v", err)
	}

	if cfg.URL != "env.example.com" {
		t.Fatalf("URL = %q", cfg.URL)
	}
	if cfg.Username != "file-user" {
		t.Fatalf("Username = %q", cfg.Username)
	}
	if cfg.Insecure {
		t.Fatal("Insecure = true, want false")
	}
	if cfg.DefaultFolder != "file-folder" {
		t.Fatalf("DefaultFolder = %q", cfg.DefaultFolder)
	}
}

func TestValidateRequiresConnectionFields(t *testing.T) {
	err := (Config{}).Validate()
	if err == nil {
		t.Fatal("Validate returned nil error")
	}
}
