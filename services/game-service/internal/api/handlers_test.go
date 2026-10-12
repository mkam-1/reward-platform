package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"reward-platform/pkg/domain"
	"reward-platform/services/game-service/internal/catalog"
)

// fakePublisher records published messages instead of talking to SQS —
// handlers_test.go is testing routing/validation/status codes, not
// messaging infrastructure.
type fakePublisher struct {
	mu        sync.Mutex
	published []string
}

func (p *fakePublisher) Publish(_ context.Context, _ string, body string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, body)
	return nil
}

func newTestHandler(pub *fakePublisher) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gameCatalog := catalog.New(domain.SystemClock{}, domain.NewStore[domain.Game](), pub, "test-queue-url")
	return NewHandler(gameCatalog, logger, nil)
}

func doRequest[T any](t *testing.T, h http.Handler, method, path string, body any, headers ...[2]string) (*httptest.ResponseRecorder, T) {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	for _, hdr := range headers {
		req.Header.Set(hdr[0], hdr[1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var decoded T
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("unmarshal response body %q: %v", rec.Body.String(), err)
		}
	}
	return rec, decoded
}

func TestCreateAndGetGame(t *testing.T) {
	h := newTestHandler(&fakePublisher{})

	createRec, created := doRequest[gameResponse](t, h, http.MethodPost, "/games", createGameRequest{Name: "dice-run", PointsPerSession: 15})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", createRec.Code)
	}

	getRec, got := doRequest[gameResponse](t, h, http.MethodGet, "/games/"+created.ID, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getRec.Code)
	}
	if got.Name != "dice-run" {
		t.Fatalf("Name = %q, want dice-run", got.Name)
	}
}

func TestRecordSessionPublishesEvent(t *testing.T) {
	pub := &fakePublisher{}
	h := newTestHandler(pub)

	_, game := doRequest[gameResponse](t, h, http.MethodPost, "/games", createGameRequest{Name: "dice-run", PointsPerSession: 15})

	rec, _ := doRequest[map[string]string](t, h, http.MethodPost, "/games/"+game.ID+"/sessions", recordSessionRequest{PlayerID: "player_1"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(pub.published))
	}
	if !bytes.Contains([]byte(pub.published[0]), []byte(`"playerId":"player_1"`)) {
		t.Fatalf("published body = %s, want it to contain playerId=player_1", pub.published[0])
	}
}

func TestRecordSessionUnknownGame(t *testing.T) {
	h := newTestHandler(&fakePublisher{})

	rec, _ := doRequest[map[string]string](t, h, http.MethodPost, "/games/does-not-exist/sessions", recordSessionRequest{PlayerID: "player_1"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
