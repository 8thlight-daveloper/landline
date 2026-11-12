package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Store struct {
	mu   sync.Mutex
	file *os.File
	log  []model.Envelope
	at   map[int64]int
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
	s := &Store{file: file, at: map[int64]int{}}
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		var e model.Envelope
		if len(line) > 0 && json.Unmarshal(line, &e) == nil && e.Seq > 0 {
			if e.ID != "" {
				s.add(e)
			} else if e.Result != "" {
				s.merge(e.Seq, e.Result)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("read the log: %w", err)
		}
	}
	if info, err := file.Stat(); err == nil && info.Size() > 0 {
		last := make([]byte, 1)
		if _, err := file.ReadAt(last, info.Size()-1); err == nil && last[0] != '\n' {
			if _, err := file.Write([]byte("\n")); err != nil {
				file.Close()
				return nil, fmt.Errorf("end the log's last line: %w", err)
			}
		}
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
	s.add(e)
	return nil
}

type resultLine struct {
	Seq    int64  `json:"seq"`
	Result string `json:"result"`
}

func (s *Store) Result(seq int64, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, known := s.at[seq]; !known {
		return fmt.Errorf("the log has no message %d", seq)
	}
	line, err := json.Marshal(resultLine{Seq: seq, Result: result})
	if err != nil {
		return err
	}
	if _, err := s.file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write the log: %w", err)
	}
	s.merge(seq, result)
	return nil
}

func (s *Store) add(e model.Envelope) {
	s.at[e.Seq] = len(s.log)
	s.log = append(s.log, e)
}

func (s *Store) merge(seq int64, result string) {
	if index, known := s.at[seq]; known {
		s.log[index].Result = result
	}
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
