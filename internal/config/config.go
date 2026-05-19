package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	EnvConfig           = "VCM_CONFIG"
	EnvURL              = "VCM_URL"
	EnvUsername         = "VCM_USERNAME"
	EnvPassword         = "VCM_PASSWORD"
	EnvDatacenter       = "VCM_DATACENTER"
	EnvInsecure         = "VCM_INSECURE"
	EnvDefaultFolder    = "VCM_DEFAULT_FOLDER"
	EnvDefaultDatastore = "VCM_DEFAULT_DATASTORE"
	EnvDefaultPool      = "VCM_DEFAULT_POOL"
)

// Config contains the connection details needed to reach vCenter.
type Config struct {
	URL              string `yaml:"url"`
	Username         string `yaml:"username"`
	Password         string `yaml:"password"`
	Datacenter       string `yaml:"datacenter"`
	Insecure         bool   `yaml:"insecure"`
	DefaultFolder    string `yaml:"defaultFolder"`
	DefaultDatastore string `yaml:"defaultDatastore"`
	DefaultPool      string `yaml:"defaultPool"`
}

// FromEnv loads configuration from VCM_* environment variables.
func FromEnv() Config {
	return FromLookup(os.LookupEnv)
}

// DefaultPath returns the config path used when no explicit path is provided.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vcm", "config.yaml"), nil
}

// Load loads configuration from a YAML file, then overlays VCM_* environment
// variables. A missing implicit config file is ignored.
func Load(path string) (Config, error) {
	return LoadWithLookup(path, os.LookupEnv)
}

// LoadWithLookup is Load with a caller-provided environment lookup for tests.
func LoadWithLookup(path string, lookup func(string) (string, bool)) (Config, error) {
	explicit := strings.TrimSpace(path) != ""
	if !explicit {
		if envPath, ok := lookup(EnvConfig); ok {
			path = strings.TrimSpace(envPath)
			explicit = path != ""
		}
	}
	if strings.TrimSpace(path) == "" {
		defaultPath, err := DefaultPath()
		if err != nil {
			return Config{}, err
		}
		path = defaultPath
	}

	cfg, err := FromFile(path)
	if err != nil {
		if explicit || !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
	}

	cfg.Apply(FromLookup(lookup))
	if v, ok := lookup(EnvInsecure); ok {
		cfg.Insecure = parseBool(v)
	}
	return cfg, nil
}

// FromFile loads configuration from a YAML file.
func FromFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file %q: %w", path, err)
	}
	cfg.trim()
	return cfg, nil
}

// FromLookup loads configuration using a caller-provided environment lookup.
func FromLookup(lookup func(string) (string, bool)) Config {
	cfg := Config{}

	if v, ok := lookup(EnvURL); ok {
		cfg.URL = strings.TrimSpace(v)
	}
	if v, ok := lookup(EnvUsername); ok {
		cfg.Username = strings.TrimSpace(v)
	}
	if v, ok := lookup(EnvPassword); ok {
		cfg.Password = v
	}
	if v, ok := lookup(EnvDatacenter); ok {
		cfg.Datacenter = strings.TrimSpace(v)
	}
	if v, ok := lookup(EnvInsecure); ok {
		cfg.Insecure = parseBool(v)
	}
	if v, ok := lookup(EnvDefaultFolder); ok {
		cfg.DefaultFolder = strings.TrimSpace(v)
	}
	if v, ok := lookup(EnvDefaultDatastore); ok {
		cfg.DefaultDatastore = strings.TrimSpace(v)
	}
	if v, ok := lookup(EnvDefaultPool); ok {
		cfg.DefaultPool = strings.TrimSpace(v)
	}

	return cfg
}

// Apply overlays non-empty values from next. Insecure is only overlaid when true;
// command-line flags handle explicit false values separately.
func (c *Config) Apply(next Config) {
	if strings.TrimSpace(next.URL) != "" {
		c.URL = strings.TrimSpace(next.URL)
	}
	if strings.TrimSpace(next.Username) != "" {
		c.Username = strings.TrimSpace(next.Username)
	}
	if next.Password != "" {
		c.Password = next.Password
	}
	if strings.TrimSpace(next.Datacenter) != "" {
		c.Datacenter = strings.TrimSpace(next.Datacenter)
	}
	if next.Insecure {
		c.Insecure = true
	}
	if strings.TrimSpace(next.DefaultFolder) != "" {
		c.DefaultFolder = strings.TrimSpace(next.DefaultFolder)
	}
	if strings.TrimSpace(next.DefaultDatastore) != "" {
		c.DefaultDatastore = strings.TrimSpace(next.DefaultDatastore)
	}
	if strings.TrimSpace(next.DefaultPool) != "" {
		c.DefaultPool = strings.TrimSpace(next.DefaultPool)
	}
}

func (c *Config) trim() {
	c.URL = strings.TrimSpace(c.URL)
	c.Username = strings.TrimSpace(c.Username)
	c.Datacenter = strings.TrimSpace(c.Datacenter)
	c.DefaultFolder = strings.TrimSpace(c.DefaultFolder)
	c.DefaultDatastore = strings.TrimSpace(c.DefaultDatastore)
	c.DefaultPool = strings.TrimSpace(c.DefaultPool)
}

func parseBool(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}

// NormalizeURL accepts a bare vCenter host or a full SDK URL and returns the
// canonical endpoint used by govmomi.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host in vCenter URL %q", raw)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/sdk"
	}

	return u.String(), nil
}

// Validate returns an error if the config cannot be used to connect to vCenter.
func (c Config) Validate() error {
	var missing []string
	if strings.TrimSpace(c.URL) == "" {
		missing = append(missing, EnvURL)
	}
	if strings.TrimSpace(c.Username) == "" {
		missing = append(missing, EnvUsername)
	}
	if c.Password == "" {
		missing = append(missing, EnvPassword)
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}
	if _, err := NormalizeURL(c.URL); err != nil {
		return fmt.Errorf("invalid %s: %w", EnvURL, err)
	}
	return nil
}

// ErrNoDatacenter is returned when an operation needs a datacenter but none was
// configured or resolved.
var ErrNoDatacenter = errors.New("no datacenter configured or resolved")
