package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/hunknownz/temujin/internal/raid"
)

// FileStore is a JSON file-based task store (like Edict v1).
type FileStore struct {
	mu   sync.Mutex
	path string
}

func NewFileStore(dataDir string) *FileStore {
	os.MkdirAll(dataDir, 0755)
	path := filepath.Join(dataDir, "tasks.json")
	// Create file if not exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.WriteFile(path, []byte("[]"), 0644)
	}
	return &FileStore{path: path}
}

func (s *FileStore) Load() ([]raid.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnsafe()
}

func (s *FileStore) loadUnsafe() ([]raid.Task, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var tasks []raid.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *FileStore) saveUnsafe(tasks []raid.Task) error {
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Update atomically modifies tasks via a callback.
func (s *FileStore) Update(fn func([]raid.Task) []raid.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.loadUnsafe()
	if err != nil {
		tasks = []raid.Task{}
	}
	tasks = fn(tasks)
	return s.saveUnsafe(tasks)
}

func (s *FileStore) FindTask(id string) (*raid.Task, error) {
	tasks, err := s.Load()
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		if tasks[i].ID == id {
			return &tasks[i], nil
		}
	}
	return nil, nil
}
