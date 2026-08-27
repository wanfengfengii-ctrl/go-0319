package resources

import (
	"errors"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

func TestModel_LeaseRenewal(t *testing.T) {
	tests := []struct {
		name      string
		now       domain.LogicalTime
		token     func(string) string
		until     domain.LogicalTime
		wantCode  domain.ErrorCode
		wantEnd   domain.LogicalTime
		wantVer   int64
		probeLive bool
	}{
		{
			name:    "active lease extends beyond its current expiry",
			now:     100,
			token:   func(token string) string { return token },
			until:   175,
			wantEnd: 175,
			wantVer: 2,
		},
		{
			name:      "active lease cannot be shortened",
			now:       100,
			token:     func(token string) string { return token },
			until:     125,
			wantCode:  domain.CodeLeaseExpired,
			probeLive: true,
		},
		{
			name:      "expired lease cannot be renewed",
			now:       150,
			token:     func(token string) string { return token },
			until:     175,
			wantCode:  domain.CodeLeaseExpired,
			probeLive: false,
		},
		{
			name:      "unknown token cannot alter a live lease",
			now:       100,
			token:     func(string) string { return "unknown-token" },
			until:     175,
			wantCode:  domain.CodeLeaseExpired,
			probeLive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &fakeClock{now: 100}
			store := openStore(t)
			manager := NewManager(store, clock)
			key := domain.TaskKey{VoyageID: "voyage", LanderID: "lander", Generation: 1}
			if err := store.CreateTask(domain.MissionTask{Key: key, Phase: domain.PhaseConfigFrozen}); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			acquired, err := manager.Acquire(key, "renewal-model", AcquireRequest{Leases: []LeaseRequest{{
				ResourceType: domain.ResourceSink,
				ResourceID:   "deck-array",
				Duration:     50,
			}}})
			if err != nil {
				t.Fatalf("Acquire: %v", err)
			}
			original := acquired.Leases[0]
			clock.now = tt.now

			got, err := manager.Renew(tt.token(original.LeaseToken), tt.until)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("Renew(%d): %v", tt.until, err)
				}
				if got.LeaseToken != original.LeaseToken || got.ResourceType != original.ResourceType || got.ResourceID != original.ResourceID || got.Key != original.Key {
					t.Fatalf("renewed lease identity changed: got %+v, original %+v", got, original)
				}
				if got.StartTime != original.StartTime || got.EndTime != tt.wantEnd || got.Version != tt.wantVer {
					t.Fatalf("renewed lease = {start:%d end:%d version:%d}, want {start:%d end:%d version:%d}", got.StartTime, got.EndTime, got.Version, original.StartTime, tt.wantEnd, tt.wantVer)
				}
				return
			}

			var domainErr *domain.DomainError
			if !errors.As(err, &domainErr) || domainErr.Code != tt.wantCode {
				t.Errorf("Renew(%d) error = %v, want domain code %s", tt.until, err, tt.wantCode)
			}
			if tt.probeLive {
				clock.now = 100
				unchanged, probeErr := manager.Renew(original.LeaseToken, 175)
				if probeErr != nil {
					t.Fatalf("renew after rejected request: %v", probeErr)
				}
				if unchanged.EndTime != 175 || unchanged.Version != 2 {
					t.Fatalf("rejected request rewrote lease: subsequent renewal = {end:%d version:%d}, want {end:175 version:2}", unchanged.EndTime, unchanged.Version)
				}
			}
		})
	}
}
