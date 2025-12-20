package keys

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Store handles API key storage and retrieval
type Store struct {
	configDir string
}

// KeyEntry represents a stored API key
type KeyEntry struct {
	Key string `json:"key"`
}

// Keys represents the keys.json structure
type Keys map[string]KeyEntry

// NewStore creates a new key store
func NewStore() (*Store, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{configDir: configDir}, nil
}

// getConfigDir returns the platform-specific config directory
func getConfigDir() (string, error) {
	// Allow override for testing
	if testDir := os.Getenv("IMGGEN_CONFIG_DIR"); testDir != "" {
		return testDir, nil
	}

	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "imggen"), nil
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "imggen"), nil
	default: // linux and others
		// Follow XDG Base Directory Specification
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, "imggen"), nil
	}
}

// Path returns the path to the keys.json file
func (s *Store) Path() string {
	return filepath.Join(s.configDir, "keys.json")
}

// load reads the keys from disk
func (s *Store) load() (Keys, error) {
	path := s.Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(Keys), nil
		}
		return nil, err
	}

	var keys Keys
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("failed to parse keys.json: %w", err)
	}
	return keys, nil
}

// save writes the keys to disk
func (s *Store) save(keys Keys) error {
	// Ensure directory exists
	if err := os.MkdirAll(s.configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}

	path := s.Path()
	// Write with restricted permissions (owner read/write only)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write keys.json: %w", err)
	}
	return nil
}

// Set stores a key for the given provider
func (s *Store) Set(provider, key string) error {
	keys, err := s.load()
	if err != nil {
		return err
	}

	keys[provider] = KeyEntry{Key: key}
	return s.save(keys)
}

// Get retrieves a key for the given provider
func (s *Store) Get(provider string) (string, error) {
	keys, err := s.load()
	if err != nil {
		return "", err
	}

	entry, ok := keys[provider]
	if !ok {
		return "", nil // Key not found, not an error
	}
	return entry.Key, nil
}

// Delete removes a key for the given provider
func (s *Store) Delete(provider string) error {
	keys, err := s.load()
	if err != nil {
		return err
	}

	if _, ok := keys[provider]; !ok {
		return fmt.Errorf("no key found for %s", provider)
	}

	delete(keys, provider)
	return s.save(keys)
}

// List returns all stored provider names
func (s *Store) List() ([]string, error) {
	keys, err := s.load()
	if err != nil {
		return nil, err
	}

	providers := make([]string, 0, len(keys))
	for provider := range keys {
		providers = append(providers, provider)
	}
	return providers, nil
}

// Exists checks if a key exists for the given provider
func (s *Store) Exists(provider string) (bool, error) {
	keys, err := s.load()
	if err != nil {
		return false, err
	}
	_, ok := keys[provider]
	return ok, nil
}

// MaskKey returns a masked version of the key for display
func MaskKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// GetAPIKey retrieves the API key using the priority order:
// 1. Explicit key passed as argument (if non-empty)
// 2. Stored key in keys.json
// 3. Environment variable
func GetAPIKey(explicitKey, provider, envVar string) (string, string, error) {
	// 1. Explicit key has highest priority
	if explicitKey != "" {
		return explicitKey, "command-line flag", nil
	}

	// 2. Check stored key
	store, err := NewStore()
	if err == nil {
		storedKey, err := store.Get(provider)
		if err == nil && storedKey != "" {
			return storedKey, fmt.Sprintf("stored key (~/.config/imggen/keys.json)"), nil
		}
	}

	// 3. Fall back to environment variable
	if envKey := os.Getenv(envVar); envKey != "" {
		return envKey, fmt.Sprintf("environment variable (%s)", envVar), nil
	}

	return "", "", fmt.Errorf("API key required: run 'imggen keys set' or set %s environment variable", envVar)
}
