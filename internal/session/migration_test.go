package session

import (
	"testing"
)

func TestMigrateSession(t *testing.T) {
	sess := &Session{ID: "test-id"}
	migrated, err := MigrateSession(sess)
	if err != nil {
		t.Fatalf("MigrateSession: %v", err)
	}
	if migrated.Version != currentSessionVersion {
		t.Errorf("version = %d, want %d", migrated.Version, currentSessionVersion)
	}
}

func TestMigratePersistenceState(t *testing.T) {
	state := &PersistenceState{}
	migrated, err := MigratePersistenceState(state)
	if err != nil {
		t.Fatalf("MigratePersistenceState: %v", err)
	}
	if migrated.Version != 1 {
		t.Errorf("version = %d, want 1", migrated.Version)
	}
}
