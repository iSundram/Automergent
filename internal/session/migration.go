package session

import (
	"encoding/json"
	"fmt"
)

const currentSessionVersion = 1

// MigrateSession checks the session version and applies any necessary migrations.
func MigrateSession(sess *Session) (*Session, error) {
	if sess == nil {
		return nil, fmt.Errorf("cannot migrate nil session")
	}

	// Future version migrations can be chained here:
	// if sess.Version < 2 { ... sess.Version = 2 }

	if sess.Version == 0 {
		sess.Version = currentSessionVersion
	}

	return sess, nil
}

// MigratePersistenceState checks and upgrades persistence state if needed.
func MigratePersistenceState(state *PersistenceState) (*PersistenceState, error) {
	if state == nil {
		return nil, fmt.Errorf("cannot migrate nil persistence state")
	}
	if state.Version == 0 {
		state.Version = 1
	}
	return state, nil
}

// Re-serialize with migration applied
func migrateJSON(data []byte) ([]byte, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	// Apply structural transformations if needed for older schemas
	if _, ok := raw["version"]; !ok {
		raw["version"] = currentSessionVersion
	}
	return json.MarshalIndent(raw, "", "  ")
}
