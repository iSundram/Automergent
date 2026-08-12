package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SecretManager handles secure storage and retrieval of sensitive configuration.
type SecretManager struct {
	// Backend determines where secrets are stored.
	Backend SecretBackend
	// KeyringService is the keyring service name for system keyring backend.
	KeyringService string
	// FilePath is the path for file-based secret storage.
	FilePath string
}

// SecretBackend represents a secret storage backend.
type SecretBackend int

const (
	// BackendEnv uses environment variables for secrets.
	BackendEnv SecretBackend = iota
	// BackendFile uses an encrypted file for secrets.
	BackendFile
	// BackendKeyring uses the system keyring (if available).
	BackendKeyring
)

// String returns the backend name.
func (b SecretBackend) String() string {
	switch b {
	case BackendEnv:
		return "env"
	case BackendFile:
		return "file"
	case BackendKeyring:
		return "keyring"
	default:
		return "unknown"
	}
}

// SecretRef references a secret by name.
type SecretRef struct {
	Name     string
	Backend  SecretBackend
	EnvVar   string
	FilePath string
}

// NewSecretManager creates a new secret manager.
func NewSecretManager(backend SecretBackend) (*SecretManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	return &SecretManager{
		Backend:        backend,
		KeyringService: "automergent",
		FilePath:       filepath.Join(home, ".automergent", "secrets.yaml"),
	}, nil
}

// Set stores a secret.
func (sm *SecretManager) Set(name, value string) error {
	switch sm.Backend {
	case BackendEnv:
		// For env backend, we just document the expected env var
		return fmt.Errorf("cannot set env secrets directly; set AUTOMERGENT_%s environment variable",
			strings.ToUpper(name))

	case BackendFile:
		return sm.setFileSecret(name, value)

	case BackendKeyring:
		// Keyring integration would go here
		// For now, fall back to file
		return sm.setFileSecret(name, value)
	}

	return fmt.Errorf("unknown backend: %v", sm.Backend)
}

// Get retrieves a secret.
func (sm *SecretManager) Get(name string) (string, error) {
	// Always check env first
	envKey := "AUTOMERGENT_" + strings.ToUpper(strings.ReplaceAll(name, ".", "_"))
	if value := os.Getenv(envKey); value != "" {
		return value, nil
	}

	switch sm.Backend {
	case BackendEnv:
		return "", fmt.Errorf("secret %q not found in environment", name)

	case BackendFile:
		return sm.getFileSecret(name)

	case BackendKeyring:
		// Try keyring first, fall back to file
		value, err := sm.getFileSecret(name)
		if err == nil {
			return value, nil
		}
		return "", fmt.Errorf("secret %q not found", name)
	}

	return "", fmt.Errorf("unknown backend: %v", sm.Backend)
}

// Delete removes a secret.
func (sm *SecretManager) Delete(name string) error {
	switch sm.Backend {
	case BackendEnv:
		return fmt.Errorf("cannot delete env secrets; unset AUTOMERGENT_%s",
			strings.ToUpper(name))

	case BackendFile:
		return sm.deleteFileSecret(name)

	case BackendKeyring:
		return sm.deleteFileSecret(name)
	}

	return fmt.Errorf("unknown backend: %v", sm.Backend)
}

// List returns all secret names.
func (sm *SecretManager) List() ([]string, error) {
	var names []string

	// Check env vars
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "AUTOMERGENT_") {
			parts := strings.SplitN(env, "=", 2)
			name := strings.ToLower(strings.TrimPrefix(parts[0], "AUTOMERGENT_"))
			names = append(names, name)
		}
	}

	// Check file
	if sm.Backend == BackendFile || sm.Backend == BackendKeyring {
		fileSecrets, err := sm.listFileSecrets()
		if err == nil {
			names = append(names, fileSecrets...)
		}
	}

	return names, nil
}

// setFileSecret stores a secret in the encrypted file.
func (sm *SecretManager) setFileSecret(name, value string) error {
	secrets, err := sm.loadSecretFile()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if secrets == nil {
		secrets = make(map[string]string)
	}

	// Simple obfuscation (not real encryption - for production use a proper crypto library)
	secrets[name] = base64.StdEncoding.EncodeToString([]byte(value))

	return sm.saveSecretFile(secrets)
}

// getFileSecret retrieves a secret from the file.
func (sm *SecretManager) getFileSecret(name string) (string, error) {
	secrets, err := sm.loadSecretFile()
	if err != nil {
		return "", err
	}

	encoded, ok := secrets[name]
	if !ok {
		return "", fmt.Errorf("secret %q not found", name)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}

	return string(decoded), nil
}

