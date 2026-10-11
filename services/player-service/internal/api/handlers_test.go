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
)

// newTestHandler builds a fresh engine + handler per test so tests never
// share state. The logger writes to io.Discard — we're asserting on HTTP
// responses here, not log output.
func newTestHandler() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(domain.NewSystemEngine(), logger)
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

func TestSessionAndBalanceFlow(t *testing.T) {
	h := newTestHandler()

	_, player := doRequest[playerResponse](t, h, http.MethodPost, "/players", createPlayerRequest{Username: "alice"})
	_, game := doRequest[gameResponse](t, h, http.MethodPost, "/games", createGameRequest{Name: "dice-run", PointsPerSession: 15})

	sessionRec, entry := doRequest[ledgerEntryResponse](t, h, http.MethodPost, "/players/"+player.ID+"/sessions", recordSessionRequest{GameID: game.ID})
	if sessionRec.Code != http.StatusCreated {
		t.Fatalf("session status = %d, want 201", sessionRec.Code)
	}
	if entry.Delta != 15 {
		t.Fatalf("Delta = %d, want 15", entry.Delta)
	}

	balanceRec, balance := doRequest[balanceResponse](t, h, http.MethodGet, "/players/"+player.ID+"/balance", nil)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("balance status = %d, want 200", balanceRec.Code)
	}
	if balance.Balance != 15 {
		t.Fatalf("balance = %d, want 15", balance.Balance)
	}
}

func TestRedeemRewardInsufficientBalance(t *testing.T) {
	h := newTestHandler()

	_, player := doRequest[playerResponse](t, h, http.MethodPost, "/players", createPlayerRequest{Username: "alice"})
	_, reward := doRequest[rewardResponse](t, h, http.MethodPost, "/rewards", createRewardRequest{Name: "console", CostPoints: 1000, Stock: 1})

	rec, body := doRequest[errorResponse](t, h, http.MethodPost, "/players/"+player.ID+"/redemptions", redeemRequest{RewardID: reward.ID})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if body.Error == "" {
		t.Fatal("expected a non-empty error message")
	}
}
