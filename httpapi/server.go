// Package httpapi exposes the calibration workflow as a JSON HTTP API. It
// wraps the calibration and arbitration services, injects the logical clock
// and scripted device adapter, and provides recovery health checks.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/arbitration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/calibration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/solver"
)

// Server is the HTTP API server.
type Server struct {
	cal   *calibration.Service
	arb   *arbitration.Service
	store persistence.Store
}

// New constructs an HTTP API server.
func New(cal *calibration.Service, arb *arbitration.Service, store persistence.Store) *Server {
	return &Server{cal: cal, arb: arb, store: store}
}

// Handler builds the routed HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/tasks", s.handleCreateTask)
	mux.HandleFunc("POST /v1/tasks/{id}/freeze", s.handleFreeze)
	mux.HandleFunc("POST /v1/tasks/{id}/bindings:acquire", s.handleAcquire)
	mux.HandleFunc("POST /leases/{token}/renew", s.handleRenew)
	mux.HandleFunc("POST /v1/tasks/{id}/clock:discipline", s.handleDiscipline)
	mux.HandleFunc("POST /v1/tasks/{id}/loopback:confirm", s.handleLoopback)
	mux.HandleFunc("POST /v1/tasks/{id}/transmissions", s.handleTransmission)
	mux.HandleFunc("POST /v1/tasks/{id}/echoes", s.handleEcho)
	mux.HandleFunc("POST /v1/tasks/{id}/solve", s.handleSolve)
	mux.HandleFunc("POST /v1/tasks/{id}/recalibrations", s.handleRecalibration)
	mux.HandleFunc("POST /v1/tasks/{id}/reviews", s.handleReview)
	mux.HandleFunc("POST /v1/tasks/{id}/terminal-decisions", s.handleTerminal)
	mux.HandleFunc("GET /v1/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("GET /v1/tasks/{id}/evidence", s.handleEvidence)
	mux.HandleFunc("GET /v1/tasks/{id}/solution", s.handleSolution)
	mux.HandleFunc("GET /v1/tasks/{id}/credential", s.handleCredential)

	return mux
}

type healthResponse struct {
	Status           string `json:"status"`
	AlgorithmVersion int    `json:"algorithm_version"`
	PhaseCount       int    `json:"phase_count"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:           "ok",
		AlgorithmVersion: solver.AlgorithmVersion,
		PhaseCount:       int(domain.PhaseTerminal) + 1,
	})
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, domain.NewError(domain.CodeInvalidRequest, "malformed JSON body"))
		return false
	}
	return true
}