// deleteFileSecret removes a secret from the file.
func (sm *SecretManager) deleteFileSecret(name string) error {
	secrets, err := sm.loadSecretFile()
	if err != nil {
		return err
	}

	delete(secrets, name)
	return sm.saveSecretFile(secrets)
}

// listFileSecrets returns all secret names from the file.
func (sm *SecretManager) listFileSecrets() ([]string, error) {
	secrets, err := sm.loadSecretFile()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	return names, nil
}

// loadSecretFile loads the secret file.
func (sm *SecretManager) loadSecretFile() (map[string]string, error) {
	content, err := os.ReadFile(sm.FilePath)
	if err != nil {
		return nil, err
	}

	var secrets map[string]string
	if err := yaml.Unmarshal(content, &secrets); err != nil {
		return nil, fmt.Errorf("parse secrets: %w", err)
	}

	return secrets, nil
}

// saveSecretFile saves the secret file.
func (sm *SecretManager) saveSecretFile(secrets map[string]string) error {
	dir := filepath.Dir(sm.FilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create secrets dir: %w", err)
	}

	data, err := yaml.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("marshal secrets: %w", err)
	}

	if err := os.WriteFile(sm.FilePath, data, 0o600); err != nil {
		return fmt.Errorf("write secrets: %w", err)
	}

	return nil
}

// ResolveSecretRefs resolves secret references in a config.
func (sm *SecretManager) ResolveSecretRefs(cfg *Config) error {
	// Resolve provider API keys
	for name, provider := range cfg.Providers {
		if strings.HasPrefix(provider.APIKey, "${") && strings.HasSuffix(provider.APIKey, "}") {
			secretName := strings.TrimSuffix(strings.TrimPrefix(provider.APIKey, "${"), "}")
			value, err := sm.Get(secretName)
			if err != nil {
				return fmt.Errorf("resolve secret %q for provider %q: %w", secretName, name, err)
			}
			provider.APIKey = value
			cfg.Providers[name] = provider
		}
	}

	return nil
}

// ParseSecretRef parses a secret reference string.
func ParseSecretRef(ref string) (*SecretRef, error) {
	// Format: ${secret:name} or ${env:VAR_NAME} or ${file:/path/to/file}
	if !strings.HasPrefix(ref, "${") || !strings.HasSuffix(ref, "}") {
		return nil, fmt.Errorf("invalid secret reference format")
	}

	inner := strings.TrimSuffix(strings.TrimPrefix(ref, "${"), "}")
	parts := strings.SplitN(inner, ":", 2)

	if len(parts) != 2 {
		return &SecretRef{Name: inner, Backend: BackendFile}, nil
	}

	sr := &SecretRef{}

	switch parts[0] {
	case "secret":
		sr.Backend = BackendFile
		sr.Name = parts[1]
	case "env":
		sr.Backend = BackendEnv
		sr.EnvVar = parts[1]
	case "file":
		sr.Backend = BackendFile
		sr.FilePath = parts[1]
	case "keyring":
		sr.Backend = BackendKeyring
		sr.Name = parts[1]
	default:
		return nil, fmt.Errorf("unknown secret backend: %s", parts[0])
	}

	return sr, nil
}

// GetProviderAPIKey resolves the API key for a provider.
func GetProviderAPIKey(cfg *Config, providerName string) (string, error) {
	// Check direct config
	if pc, ok := cfg.Providers[providerName]; ok && pc.APIKey != "" {
		// Check if it's a secret reference
		if strings.HasPrefix(pc.APIKey, "${") {
			sm, err := NewSecretManager(BackendFile)
			if err != nil {
				return "", err
			}
			ref, err := ParseSecretRef(pc.APIKey)
			if err != nil {
				return "", err
			}
			return sm.Get(ref.Name)
		}
		return pc.APIKey, nil
	}

	// Check environment variables
	envKeys := map[string][]string{
		"google": {"GOOGLE_API_KEY", "GEMINI_API_KEY"},
	}

	for _, envKey := range envKeys[providerName] {
		if value := os.Getenv(envKey); value != "" {
			return value, nil
		}
	}

	// Check secret manager
	sm, err := NewSecretManager(BackendFile)
	if err != nil {
		return "", err
	}

	return sm.Get(providerName + "_api_key")
}
