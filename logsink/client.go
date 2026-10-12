// Package logsink ships structured application logs into Elasticsearch
// (OpenSearch on real AWS — see ADR-0009) for search, in addition to
// each service's existing stdout/CloudWatch logging. It's its own
// shared module for the same reason mq is: player-service,
// game-service and rewards-service all need it, and Go's internal/
// visibility rule would otherwise make it importable from only
// whichever one service happened to own it.
package logsink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/go-elasticsearch/v8"
)

// Client wraps the Elasticsearch client plus a few conveniences every
// caller needs (consistent indexing, a reachability check for
// /readyz). Deliberately thin — this package's job is shipping log
// documents, not being a general-purpose Elasticsearch SDK wrapper.
type Client struct {
	es *elasticsearch.Client
}

// NewClient builds a Client pointed at endpoint (empty uses the SDK's
// own default resolution, which doesn't apply here — endpoint is
// always required, unlike mq/dynamodb's local-vs-real-AWS override
// pattern, because Elasticsearch/OpenSearch has no equivalent
// "resolve the real regional endpoint automatically" behavior).
func NewClient(endpoint string) (*Client, error) {
	es, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{endpoint}})
	if err != nil {
		return nil, fmt.Errorf("new elasticsearch client: %w", err)
	}
	return &Client{es: es}, nil
}

// Index writes doc to the given index, auto-generating the document ID
// (log documents have no natural key worth deduplicating on — unlike
// every other write in this project, a duplicated log line is harmless
// clutter, not a correctness bug, so there's no idempotency key here).
func (c *Client) Index(ctx context.Context, index string, doc any) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal log document: %w", err)
	}
	resp, err := c.es.Index(index, bytes.NewReader(body), c.es.Index.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("index log document: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.IsError() {
		return fmt.Errorf("index log document: elasticsearch returned %s", resp.Status())
	}
	return nil
}

// Ping checks the cluster is reachable — used as the dependency check
// behind each service's /readyz, same pattern as the DynamoDB/Postgres
// Ping methods elsewhere in this project.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.es.Ping(c.es.Ping.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("ping elasticsearch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.IsError() {
		return fmt.Errorf("ping elasticsearch: returned %s", resp.Status())
	}
	return nil
}
