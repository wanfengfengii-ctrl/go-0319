// Command acoustic-array-deployment-gate is the entry point for the deep-sea
// lander acoustic transponder array calibration and deployment-gate service.
// It wires the SQLite store, the calibration and arbitration services, the
// injected logical clock and scripted device adapter, and serves the JSON HTTP
// API.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/arbitration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/calibration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/httpapi"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

// systemClock is the process-local default logical clock. The calibration
// domain only consumes domain.Clock; production may replace this with a
// disciplined reference clock.
type systemClock struct{}

func (systemClock) Now() domain.LogicalTime {
	return domain.LogicalTime(time.Now().UnixMicro())
}

// scriptedAdapter is a deterministic device adapter used for development and
// tests. It reports a healthy reference clock (zero drift) and valid responses
// for every other device; tests substitute failure-injecting adapters.
type scriptedAdapter struct{}

func (scriptedAdapter) Call(kind domain.DeviceKind, attempt int, now domain.LogicalTime) domain.DeviceResult {
	return domain.DeviceResult{Kind: kind, Attempt: attempt, Valid: true}
}

func main() {
	addr := flag.String("addr", envOr("BENZHI_ADDR", ":8080"), "listen address")
	dbPath := flag.String("db", envOr("BENZHI_DB", "benzhi.db"), "SQLite database path")
	flag.Parse()

	var _ domain.Clock = systemClock{}
	var _ domain.DeviceAdapter = scriptedAdapter{}

	store, err := persistence.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if repaired, err := store.Recover(); err != nil {
		log.Fatalf("recover: %v", err)
	} else if repaired {
		log.Printf("store snapshot digest repaired on startup")
	}

	cal := calibration.New(store, systemClock{}, scriptedAdapter{})
	arb := arbitration.New(store, systemClock{})
	api := httpapi.New(cal, arb, store)

	log.Printf("listening on %s (db=%s)", *addr, *dbPath)
	if err := http.ListenAndServe(*addr, api.Handler()); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
