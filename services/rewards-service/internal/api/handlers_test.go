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
	"reward-platform/services/rewards-service/internal/catalog"
)

// fakePlayerBalance stands in for a gRPC call to player-service — same
// shape as catalog's own test fake (catalog_test.go), redeclared here
// since that one lives in package catalog_test and isn't importable.
type fakePlayerBalance struct {
	mu       sync.Mutex
	balances map[string]int
}

func (f *fakePlayerBalance) GetBalance(_ context.Context, playerID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.balances[playerID], nil
}

func (f *fakePlayerBalance) DebitPoints(_ context.Context, playerID string, amount int, _, _ string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.balances[playerID] < amount {
		return 0, domain.ErrInsufficientBalance
	}
	f.balances[playerID] -= amount
	return f.balances[playerID], nil
}

func newTestHandler(balances map[string]int) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rewardCatalog := catalog.New(domain.SystemClock{}, domain.NewMemoryRewardRepository(), domain.NewMemoryRedemptionRepository(), &fakePlayerBalance{balances: balances})
	return NewHandler(rewardCatalog, logger, nil)
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

func TestHealthz(t *testing.T) {
	h := newTestHandler(nil)
	rec, body := doRequest[map[string]string](t, h, http.MethodGet, "/healthz", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v, want status=ok", body)
	}
}

func TestCreateAndListRewards(t *testing.T) {
	h := newTestHandler(nil)

	createRec, created := doRequest[rewardResponse](t, h, http.MethodPost, "/rewards", createRewardRequest{Name: "sticker-pack", CostPoints: 10, Stock: 5})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", createRec.Code)
	}

	listRec, list := doRequest[[]rewardResponse](t, h, http.MethodGet, "/rewards", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listRec.Code)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want just %+v", list, created)
	}
}

func TestRedeemRewardSuccess(t *testing.T) {
	h := newTestHandler(map[string]int{"player_1": 50})
	_, reward := doRequest[rewardResponse](t, h, http.MethodPost, "/rewards", createRewardRequest{Name: "sticker-pack", CostPoints: 30, Stock: 5})

	rec, redemption := doRequest[redemptionResponse](t, h, http.MethodPost, "/players/player_1/redemptions", redeemRequest{RewardID: reward.ID}, [2]string{"Idempotency-Key", "demo-key-1"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if redemption.ID != "demo-key-1" {
		t.Fatalf("redemption.ID = %q, want the Idempotency-Key header value", redemption.ID)
	}
	if redemption.PointsSpent != 30 {
		t.Fatalf("PointsSpent = %d, want 30", redemption.PointsSpent)
	}
}

func TestRedeemRewardInsufficientBalance(t *testing.T) {
	h := newTestHandler(map[string]int{"player_1": 10})
	_, reward := doRequest[rewardResponse](t, h, http.MethodPost, "/rewards", createRewardRequest{Name: "console", CostPoints: 1000, Stock: 5})

	rec, body := doRequest[errorResponse](t, h, http.MethodPost, "/players/player_1/redemptions", redeemRequest{RewardID: reward.ID})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if body.Error == "" {
		t.Fatal("expected a non-empty error message")
	}
}

func TestListRedemptions(t *testing.T) {
	h := newTestHandler(map[string]int{"player_1": 100})
	_, reward := doRequest[rewardResponse](t, h, http.MethodPost, "/rewards", createRewardRequest{Name: "sticker-pack", CostPoints: 10, Stock: 5})

	for i := 0; i < 2; i++ {
		doRequest[redemptionResponse](t, h, http.MethodPost, "/players/player_1/redemptions", redeemRequest{RewardID: reward.ID})
	}

	rec, list := doRequest[[]redemptionResponse](t, h, http.MethodGet, "/players/player_1/redemptions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
}
