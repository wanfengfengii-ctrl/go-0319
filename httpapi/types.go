package httpapi

import (
	"strconv"
	"strings"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/resources"
)

// createTaskRequest is the JSON body for POST /v1/tasks.
type createTaskRequest struct {
	VoyageID   string `json:"voyage_id"`
	LanderID   string `json:"lander_id"`
	Generation int    `json:"generation"`
}

// refPointJSON is a reference point in the freeze request.
type refPointJSON struct {
	ID string `json:"id"`
	X  int64  `json:"x"`
	Y  int64  `json:"y"`
	Z  int64  `json:"z"`
}

// transponderJSON is a transponder in the freeze request.
type transponderJSON struct {
	ID         string `json:"id"`
	Serial     string `json:"serial"`
	MountPoint string `json:"mount_point"`
	X          int64  `json:"x"`
	Y          int64  `json:"y"`
	Z          int64  `json:"z"`
}

// layerJSON is one sound speed layer.
type layerJSON struct {
	TopMM    int64 `json:"top_mm"`
	BottomMM int64 `json:"bottom_mm"`
	SpeedMMS int64 `json:"speed_mm_s"`
}

// slotJSON is one transmit slot.
type slotJSON struct {
	ID      string `json:"id"`
	StartUS int64  `json:"start_us"`
	EndUS   int64  `json:"end_us"`
}

// lineJSON is one planned calibration line.
type lineJSON struct {
	ID          string `json:"id"`
	Reference   string `json:"reference"`
	Transponder string `json:"transponder"`
}

// qualJSON is a reviewer qualification snapshot entry.
type qualJSON struct {
	ReviewerID string `json:"reviewer_id"`
	ValidUntil int64  `json:"valid_until"`
}

// freezeRequest is the JSON body for POST /v1/tasks/{id}/freeze.
type freezeRequest struct {
	Version              int64             `json:"version"`
	MountBases           []string          `json:"mount_bases"`
	ReferencePoints      []refPointJSON    `json:"reference_points"`
	Transponders         []transponderJSON `json:"transponders"`
	Profile              []layerJSON       `json:"profile"`
	Slots                []slotJSON        `json:"slots"`
	TransmitCodes        map[string]string `json:"transmit_codes"`
	ClockSource          string            `json:"clock_source"`
	Lines                []lineJSON        `json:"lines"`
	ReviewQualifications []qualJSON        `json:"review_qualifications"`
	TransducerDelayUS    int64             `json:"transducer_delay_us"`
	ResidualThresholdMM  int64             `json:"residual_threshold_mm"`
	DriftThresholdUS     int64             `json:"drift_threshold_us"`
	CounterModulus       int64             `json:"counter_modulus"`
	SequenceMax          uint64            `json:"sequence_max"`
	RetryMax             int               `json:"retry_max"`
}

func (r freezeRequest) toConfig() catalog.FrozenConfiguration {
	cfg := catalog.FrozenConfiguration{
		Version:             r.Version,
		MountBases:          r.MountBases,
		TransmitCodes:       r.TransmitCodes,
		ClockSource:         r.ClockSource,
		TransducerDelayUS:   r.TransducerDelayUS,
		ResidualThresholdMM: r.ResidualThresholdMM,
		DriftThresholdUS:    r.DriftThresholdUS,
		CounterModulus:      r.CounterModulus,
		SequenceMax:         r.SequenceMax,
		RetryMax:            r.RetryMax,
	}
	for _, p := range r.ReferencePoints {
		cfg.ReferencePoints = append(cfg.ReferencePoints, catalog.ReferencePoint{
			ID:    p.ID,
			Coord: catalog.Vec3{X: p.X, Y: p.Y, Z: p.Z},
		})
	}
	for _, t := range r.Transponders {
		cfg.Transponders = append(cfg.Transponders, catalog.TransponderSpec{
			ID:         t.ID,
			Serial:     t.Serial,
			MountPoint: t.MountPoint,
			Coord:      catalog.Vec3{X: t.X, Y: t.Y, Z: t.Z},
		})
	}
	for _, l := range r.Profile {
		cfg.Profile.Layers = append(cfg.Profile.Layers, catalog.SoundSpeedLayer{
			TopMM:    l.TopMM,
			BottomMM: l.BottomMM,
			SpeedMMS: l.SpeedMMS,
		})
	}
	for _, s := range r.Slots {
		cfg.Slots = append(cfg.Slots, catalog.Slot{ID: s.ID, StartUS: s.StartUS, EndUS: s.EndUS})
	}
	for _, l := range r.Lines {
		cfg.Lines = append(cfg.Lines, catalog.Line{ID: l.ID, Reference: l.Reference, Transponder: l.Transponder})
	}
	for _, q := range r.ReviewQualifications {
		cfg.ReviewQualifications = append(cfg.ReviewQualifications, domain.ReviewQualification{
			ReviewerID: q.ReviewerID,
			ValidUntil: domain.LogicalTime(q.ValidUntil),
		})
	}
	return cfg
}

