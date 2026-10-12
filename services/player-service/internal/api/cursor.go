package api

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"reward-platform/pkg/domain"
)

// Cursors are opaque to callers on purpose — encoding them (rather than
// accepting raw createdAt/id query params) means the pagination scheme
// underneath can change later without breaking anyone who treats the
// cursor as "whatever GET /players gave me back last time."
func encodeCursor(p domain.Player) string {
	raw := fmt.Sprintf("%s|%s", p.CreatedAt.Format(time.RFC3339Nano), p.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (*domain.PlayerPageCursor, error) {
	if s == "" {
		return nil, nil //nolint:nilnil // "no cursor" is a legitimate, distinct state from "invalid cursor" below
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid cursor: malformed")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	return &domain.PlayerPageCursor{CreatedAt: createdAt, ID: parts[1]}, nil
}
