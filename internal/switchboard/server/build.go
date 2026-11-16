package server

import "net/http"

type BuildResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func (s *Server) build(w http.ResponseWriter, r *http.Request) {
	_, who := s.callerFor(r)
	if who == agentCaller {
		fail(w, http.StatusForbidden, "an agent token may not read the build")
		return
	}
	if who == unknownCaller && !s.isSecret(r) {
		fail(w, http.StatusUnauthorized, "missing or wrong switchboard secret")
		return
	}
	write(w, http.StatusOK, s.cfg.Build)
}
