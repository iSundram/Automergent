package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
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

// maxSecretFileSize limits the size of the secrets file to prevent DoS when reading.
const maxSecretFileSize = 1 << 20 // 1 MiB

// setFileSecret stores a secret in the encrypted file. If an environment passphrase
// AUTOMERGENT_SECRET_PASSPHRASE is set, the secret will be encrypted with AES-GCM
// using a key derived via PBKDF2. Otherwise, for backward compatibility, it will
// be stored as legacy base64-encoded plaintext.
func (sm *SecretManager) setFileSecret(name, value string) error {
	secrets, err := sm.loadSecretFile()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if secrets == nil {
		secrets = make(map[string]string)
	}

	pass := os.Getenv("AUTOMERGENT_SECRET_PASSPHRASE")
	if pass == "" {
		// Legacy behavior: base64-encode the plaintext (not secure)
		secrets[name] = base64.StdEncoding.EncodeToString([]byte(value))
	} else {
		enc, err := encrypt([]byte(value), pass)
		if err != nil {
			return fmt.Errorf("encrypt secret: %w", err)
		}
		// Mark as v2 so we can detect and decrypt later
		secrets[name] = "v2:" + enc
	}

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

	// New encrypted format: "v2:<base64(salt||nonce||ciphertext)>"
	if strings.HasPrefix(encoded, "v2:") {
		pass := os.Getenv("AUTOMERGENT_SECRET_PASSPHRASE")
		if pass == "" {
			return "", fmt.Errorf("secret %q is encrypted but AUTOMERGENT_SECRET_PASSPHRASE is not set", name)
		}
		raw := strings.TrimPrefix(encoded, "v2:")
		dec, err := decrypt(raw, pass)
		if err != nil {
			return "", fmt.Errorf("decrypt secret: %w", err)
		}
		return string(dec), nil
	}

	// Legacy base64-encoded plaintext
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

// loadSecretFile loads the secret file with file size validation to avoid DoS.
func (sm *SecretManager) loadSecretFile() (map[string]string, error) {
	st, err := os.Stat(sm.FilePath)
	if err != nil {
		return nil, err
	}
	if st.Size() > maxSecretFileSize {
		return nil, fmt.Errorf("secrets file too large: %d bytes", st.Size())
	}

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

// pbkdf2Key derives a key of desired length using PBKDF2-HMAC-SHA256.
func pbkdf2Key(password, salt []byte, iter, keyLen int) []byte {
	// PBKDF2 implementation (HMAC-SHA256)
	var hLen = sha256.Size
	nBlocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, nBlocks*hLen)
	for block := 1; block <= nBlocks; block++ {
		// U_1 = PRF(password, salt || INT(block))
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iter; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := 0; j < len(t); j++ {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

// encrypt encrypts plaintext with a key derived from passphrase using PBKDF2.
// Returns base64(salt||nonce||ciphertext).
func encrypt(plaintext []byte, passphrase string) (string, error) {
	// Generate salt
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := pbkdf2Key([]byte(passphrase), salt, 100000, 32)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ct := gcm.Seal(nil, nonce, plaintext, nil)
	buf := make([]byte, 0, len(salt)+len(nonce)+len(ct))
	buf = append(buf, salt...)
	buf = append(buf, nonce...)
	buf = append(buf, ct...)
	return base64.StdEncoding.EncodeToString(buf), nil
}

// decrypt reverses encrypt() using the provided passphrase.
func decrypt(dataB64 string, passphrase string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	if len(data) < 16 { // salt
		return nil, fmt.Errorf("ciphertext too short")
	}
	salt := data[:16]
	rest := data[16:]

	key := pbkdf2Key([]byte(passphrase), salt, 100000, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(rest) < nonceSize {
		return nil, fmt.Errorf("ciphertext missing nonce")
	}
	nonce := rest[:nonceSize]
	ct := rest[nonceSize:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("gcm open: %w", err)
	}
	return pt, nil
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

	// Check environment variables (provider-specific names from the catalog)
	for _, envKey := range ProviderEnvKeys(providerName) {
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

// ProviderAPIKeySource reports where a provider's API key resolves from,
// without revealing the key itself: "config", "config (secret ref)",
// "env NAME", "secret store", or "" when no key is set anywhere.
// It mirrors the resolution order of GetProviderAPIKey.
func ProviderAPIKeySource(cfg *Config, providerName string) string {
	if pc, ok := cfg.Providers[providerName]; ok && pc.APIKey != "" {
		if strings.HasPrefix(pc.APIKey, "${") {
			return "config (secret ref)"
		}
		return "config"
	}

	for _, envKey := range ProviderEnvKeys(providerName) {
		if os.Getenv(envKey) != "" {
			return "env " + envKey
		}
	}

	sm, err := NewSecretManager(BackendFile)
	if err == nil {
		if value, err := sm.Get(providerName + "_api_key"); err == nil && value != "" {
			return "secret store"
		}
	}
	return ""
}
