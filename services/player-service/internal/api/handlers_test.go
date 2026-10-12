package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"reward-platform/pkg/domain"
	"reward-platform/services/player-service/internal/ledger"
)

// newTestHandler builds a fresh ledger + handler per test, backed by
// in-memory repositories, so tests never share state and don't need a
// real database. The logger writes to io.Discard — we're asserting on
// HTTP responses here, not log output.
func newTestHandler() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	playerLedger := ledger.New(domain.SystemClock{}, domain.NewMemoryPlayerRepository(), domain.NewMemoryLedgerRepository())
	return NewHandler(playerLedger, logger)
}

// doRequest round-trips a request through the handler and decodes the
// JSON response body into T, returning the recorder so tests can also
// assert on the status code.
func doRequest[T any](t *testing.T, h http.Handler, method, path string, body any) (*httptest.ResponseRecorder, T) {
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

func TestHealthz(t *testing.T) {
	h := newTestHandler()
	rec, body := doRequest[map[string]string](t, h, http.MethodGet, "/healthz", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v, want status=ok", body)
	}
}

func TestCreateAndGetPlayer(t *testing.T) {
	h := newTestHandler()

	createRec, created := doRequest[playerResponse](t, h, http.MethodPost, "/players", createPlayerRequest{Username: "alice"})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", createRec.Code)
	}
	if created.Username != "alice" || created.ID == "" {
		t.Fatalf("created player = %+v, want non-empty ID and Username=alice", created)
	}

	getRec, got := doRequest[playerResponse](t, h, http.MethodGet, "/players/"+created.ID, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getRec.Code)
	}
	if got.ID != created.ID {
		t.Fatalf("got ID %q, want %q", got.ID, created.ID)
	}
}

func TestCreatePlayerValidation(t *testing.T) {
	h := newTestHandler()
	rec, body := doRequest[errorResponse](t, h, http.MethodPost, "/players", createPlayerRequest{Username: ""})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if body.Error == "" {
		t.Fatal("expected a non-empty error message")
	}
}

func TestGetPlayerNotFound(t *testing.T) {
	h := newTestHandler()
	rec, _ := doRequest[errorResponse](t, h, http.MethodGet, "/players/does-not-exist", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestListPlayers(t *testing.T) {
	h := newTestHandler()

	doRequest[playerResponse](t, h, http.MethodPost, "/players", createPlayerRequest{Username: "alice"})
	doRequest[playerResponse](t, h, http.MethodPost, "/players", createPlayerRequest{Username: "bob"})

	rec, list := doRequest[[]playerResponse](t, h, http.MethodGet, "/players", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
}

func TestGetBalanceForNewPlayerIsZero(t *testing.T) {
	h := newTestHandler()
	_, player := doRequest[playerResponse](t, h, http.MethodPost, "/players", createPlayerRequest{Username: "alice"})

	rec, balance := doRequest[balanceResponse](t, h, http.MethodGet, "/players/"+player.ID+"/balance", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if balance.Balance != 0 {
		t.Fatalf("balance = %d, want 0", balance.Balance)
	}
}
