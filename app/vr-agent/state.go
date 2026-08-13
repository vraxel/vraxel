package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"vraxel.io/vraxel/lib/agent/client"
)

// state is what the agent persists across restarts: the durable
// credential and the identity the server assigned it.
//
// Nothing else belongs here. Job progress deliberately is not persisted
// (an interrupted job is failed, not resumed), so a lost state file costs
// exactly one re-registration.
type state struct {
	ServerURL  string `json:"serverUrl"`
	AgentID    string `json:"agentId"`
	HostID     int64  `json:"hostId"`
	AgentToken string `json:"agentToken"`
}

// stateFileMode is 0600: the file holds a durable credential that opens
// this host's control channel.
const stateFileMode = 0o600

// loadState reads the persisted state. Returns nil when the file does
// not exist, which is the "not yet registered" signal.
func loadState(path string) (*state, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state %s: %w", path, err)
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", path, err)
	}
	if s.AgentToken == "" {
		return nil, fmt.Errorf("state %s has no agentToken", path)
	}
	return &s, nil
}

// saveState writes the state atomically at 0600.
//
// Atomic because a crash between truncate and write would leave the
// agent with no credential and no way to get one (the join token is
// single-use and already consumed) -- an unrecoverable state requiring
// manual re-onboarding.
func saveState(path string, s *state, log client.Logger) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := path + ".tmp"
	if err := writeSynced(tmp, data); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("install state: %w", err)
	}
	// Fsync the directory too: the rename is only durable once the
	// directory entry itself is on disk. Without both syncs "atomic" holds
	// against a crash but not against power loss, which is the case that
	// costs the credential outright -- the join token that produced it is
	// already spent.
	//
	// A failure here is reported, not returned: the state file is written
	// and renamed at this point, so the registration DID succeed. Failing
	// it would send the operator back to a join token that has already
	// been consumed, trading a lost power-loss guarantee for a certain,
	// unrecoverable failure.
	if err := syncDir(filepath.Dir(path)); err != nil {
		log.Warnf("vr-agent: %v (the state file is written; only the power-loss guarantee is weaker)", err)
	}
	return nil
}

// writeSynced writes data and flushes it to stable storage before the
// caller renames it into place.
func writeSynced(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, stateFileMode)
	if err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write state: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync state: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	return nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open state dir: %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync state dir: %w", err)
	}
	return nil
}
