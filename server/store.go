package server

import (
	"encoding/gob"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

var ErrServerFileNotExist = errors.New("server file does not exists in path")

type storer struct {
	mu       *sync.RWMutex
	filePath string
}

func NewStorer(filePath string) *storer {
	return &storer{mu: &sync.RWMutex{}, filePath: filePath}
}

func (s *storer) Store(data *Server) error {
	path, err := filepath.Abs(s.filePath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	defer f.Close()
	if err != nil {
		return err
	}
	return gob.NewEncoder(f).Encode(data)
}

func (s *storer) Restore() (*Server, error) {
	path, err := filepath.Abs(s.filePath)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, ErrServerFileNotExist
	}

	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	server := &Server{}
	if err = gob.NewDecoder(f).Decode(server); err != nil {
		return nil, err
	}
	return server, nil
}
