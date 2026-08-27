package httpapi

import "net/http"

// handleReview handles POST /v1/tasks/{id}/reviews.
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	var req reviewRequest
	if !s.decode(w, r, &req) {
		return
	}
	review, err := s.arb.AddReview(key, req.ReviewerID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, review)
}

// handleTerminal handles POST /v1/tasks/{id}/terminal-decisions.
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	var req terminalRequest
	if !s.decode(w, r, &req) {
		return
	}
	decision, err := s.arb.DecideTerminal(key, req.State)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}

// handleCredential handles GET /v1/tasks/{id}/credential.
func (s *Server) handleCredential(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	cred, err := s.store.GetCredential(key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"credential_digest": cred.Digest,
		"issued_at":         int64(cred.IssuedAt),
		"voyage_id":         cred.Key.VoyageID,
		"lander_id":         cred.Key.LanderID,
		"generation":        cred.Key.Generation,
	})
}
