CREATE TABLE outbox_events (
    id           TEXT PRIMARY KEY,
    event_type   TEXT NOT NULL,
    payload      TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ
);

-- The relay polls for unpublished rows in creation order; this index is
-- what keeps that a cheap, indexed lookup instead of a growing table
-- scan as published rows pile up.
CREATE INDEX idx_outbox_events_unpublished ON outbox_events (created_at) WHERE published_at IS NULL;
