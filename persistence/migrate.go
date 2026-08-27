package persistence

// schemaVersion is the current database migration version. Startup always
// verifies this value before serving traffic.
const schemaVersion = 1

// schema is the full SQLite schema. It uses WAL mode and appends-only event
// logging; unique indexes enforce the active-identity and unexpired-lease
// exclusivity invariants at the database level.
const schema = `
CREATE TABLE IF NOT EXISTS schema_meta (version INTEGER NOT NULL);

CREATE TABLE IF NOT EXISTS tasks (
  voyage_id      TEXT NOT NULL,
  lander_id      TEXT NOT NULL,
  generation     INTEGER NOT NULL,
  phase          INTEGER NOT NULL,
  config_version INTEGER NOT NULL DEFAULT 0,
  frozen_digest  TEXT NOT NULL DEFAULT '',
  terminal_state TEXT NOT NULL DEFAULT '',
  created_at     INTEGER NOT NULL,
  current_epoch  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (voyage_id, lander_id, generation)
);

CREATE TABLE IF NOT EXISTS configurations (
  voyage_id  TEXT NOT NULL,
  lander_id  TEXT NOT NULL,
  generation INTEGER NOT NULL,
  version    INTEGER NOT NULL,
  payload    TEXT NOT NULL,
  PRIMARY KEY (voyage_id, lander_id, generation)
);

CREATE TABLE IF NOT EXISTS bindings (
  serial             TEXT NOT NULL,
  voyage_id          TEXT NOT NULL,
  lander_id          TEXT NOT NULL,
  generation         INTEGER NOT NULL,
  mount_point        TEXT NOT NULL,
  binding_generation INTEGER NOT NULL,
  bound_at           INTEGER NOT NULL,
  PRIMARY KEY (serial)
);

CREATE TABLE IF NOT EXISTS resource_leases (
  resource_type TEXT NOT NULL,
  resource_id   TEXT NOT NULL,
  voyage_id     TEXT NOT NULL,
  lander_id     TEXT NOT NULL,
  generation    INTEGER NOT NULL,
  lease_token   TEXT NOT NULL,
  start_time    INTEGER NOT NULL,
  end_time      INTEGER NOT NULL,
  version       INTEGER NOT NULL,
  PRIMARY KEY (resource_type, resource_id)
);

CREATE TABLE IF NOT EXISTS idempotency (
  idem_key       TEXT PRIMARY KEY,
  content_digest TEXT NOT NULL,
  response_digest TEXT NOT NULL,
  recorded_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS epochs (
  voyage_id    TEXT NOT NULL,
  lander_id    TEXT NOT NULL,
  generation   INTEGER NOT NULL,
  epoch        INTEGER NOT NULL,
  reason       TEXT NOT NULL,
  clock_source TEXT NOT NULL,
  drift_us     INTEGER NOT NULL,
  start_time   INTEGER NOT NULL,
  end_time     INTEGER NOT NULL,
  PRIMARY KEY (voyage_id, lander_id, generation, epoch)
);

CREATE TABLE IF NOT EXISTS evidence (
  seq           INTEGER PRIMARY KEY AUTOINCREMENT,
  voyage_id     TEXT NOT NULL,
  lander_id     TEXT NOT NULL,
  generation    INTEGER NOT NULL,
  transponder   TEXT NOT NULL,
  epoch         INTEGER NOT NULL,
  sequence      INTEGER NOT NULL,
  line          TEXT NOT NULL,
  kind          TEXT NOT NULL,
  transmit_us   INTEGER NOT NULL,
  receive_us    INTEGER NOT NULL,
  valid         INTEGER NOT NULL,
  content_digest TEXT NOT NULL,
  recorded_at   INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_valid_receive
  ON evidence(voyage_id, lander_id, generation, transponder, epoch, sequence)
  WHERE kind = 'receive' AND valid = 1;

CREATE TABLE IF NOT EXISTS solve_results (
  voyage_id  TEXT NOT NULL,
  lander_id  TEXT NOT NULL,
  generation INTEGER NOT NULL,
  payload    TEXT NOT NULL,
  PRIMARY KEY (voyage_id, lander_id, generation)
);

CREATE TABLE IF NOT EXISTS retry_calls (
  voyage_id  TEXT NOT NULL,
  lander_id  TEXT NOT NULL,
  generation INTEGER NOT NULL,
  device     TEXT NOT NULL,
  call_seq   INTEGER NOT NULL,
  attempt    INTEGER NOT NULL,
  next_time  INTEGER NOT NULL,
  last_error TEXT NOT NULL,
  PRIMARY KEY (voyage_id, lander_id, generation, device, call_seq)
);

CREATE TABLE IF NOT EXISTS recalibrations (
  voyage_id             TEXT NOT NULL,
  lander_id             TEXT NOT NULL,
  generation            INTEGER NOT NULL,
  batch_seq             INTEGER NOT NULL,
  reason                TEXT NOT NULL,
  affected_transponders TEXT NOT NULL,
  affected_lines        TEXT NOT NULL,
  new_epoch             INTEGER NOT NULL,
  created_at            INTEGER NOT NULL,
  PRIMARY KEY (voyage_id, lander_id, generation, batch_seq)
);

CREATE TABLE IF NOT EXISTS reviews (
  voyage_id     TEXT NOT NULL,
  lander_id     TEXT NOT NULL,
  generation    INTEGER NOT NULL,
  reviewer_id   TEXT NOT NULL,
  config_digest TEXT NOT NULL,
  solve_digest  TEXT NOT NULL,
  reviewed_at   INTEGER NOT NULL,
  PRIMARY KEY (voyage_id, lander_id, generation, reviewer_id)
);

CREATE TABLE IF NOT EXISTS terminal_decisions (
  voyage_id         TEXT NOT NULL,
  lander_id         TEXT NOT NULL,
  generation        INTEGER NOT NULL,
  barrier_seq       INTEGER NOT NULL,
  state             TEXT NOT NULL,
  credential_digest TEXT NOT NULL,
  decided_at        INTEGER NOT NULL,
  PRIMARY KEY (voyage_id, lander_id, generation)
);

CREATE TABLE IF NOT EXISTS credentials (
  voyage_id  TEXT NOT NULL,
  lander_id  TEXT NOT NULL,
  generation INTEGER NOT NULL,
  digest     TEXT NOT NULL,
  issued_at  INTEGER NOT NULL,
  PRIMARY KEY (voyage_id, lander_id, generation)
);

CREATE TABLE IF NOT EXISTS event_log (
  seq          INTEGER PRIMARY KEY AUTOINCREMENT,
  voyage_id    TEXT NOT NULL,
  lander_id    TEXT NOT NULL,
  generation   INTEGER NOT NULL,
  kind         TEXT NOT NULL,
  payload      TEXT NOT NULL,
  logical_time INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS snapshot_digest (
  id     INTEGER PRIMARY KEY CHECK (id = 0),
  digest TEXT NOT NULL
);
`
