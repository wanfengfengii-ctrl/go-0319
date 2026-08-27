package calibration

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

type scheduledTimeoutDevice struct {
	attempts []int
}

func (d *scheduledTimeoutDevice) Call(kind domain.DeviceKind, attempt int, now domain.LogicalTime) domain.DeviceResult {
	d.attempts = append(d.attempts, attempt)
	return domain.DeviceResult{
		Kind:      kind,
		Attempt:   attempt,
		Retry:     true,
		RetryTime: now + 50,
		Err:       domain.CodeDeviceTimeout,
	}
}

func TestModel_DisciplineClockHonorsRetrySchedule(t *testing.T) {
	clock := &fakeClock{now: 100}
	device := &scheduledTimeoutDevice{}
	store, err := persistence.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	service := New(store, clock, device)
	key := domain.TaskKey{VoyageID: "retry-voyage", LanderID: "retry-lander", Generation: 1}
	mustFreezeAndAcquire(t, service, key)

	if _, err := service.DisciplineClock(key); err == nil {
		t.Fatal("initial discipline unexpectedly succeeded")
	}

	tests := []struct {
		name         string
		now          domain.LogicalTime
		wantCalls    int
		wantRetries  int
		wantAttempt  int
		wantNextTime domain.LogicalTime
	}{
		{
			name:         "before scheduled logical time",
			now:          149,
			wantCalls:    1,
			wantRetries:  1,
			wantAttempt:  0,
			wantNextTime: 150,
		},
		{
			name:         "at scheduled logical time",
			now:          150,
			wantCalls:    2,
			wantRetries:  2,
			wantAttempt:  1,
			wantNextTime: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock.now = tt.now
			_, err := service.DisciplineClock(key)
			var domainErr *domain.DomainError
			if !errors.As(err, &domainErr) || domainErr.Code != domain.CodeDeviceTimeout {
				t.Fatalf("DisciplineClock error = %v, want %s", err, domain.CodeDeviceTimeout)
			}
			if got := len(device.attempts); got != tt.wantCalls {
				t.Fatalf("device calls = %d, want %d (attempts %v)", got, tt.wantCalls, device.attempts)
			}

			retries, err := store.ListRetryCalls(key)
			if err != nil {
				t.Fatalf("ListRetryCalls: %v", err)
			}
			if got := len(retries); got != tt.wantRetries {
				t.Fatalf("retry calls = %d, want %d", got, tt.wantRetries)
			}
			latest := retries[len(retries)-1]
			if latest.Attempt != tt.wantAttempt || latest.NextTime != tt.wantNextTime {
				t.Fatalf("latest retry = {attempt:%d next:%d}, want {attempt:%d next:%d}", latest.Attempt, latest.NextTime, tt.wantAttempt, tt.wantNextTime)
			}

			task, err := store.GetTask(key)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if task.Phase != domain.PhaseBindingsAcquired {
				t.Fatalf("phase = %v, want %v", task.Phase, domain.PhaseBindingsAcquired)
			}
		})
	}
}
