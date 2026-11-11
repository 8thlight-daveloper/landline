package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type snapshot struct {
	Agents  []model.Agent     `json:"agents"`
	Tokens  map[string]string `json:"tokens"`
	Cursors map[string]int64  `json:"cursors"`
}

func (s *Server) loadState() {
	if s.cfg.StatePath == "" {
		return
	}
	raw, err := os.ReadFile(s.cfg.StatePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			s.cfg.Notify("the saved state could not be read, so every agent must register again: " + err.Error())
		}
		return
	}
	var snap snapshot
	if json.Unmarshal(raw, &snap) != nil {
		s.cfg.Notify("the saved state could not be read, so every agent must register again")
		return
	}
	s.agents = snap.Agents
	if snap.Tokens != nil {
		s.tokens = snap.Tokens
	}
	if snap.Cursors != nil {
		s.cursors = snap.Cursors
	}
}

func (s *Server) saveLocked() {
	if s.cfg.StatePath == "" {
		return
	}
	raw, err := json.Marshal(snapshot{Agents: s.agents, Tokens: s.tokens, Cursors: s.cursors})
	if err == nil {
		err = os.MkdirAll(filepath.Dir(s.cfg.StatePath), 0o700)
	}
	if err == nil {
		temp := s.cfg.StatePath + ".tmp"
		if err = os.WriteFile(temp, raw, 0o600); err == nil {
			if err = os.Chmod(temp, 0o600); err == nil {
				err = os.Rename(temp, s.cfg.StatePath)
			}
		}
	}
	if err != nil {
		s.cfg.Notify("could not save the state: " + err.Error())
	}
}
