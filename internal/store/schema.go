// Package store is the SQLite persistence layer for the SPC engine. It owns
// the schema, the connection and the transaction primitives; the chart service
// layers business rules on top.
//
// All write operations run inside a caller-begun IMMEDIATE transaction; Open
// sets _txlock=immediate so every BEGIN is BEGIN IMMEDIATE, serializing
// writers. A single pooled connection (SetMaxOpenConns(1)) avoids "database is
// locked" from interleaved write transactions on extra connections. Control
// limits and violations are recomputed from measurements on every mutation and
// also on demand by the consistency check; the control_limits row is a cache
// the consistency check re-derives and compares against.
package store

// schema is applied on every Open; SQLite's IF NOT EXISTS makes it idempotent
// so a fresh file, a reused file and a post-crash file all converge to the same
// shape.
const schema = `
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS charts (
	chart_id        TEXT PRIMARY KEY,
	name            TEXT NOT NULL,
	characteristic  TEXT NOT NULL,
	chart_type      TEXT NOT NULL,
	unit            TEXT NOT NULL,
	subgroup_size   INTEGER NOT NULL,
	usl             REAL,
	lsl             REAL,
	target          REAL,
	created_seq     INTEGER NOT NULL,
	archived        INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_charts_archived ON charts(archived);
CREATE INDEX IF NOT EXISTS idx_charts_seq ON charts(created_seq);

CREATE TABLE IF NOT EXISTS measurements (
	measurement_id  TEXT PRIMARY KEY,
	chart_id        TEXT NOT NULL,
	subgroup_seq    INTEGER NOT NULL,
	values_json     TEXT NOT NULL,
	value_avg       REAL NOT NULL,
	range_value     REAL NOT NULL,
	defectives      INTEGER NOT NULL DEFAULT 0,
	n_observed      INTEGER NOT NULL DEFAULT 0,
	occurred_seq    INTEGER NOT NULL,
	excluded        INTEGER NOT NULL DEFAULT 0,
	created_seq     INTEGER NOT NULL,
	UNIQUE(chart_id, subgroup_seq)
);

CREATE INDEX IF NOT EXISTS idx_meas_chart ON measurements(chart_id, subgroup_seq);
CREATE INDEX IF NOT EXISTS idx_meas_excluded ON measurements(chart_id, excluded);

CREATE TABLE IF NOT EXISTS violations (
	violation_id    TEXT PRIMARY KEY,
	chart_id        TEXT NOT NULL,
	measurement_seq INTEGER NOT NULL,
	rule            TEXT NOT NULL,
	severity        TEXT NOT NULL,
	detected_seq    INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_viol_chart ON violations(chart_id, measurement_seq);

CREATE TABLE IF NOT EXISTS rule_config (
	chart_id  TEXT NOT NULL,
	rule      TEXT NOT NULL,
	enabled   INTEGER NOT NULL DEFAULT 1,
	PRIMARY KEY (chart_id, rule)
);

CREATE TABLE IF NOT EXISTS control_limits (
	chart_id        TEXT PRIMARY KEY,
	baseline_count  INTEGER NOT NULL,
	cl              REAL NOT NULL,
	ucl             REAL NOT NULL,
	lcl             REAL NOT NULL,
	sigma_within    REAL NOT NULL,
	sigma_overall   REAL NOT NULL,
	computed_seq    INTEGER NOT NULL
);

-- meta holds a single monotonic sequence counter used to stamp created_seq on
-- every chart/measurement/violation row. It is the engine's logical clock:
-- ordering lists by created_seq reflects true insertion order across restarts.
-- The row is seeded on first Open.
INSERT OR IGNORE INTO meta(key, value) VALUES ('next_seq', 0);
`
