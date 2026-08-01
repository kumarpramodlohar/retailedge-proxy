package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Config holds all site-specific configuration for the Store VM.
// Loaded from /etc/retailedge/site.conf at startup.
// No service discovery — every value is static per store.
type Config struct {
	StoreID        string
	CloudAPIURL    string
	PubSubProject  string
	PubSubSubscription string
	CredentialsFile string
	DBPath         string
	SocketPath     string
}

// Load reads the site config file and returns a populated Config.
// Returns error if any required field is missing.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		values[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{
		StoreID:            values["STORE_ID"],
		CloudAPIURL:        values["CLOUD_API_URL"],
		PubSubProject:      values["PUBSUB_PROJECT"],
		PubSubSubscription: values["PUBSUB_SUBSCRIPTION"],
		CredentialsFile:    values["GOOGLE_APPLICATION_CREDENTIALS"],
		DBPath:             values["DB_PATH"],
		SocketPath:         values["SOCKET_PATH"],
	}

	return cfg, cfg.validate()
}

// validate checks all required fields are present.
func (c *Config) validate() error {
	required := map[string]string{
		"STORE_ID":                       c.StoreID,
		"PUBSUB_PROJECT":                 c.PubSubProject,
		"PUBSUB_SUBSCRIPTION":            c.PubSubSubscription,
		"GOOGLE_APPLICATION_CREDENTIALS": c.CredentialsFile,
		"DB_PATH":                        c.DBPath,
		"SOCKET_PATH":                    c.SocketPath,
	}
	for key, val := range required {
		if val == "" {
			return fmt.Errorf("missing required config: %s", key)
		}
	}
	return nil
}