// bindRequest is the JSON body for POST /v1/tasks/{id}/bindings:acquire.
type bindRequest struct {
	Bindings []struct {
		Serial     string `json:"serial"`
		MountPoint string `json:"mount_point"`
	} `json:"bindings"`
	Leases []struct {
		ResourceType domain.ResourceType `json:"resource_type"`
		ResourceID   string              `json:"resource_id"`
		Duration     int64               `json:"duration_us"`
	} `json:"leases"`
}

func (r bindRequest) toAcquire() resources.AcquireRequest {
	req := resources.AcquireRequest{}
	for _, b := range r.Bindings {
		req.Bindings = append(req.Bindings, resources.BindingRequest{Serial: b.Serial, MountPoint: b.MountPoint})
	}
	for _, l := range r.Leases {
		req.Leases = append(req.Leases, resources.LeaseRequest{
			ResourceType: l.ResourceType,
			ResourceID:   l.ResourceID,
			Duration:     domain.LogicalTime(l.Duration),
		})
	}
	return req
}

// renewalRequest is the JSON body for POST /leases/{token}:renew.
type renewalRequest struct {
	UntilUS int64 `json:"until_us"`
}

// transmissionRequest is the JSON body for POST /v1/tasks/{id}/transmissions.
type transmissionRequest struct {
	Transponder string `json:"transponder"`
	Line        string `json:"line"`
	TransmitUS  int64  `json:"transmit_us"`
}

// echoRequest is the JSON body for POST /v1/tasks/{id}/echoes.
type echoRequest struct {
	Epoch       int64  `json:"epoch"`
	Transponder string `json:"transponder"`
	Sequence    uint64 `json:"sequence"`
	Line        string `json:"line"`
	TransmitUS  int64  `json:"transmit_us"`
	ReceiveUS   int64  `json:"receive_us"`
}

// reviewRequest is the JSON body for POST /v1/tasks/{id}/reviews.
type reviewRequest struct {
	ReviewerID string `json:"reviewer_id"`
}

// terminalRequest is the JSON body for POST /v1/tasks/{id}/terminal-decisions.
type terminalRequest struct {
	State domain.TerminalState `json:"state"`
}

// taskResponse is the JSON response for a task query.
type taskResponse struct {
	VoyageID      string               `json:"voyage_id"`
	LanderID      string               `json:"lander_id"`
	Generation    int                  `json:"generation"`
	Phase         string               `json:"phase"`
	ConfigVersion int64                `json:"config_version"`
	FrozenDigest  string               `json:"frozen_digest"`
	TerminalState domain.TerminalState `json:"terminal_state"`
	CurrentEpoch  int64                `json:"current_epoch"`
	CreatedAt     int64                `json:"created_at"`
}

func taskResponseFrom(t domain.MissionTask) taskResponse {
	return taskResponse{
		VoyageID:      t.Key.VoyageID,
		LanderID:      t.Key.LanderID,
		Generation:    t.Key.Generation,
		Phase:         t.Phase.String(),
		ConfigVersion: t.ConfigVersion,
		FrozenDigest:  t.FrozenDigest,
		TerminalState: t.TerminalState,
		CurrentEpoch:  t.CurrentEpoch,
		CreatedAt:     int64(t.CreatedLogicalTime),
	}
}

// parseTaskID splits a "{voyage}:{lander}:{generation}" path id.
func parseTaskID(id string) (domain.TaskKey, error) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return domain.TaskKey{}, domain.NewError(domain.CodeInvalidRequest, "task id must be voyage:lander:generation")
	}
	gen, err := strconv.Atoi(parts[2])
	if err != nil {
		return domain.TaskKey{}, domain.NewError(domain.CodeInvalidRequest, "invalid generation")
	}
	return domain.TaskKey{VoyageID: parts[0], LanderID: parts[1], Generation: gen}, nil
}
