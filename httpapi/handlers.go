package httpapi

import (
	"net/http"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// handleCreateTask handles POST /v1/tasks.
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if !s.decode(w, r, &req) {
		return
	}
	key := domain.TaskKey{VoyageID: req.VoyageID, LanderID: req.LanderID, Generation: req.Generation}
	task, err := s.cal.CreateTask(key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, taskResponseFrom(task))
}

// handleFreeze handles POST /v1/tasks/{id}/freeze.
func (s *Server) handleFreeze(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	var req freezeRequest
	if !s.decode(w, r, &req) {
		return
	}
	digest, err := s.cal.Freeze(key, req.toConfig(), req.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"frozen_digest": digest})
}

// handleAcquire handles POST /v1/tasks/{id}/bindings:acquire.
func (s *Server) handleAcquire(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	var req bindRequest
	if !s.decode(w, r, &req) {
		return
	}
	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		writeError(w, domain.NewError(domain.CodeInvalidRequest, "missing Idempotency-Key header"))
		return
	}
	res, err := s.cal.AcquireBindings(key, idemKey, req.toAcquire())
	if err != nil {
		writeError(w, err)
		return
	}
	type leaseOut struct {
		ResourceType domain.ResourceType `json:"resource_type"`
		ResourceID   string              `json:"resource_id"`
		LeaseToken   string              `json:"lease_token"`
	}
	leases := make([]leaseOut, 0, len(res.Leases))
	for _, l := range res.Leases {
		leases = append(leases, leaseOut{ResourceType: l.ResourceType, ResourceID: l.ResourceID, LeaseToken: l.LeaseToken})
	}
	writeJSON(w, http.StatusOK, map[string]any{"bindings": res.Bindings, "leases": leases})
}

// handleRenew handles POST /leases/{token}:renew.
func (s *Server) handleRenew(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	var req renewalRequest
	if !s.decode(w, r, &req) {
		return
	}
	lease, err := s.cal.RenewLease(token, domain.LogicalTime(req.UntilUS))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lease)
}

// handleGetTask handles GET /v1/tasks/{id}.
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	key, ok := s.taskKey(w, r)
	if !ok {
		return
	}
	task, err := s.store.GetTask(key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, taskResponseFrom(task))
}

// taskKey extracts and parses the {id} path parameter.
func (s *Server) taskKey(w http.ResponseWriter, r *http.Request) (domain.TaskKey, bool) {
	key, err := parseTaskID(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return domain.TaskKey{}, false
	}
	return key, true
}
