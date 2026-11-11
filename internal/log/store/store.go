package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Store struct {
	mu   sync.Mutex
	file *os.File
	log  []model.Envelope
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create the log folder: %w", err)
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open the log: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("make the log private: %w", err)
	}
	s := &Store{file: file}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*model.MaxBodyBytes)
	for scanner.Scan() {
		var e model.Envelope
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			continue
		}
		s.log = append(s.log, e)
	}
	if err := scanner.Err(); err != nil {
		file.Close()
		return nil, fmt.Errorf("read the log: %w", err)
	}
	return s, nil
}

func (s *Store) Append(e model.Envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := s.file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write the log: %w", err)
	}
	s.log = append(s.log, e)
	return nil
}

func (s *Store) All() []model.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.Envelope(nil), s.log...)
}

func (s *Store) LastSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.log) == 0 {
		return 0
	}
	return s.log[len(s.log)-1].Seq
}

func (s *Store) Close() error { return s.file.Close() }
