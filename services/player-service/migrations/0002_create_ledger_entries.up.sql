CREATE TABLE ledger_entries (
    id         TEXT PRIMARY KEY,
    player_id  TEXT NOT NULL REFERENCES players(id),
    delta      INTEGER NOT NULL,
    reason     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

-- Balance() is `SELECT SUM(delta) WHERE player_id = $1` — this index is
-- what keeps that fast as the ledger grows, instead of a full table scan
-- per balance check.
CREATE INDEX idx_ledger_entries_player_id ON ledger_entries (player_id);
