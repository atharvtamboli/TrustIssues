package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Account struct {
	ID, Role, Org string
	PasswordHash  []byte
}
type Submission struct {
	Salt      string `json:"salt,omitempty"`
	Source    string `json:"source,omitempty"`
	Envelope  []byte `json:"envelope,omitempty"`
	PendingTx string `json:"pending_tx,omitempty"`
	RevokeTx  string `json:"revoke_tx,omitempty"`
	PendingFile
	ID         string `json:"id"`
	HolderID   string `json:"holder_id"`
	IssuerOrg  string `json:"issuer_org"`
	Status     string `json:"status"`
	ApprovedBy string `json:"approved_by,omitempty"`
}
type State struct {
	Careers       map[string]CareerProfile
	Applications  map[string]map[string]Application
	Version       int
	Accounts      map[string]Account
	Submissions   map[string]Submission
	Order         []string
	Chain         []Block
	Jobs          []Job
	LegacyPending []PendingFile
}
type Store struct {
	mu    sync.RWMutex
	state *State
	dir   string
	key   []byte
	lock  *os.File
}

func randomID() string { var b [32]byte; rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func utcNow() string   { return time.Now().UTC().Format(time.RFC3339Nano) }
func openStore(dir, legacy string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := lockDirectory(filepath.Join(dir, "writer.lock"))
	if err != nil {
		return nil, fmt.Errorf("cannot lock data directory (stop the running server before provisioning accounts): %w", err)
	}
	s := &Store{dir: dir, lock: f}
	if err = s.load(legacy); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) load(legacy string) error {
	path := filepath.Join(s.dir, "state.enc")
	data, err := os.ReadFile(path)
	fresh := errors.Is(err, os.ErrNotExist)
	if err != nil && !fresh {
		return err
	}
	keyPath := filepath.Join(s.dir, "state.key")
	key, err := os.ReadFile(keyPath)
	if fresh && err == nil {
		return errors.New("state.enc missing from initialized data directory; restore the snapshot from backup")
	}
	newKey := fresh && errors.Is(err, os.ErrNotExist)
	if newKey {
		key = make([]byte, 32)
		rand.Read(key)
		err = nil
	}
	if err != nil {
		return err
	}
	s.key = key
	st := &State{Version: 1, Accounts: map[string]Account{}, Submissions: map[string]Submission{}}
	if fresh {
		if err = st.importLegacy(legacy); err != nil {
			return err
		}
	} else {
		plain, e := unseal(data, key)
		if e != nil {
			return e
		}
		if err = json.Unmarshal(plain, st); err != nil {
			return err
		}
		if st.Version != 1 || st.Accounts == nil || st.Submissions == nil {
			return errors.New("invalid store schema")
		}
	}
	for i := range st.Jobs {
		if st.Jobs[i].ID == "" {
			st.Jobs[i].ID = fmt.Sprintf("legacy-%d", i+1)
		}
	}
	if st.Jobs == nil {
		st.Jobs = []Job{}
	}
	if err = validateChain(st.Chain); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(s.dir, "objects"), 0700); err != nil {
		return err
	}
	if fresh {
		if err = atomicWrite(keyPath, key); err != nil {
			return err
		}
		if err = s.persist(st); err != nil {
			return err
		}
	}
	s.state = st
	return nil
}
func gcmFor(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func unseal(data, key []byte) ([]byte, error) {
	g, err := gcmFor(key)
	if err != nil {
		return nil, err
	}
	if len(data) < g.NonceSize()+g.Overhead() {
		return nil, errors.New("truncated encrypted state")
	}
	return g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], nil)
}
func (st *State) importLegacy(dir string) error {
	b, err := os.ReadFile(filepath.Join(dir, "blockchain.json"))
	if err == nil { // Historical format only. All Phase 1 writes use the installation key.
		plain, e := unseal(b, []byte("12345678901234567890123456789012"))
		if e != nil {
			return e
		}
		if e = json.Unmarshal(plain, &st.Chain); e != nil {
			return e
		}
	} else if errors.Is(err, os.ErrNotExist) {
		genesis := Block{Timestamp: utcNow(), FileName: "Genesis Block", FileHash: "0", PrevHash: "0", Status: "verified", Skills: []string{}}
		genesis.Hash = calculateHash(genesis)
		st.Chain = []Block{genesis}
	} else {
		return err
	}
	for name, dst := range map[string]any{"jobs.json": &st.Jobs, "pending.json": &st.LegacyPending} {
		data, e := os.ReadFile(filepath.Join(dir, name))
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return e
		}
		if e = json.Unmarshal(data, dst); e != nil {
			return fmt.Errorf("%s: %w", name, e)
		}
	}
	return nil
}
func validateChain(chain []Block) error {
	if len(chain) == 0 {
		return errors.New("empty ledger")
	}
	prev := "0"
	for i, b := range chain {
		if b.Index != i || b.PrevHash != prev || b.Hash != calculateHash(b) {
			return fmt.Errorf("invalid ledger block %d", i)
		}
		prev = b.Hash
	}
	return nil
}
func atomicWrite(path string, bytes []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".commit-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(bytes); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return replaceFile(f.Name(), path)
}
func (s *Store) persist(st *State) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	g, err := gcmFor(s.key)
	if err != nil {
		return err
	}
	nonce := make([]byte, g.NonceSize())
	rand.Read(nonce)
	return atomicWrite(filepath.Join(s.dir, "state.enc"), g.Seal(nonce, nonce, data, nil))
}

// Published snapshots are immutable. Failed transactions never publish their candidate.
func (s *Store) snapshot() *State { s.mu.RLock(); defer s.mu.RUnlock(); return s.state }
func (s *Store) update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	var next State
	if err = json.Unmarshal(data, &next); err != nil {
		return err
	}
	if err = fn(&next); err != nil {
		return err
	}
	if err = s.persist(&next); err != nil {
		return err
	}
	s.state = &next
	return nil
}
