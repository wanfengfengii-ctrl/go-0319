#!/usr/bin/env bash
# Smoke test for the deep-sea lander acoustic array calibration service.
#
# It builds the binary, starts the service on a local ephemeral port, drives a
# full deterministic calibration workflow through the public HTTP API, asserts
# the deployment credential is issued, and cleans up every process and
# temporary file it created. It performs no external network access and does
# not merely call `go test`.
set -euo pipefail

PORT="${BENZHI_PORT:-18080}"
BASE="http://127.0.0.1:${PORT}"

# Create a dedicated temp dir; cleanup removes only the files/dir we created.
TMPDIR="$(mktemp -d)"
BIN="${TMPDIR}/acoustic-array-deployment-gate"
DB="${TMPDIR}/benzhi.db"
PID=""

cleanup() {
  if [[ -n "${PID}" ]]; then
    kill "${PID}" 2>/dev/null || true
    wait "${PID}" 2>/dev/null || true
  fi
  rm -f "${DB}" "${DB}-wal" "${DB}-shm" "${BIN}"
  rmdir "${TMPDIR}" 2>/dev/null || true
}
trap cleanup EXIT

echo "building service binary"
go build -o "${BIN}" .

echo "starting service on port ${PORT}"
"${BIN}" -addr "127.0.0.1:${PORT}" -db "${DB}" &
PID=$!

# Wait for the health endpoint to come up (bounded, deterministic).
up=0
for _ in $(seq 1 100); do
  if status="$(curl -sS -o /dev/null -w '%{http_code}' "${BASE}/healthz" 2>/dev/null)"; then
    if [[ "${status}" == "200" ]]; then up=1; break; fi
  fi
  sleep 0.05
done
if [[ "${up}" != "1" ]]; then
  echo "service did not become healthy" >&2
  exit 1
fi

# request: curl a JSON endpoint and capture the status code and body separately.
request() {
  local method="$1" path="$2" body="${3:-}" idem="${4:-}"
  local args=(-sS -X "${method}" -o "${TMPDIR}/resp.body" -w '%{http_code}')
  [[ -n "${idem}" ]] && args+=(-H "Idempotency-Key: ${idem}")
  if [[ -n "${body}" ]]; then
    args+=(-H 'Content-Type: application/json' --data "${body}")
  fi
  status="$(curl "${args[@]}" "${BASE}${path}")"
  body_out="$(cat "${TMPDIR}/resp.body")"
}

expect_status() {
  local want="$1" got="$2" label="$3"
  if [[ "${got}" != "${want}" ]]; then
    echo "FAIL ${label}: expected HTTP ${want}, got ${got}; body=${body_out}" >&2
    exit 1
  fi
}

echo "probing health"
request GET /healthz
expect_status 200 "${status}" "healthz"

echo "creating task"
request POST /v1/tasks '{"voyage_id":"v","lander_id":"L","generation":1}'
expect_status 201 "${status}" "create task"

echo "freezing configuration"
freeze_body='{
  "version":1,
  "clock_source":"gps",
  "reference_points":[
    {"id":"r0","x":0,"y":0,"z":0},
    {"id":"r1","x":10000,"y":0,"z":0},
    {"id":"r2","x":0,"y":10000,"z":0},
    {"id":"r3","x":0,"y":0,"z":10000}
  ],
  "transponders":[{"id":"t0","serial":"S1","mount_point":"m0","x":2000,"y":3000,"z":4000}],
  "profile":[{"top_mm":0,"bottom_mm":100000,"speed_mm_s":1000000}],
  "slots":[{"id":"s0","start_us":0,"end_us":1000}],
  "transmit_codes":{"t0":"code-1"},
  "lines":[
    {"id":"l0","reference":"r0","transponder":"t0"},
    {"id":"l1","reference":"r1","transponder":"t0"},
    {"id":"l2","reference":"r2","transponder":"t0"},
    {"id":"l3","reference":"r3","transponder":"t0"}
  ],
  "review_qualifications":[
    {"reviewer_id":"alice","valid_until":8000000000000000000},
    {"reviewer_id":"bob","valid_until":8000000000000000000}
  ],
  "transducer_delay_us":100,
  "residual_threshold_mm":10,
  "drift_threshold_us":100,
  "counter_modulus":1000000,
  "sequence_max":1000,
  "retry_max":3
}'
request POST /v1/tasks/v:L:1/freeze "${freeze_body}"
expect_status 200 "${status}" "freeze"

echo "acquiring bindings and leases"
bind_body='{
  "bindings":[{"serial":"S1","mount_point":"m0"}],
  "leases":[
    {"resource_type":"sink","resource_id":"sink1","duration_us":1000},
    {"resource_type":"reference_clock_port","resource_id":"clk1","duration_us":1000}
  ]
}'
request POST /v1/tasks/v:L:1/bindings:acquire "${bind_body}" "idem-1"
expect_status 200 "${status}" "acquire"

echo "disciplining clock and confirming loopback"
request POST /v1/tasks/v:L:1/clock:discipline '{}'
expect_status 200 "${status}" "discipline"
request POST /v1/tasks/v:L:1/loopback:confirm '{}'
expect_status 200 "${status}" "loopback"

echo "transmitting and receiving echoes"
# Echo receive times = 2 * integer_distance + transducer_delay(100) us.
for entry in "l0 10770" "l1 18968" "l2 16714" "l3 14100"; do
  set -- ${entry}
  line="$1" receive_us="$2"
  request POST /v1/tasks/v:L:1/transmissions "{\"transponder\":\"t0\",\"line\":\"${line}\",\"transmit_us\":0}"
  expect_status 200 "${status}" "transmit ${line}"
done
seq_num=0
for entry in "l0 10770" "l1 18968" "l2 16714" "l3 14100"; do
  set -- ${entry}
  line="$1" receive_us="$2"
  request POST /v1/tasks/v:L:1/echoes "{\"epoch\":1,\"transponder\":\"t0\",\"sequence\":${seq_num},\"line\":\"${line}\",\"transmit_us\":0,\"receive_us\":${receive_us}}"
  expect_status 200 "${status}" "echo ${line}"
  seq_num=$((seq_num + 1))
done

echo "solving baselines"
request POST /v1/tasks/v:L:1/solve '{}'
expect_status 200 "${status}" "solve"

echo "submitting dual review"
request POST /v1/tasks/v:L:1/reviews '{"reviewer_id":"alice"}'
expect_status 200 "${status}" "review alice"
request POST /v1/tasks/v:L:1/reviews '{"reviewer_id":"bob"}'
expect_status 200 "${status}" "review bob"

echo "issuing terminal admission decision"
request POST /v1/tasks/v:L:1/terminal-decisions '{"state":"admitted"}'
expect_status 200 "${status}" "terminal"

echo "querying deployment credential"
request GET /v1/tasks/v:L:1/credential
expect_status 200 "${status}" "credential"
# Assert the credential digest is a non-empty string using the captured body
# variable (no curl | grep pipeline).
cred_digest="$(printf '%s' "${body_out}" | sed -n 's/.*"credential_digest":"\([^"]*\)".*/\1/p')"
if [[ -z "${cred_digest}" ]]; then
  echo "FAIL credential: missing non-empty credential_digest; body=${body_out}" >&2
  exit 1
fi

echo "SMOKE OK: credential_digest=${cred_digest}"
