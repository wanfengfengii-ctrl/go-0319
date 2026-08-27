package httpapi

import (
	"net/http"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/calibration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// handleDiscipline handles POST /v1/tasks/{id}/clock:discipline.
func (s *Server) handleDiscipline(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	epoch, err := s.cal.DisciplineClock(key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, epoch)
}

// handleLoopback handles POST /v1/tasks/{id}/loopback:confirm.
func (s *Server) handleLoopback(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	if err := s.cal.ConfirmLoopback(key); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "loopback_confirmed"})
}

// handleTransmission handles POST /v1/tasks/{id}/transmissions.
func (s *Server) handleTransmission(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	var req transmissionRequest
	if !s.decode(w, r, &req) {
		return
	}
	ev, err := s.cal.RecordTransmission(key, req.Transponder, req.Line, domain.LogicalTime(req.TransmitUS))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// handleEcho handles POST /v1/tasks/{id}/echoes.
func (s *Server) handleEcho(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	var req echoRequest
	if !s.decode(w, r, &req) {
		return
	}
	outcome, ev, err := s.cal.RecordEcho(key, req.Epoch, req.Transponder, req.Sequence, req.Line, domain.LogicalTime(req.TransmitUS), domain.LogicalTime(req.ReceiveUS))
	if err != nil {
		writeError(w, err)
		return
	}
	status := "accepted"
	if outcome == calibration.EchoLate {
		status = "late"
	} else if outcome == calibration.EchoDuplicate {
		status = "duplicate"
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "evidence": ev})
}

// handleSolve handles POST /v1/tasks/{id}/solve.
func (s *Server) handleSolve(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	res, err := s.cal.Solve(key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleRecalibration handles POST /v1/tasks/{id}/recalibrations.
func (s *Server) handleRecalibration(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "residual_exceeded"
	}
	batch, err := s.arb.BuildRecalibration(key, reason)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

// handleEvidence handles GET /v1/tasks/{id}/evidence.
func (s *Server) handleEvidence(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	ev, err := s.store.ListEvidence(key)
	if err != nil {
		writeError(w, err)
		return
	}
	if ev == nil {
		ev = []domain.TimestampEvidence{}
	}
	writeJSON(w, http.StatusOK, ev)
}

// handleSolution handles GET /v1/tasks/{id}/solution.
func (s *Server) handleSolution(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	sr, err := s.store.GetSolve(key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sr)
}
