package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"aperture/pkg/ips"
)

// Store persists statements. One file per IPS so a restart reads back
// exactly what was accepted; the hash is recomputed, never stored.
// Bound is the statement the live book is under; GET /ips with no id
// returns it so every client paints the same line.
type Store interface {
	Get(id string) (ips.IPS, error)
	Put(p ips.IPS) error
	List() ([]ips.IPS, error)
	Bound() (ips.IPS, error)
	SetBound(id string) error
}

var ErrNotFound = errors.New("ips: not found")

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// boundFile is not a .json so List ignores it.
const boundFile = "BOUND"

// FileStore keeps data/ips/<id>.json. data/ is gitignored except roster/.
type FileStore struct {
	Dir string
	mu  sync.Mutex
}

func NewFileStore(dir string) *FileStore {
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join("data", "ips")
	}
	return &FileStore{Dir: dir}
}

func (s *FileStore) path(id string) (string, error) {
	if !safeID.MatchString(id) {
		return "", fmt.Errorf("ips: id %q must match %s", id, safeID)
	}
	return filepath.Join(s.Dir, id+".json"), nil
}

func (s *FileStore) Get(id string) (ips.IPS, error) {
	p, err := s.path(id)
	if err != nil {
		return ips.IPS{}, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return ips.IPS{}, ErrNotFound
	}
	if err != nil {
		return ips.IPS{}, err
	}
	var out ips.IPS
	if err := json.Unmarshal(b, &out); err != nil {
		return ips.IPS{}, err
	}
	return out, nil
}

func (s *FileStore) Put(p ips.IPS) error {
	path, err := s.path(p.ID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *FileStore) List() ([]ips.IPS, error) {
	ents, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []ips.IPS
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p, err := s.Get(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *FileStore) Bound() (ips.IPS, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, boundFile))
	if errors.Is(err, os.ErrNotExist) {
		return ips.IPS{}, ErrNotFound
	}
	if err != nil {
		return ips.IPS{}, err
	}
	id := strings.TrimSpace(string(b))
	if id == "" {
		return ips.IPS{}, ErrNotFound
	}
	return s.Get(id)
}

func (s *FileStore) SetBound(id string) error {
	if _, err := s.path(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(s.Dir, boundFile+".tmp")
	if err := os.WriteFile(tmp, []byte(id+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.Dir, boundFile))
}

// MemStore is for tests and replays.
type MemStore struct {
	mu    sync.Mutex
	m     map[string]ips.IPS
	bound string
}

func NewMemStore() *MemStore { return &MemStore{m: map[string]ips.IPS{}} }

func (s *MemStore) Get(id string) (ips.IPS, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.m[id]
	if !ok {
		return ips.IPS{}, ErrNotFound
	}
	return p, nil
}

func (s *MemStore) Put(p ips.IPS) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[p.ID] = p
	return nil
}

func (s *MemStore) List() ([]ips.IPS, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ips.IPS, 0, len(s.m))
	for _, p := range s.m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *MemStore) Bound() (ips.IPS, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bound == "" {
		return ips.IPS{}, ErrNotFound
	}
	p, ok := s.m[s.bound]
	if !ok {
		return ips.IPS{}, ErrNotFound
	}
	return p, nil
}

func (s *MemStore) SetBound(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; !ok {
		return ErrNotFound
	}
	s.bound = id
	return nil
}
