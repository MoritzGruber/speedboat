package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/automerge/automerge-go"
)

// FileStore manages automerge documents locally on disk.
type FileStore struct {
	dir string
	m   sync.RWMutex
}

// NewFileStore initializes the storage directory.
func NewFileStore(storageDir string) (*FileStore, error) {
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	return &FileStore{dir: storageDir}, nil
}

// path constructs the file path for a given issue ID.
func (s *FileStore) path(id string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s.am", id))
}

// Save persists an Automerge document to the file system.
func (s *FileStore) Save(id string, doc *automerge.Doc) error {
	s.m.Lock()
	defer s.m.Unlock()

	data := doc.Save()
	return os.WriteFile(s.path(id), data, 0644)
}

// Load retrieves an Automerge document from the file system.
func (s *FileStore) Load(id string) (*automerge.Doc, error) {
	s.m.RLock()
	defer s.m.RUnlock()

	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, fmt.Errorf("failed to read file for id %s: %w", id, err)
	}

	doc, err := automerge.Load(data)
	if err != nil {
		return nil, fmt.Errorf("failed to load automerge doc %s: %w", id, err)
	}
	return doc, nil
}

// ListAll loads all saved Automerge documents from the directory.
func (s *FileStore) ListAll() ([]*automerge.Doc, error) {
	s.m.RLock()
	defer s.m.RUnlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var docs []*automerge.Doc
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".am") {
			id := strings.TrimSuffix(entry.Name(), ".am")
			doc, err := s.Load(id)
			if err == nil {
				docs = append(docs, doc)
			}
		}
	}
	return docs, nil
}

// Delete removes a document from the file system.
func (s *FileStore) Delete(id string) error {
	s.m.Lock()
	defer s.m.Unlock()

	err := os.Remove(s.path(id))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
