package diagnostics

import "github.com/iSundram/Automergent/internal/config"

// loadConfig is a thin wrapper to keep the import in a single place inside the
// package.
func loadConfig() *config.Config {
	return config.Default()
}
