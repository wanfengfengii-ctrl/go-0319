package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

// errorResponse is the stable error envelope {code, message, details}.
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// writeJSON writes a JSON response with a status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError maps an error to the stable error envelope and an appropriate
// HTTP status. Domain errors carry their code directly; other errors map to an
// internal error.
func writeError(w http.ResponseWriter, err error) {
	if de, ok := err.(*domain.DomainError); ok {
		status := statusForCode(de.Code)
		writeJSON(w, status, errorResponse{Code: string(de.Code), Message: de.Message, Details: de.Details})
		return
	}
	if errors.Is(err, persistence.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Code: string(domain.CodeTaskNotFound), Message: "task not found"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, errorResponse{Code: "INTERNAL", Message: err.Error()})
}

// statusForCode maps a stable error code to an HTTP status.
func statusForCode(code domain.ErrorCode) int {
	switch code {
	case domain.CodeTaskNotFound:
		return http.StatusNotFound
	case domain.CodeConfigStale, domain.CodeIdempotencyConflict, domain.CodeDuplicateIdentity,
		domain.CodeResourceBusy, domain.CodeLeaseExpired, domain.CodeStageOutOfOrder,
		domain.CodeSequenceDuplicate, domain.CodeSequenceConflict, domain.CodeSequenceWrap,
		domain.CodeEpochMismatch, domain.CodeTerminalAlreadyDecided, domain.CodeReviewNotReady,
		domain.CodeTerminalNotReady:
		return http.StatusConflict
	case domain.CodeInvalidRequest:
		return http.StatusBadRequest
	case domain.CodeProfileGap, domain.CodeProfileOverlap, domain.CodeSlotCollision,
		domain.CodeGraphDisconnected, domain.CodeGeometryDegenerate, domain.CodeArithmeticOverflow,
		domain.CodeInsufficientData:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusBadRequest
	}
}
