//go:build integration

// Run explicitly (needs Docker):
//
//	go test -tags=integration ./services/rewards-service/internal/dynamodb/...
package dynamodb_test

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"reward-platform/pkg/domain"
	"reward-platform/services/rewards-service/internal/dynamodb"
)

// newTestClient starts a real, throwaway DynamoDB Local container (the
// same image used in deploy/docker-compose.yml) and creates the table
// against it. There's no official testcontainers module for DynamoDB
// Local the way there is for Postgres, so this uses the generic
// container API directly — exactly what that module would do
// underneath anyway.
func newTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "amazon/dynamodb-local:latest",
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor:   wait.ForListeningPort("8000/tcp").WithStartupTimeout(30 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start dynamodb-local container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	// Endpoint already returns a full "http://host:port" URL when given
	// a scheme — it is not just "host:port", despite how that reads.
	endpoint, err := container.Endpoint(ctx, "http")
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}

	// The AWS SDK insists on *some* credentials being configured even
	// against DynamoDB Local, which doesn't check them at all — these
	// dummy values exist purely to satisfy that, not for any real auth.
	t.Setenv("AWS_ACCESS_KEY_ID", "local")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "local")
	t.Setenv("AWS_REGION", "us-east-1")

	client, err := dynamodb.NewClient(ctx, endpoint, "reward_platform_test")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.EnsureTable(ctx); err != nil {
		t.Fatalf("ensure table: %v", err)
	}
	return client
}

func TestRewardAndRedemptionRepositories(t *testing.T) {
	ctx := context.Background()
	client := newTestClient(t)

	rewards := dynamodb.NewRewardRepository(client)
	redemptions := dynamodb.NewRedemptionRepository(client)

	reward := domain.Reward{
		Model:      domain.Model{ID: "reward_test_1", CreatedAt: time.Now().UTC().Truncate(time.Millisecond)},
		Name:       "sticker-pack",
		CostPoints: 30,
		Stock:      1,
	}
	if err := rewards.Put(ctx, reward); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := rewards.Get(ctx, reward.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "sticker-pack" || got.Stock != 1 {
		t.Fatalf("Get = %+v, want Name=sticker-pack Stock=1", got)
	}

	list, err := rewards.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}

	// DecrementStock: first call succeeds (1 -> 0), second must fail
	// with ErrOutOfStock via its ConditionExpression.
	if err := rewards.DecrementStock(ctx, reward.ID); err != nil {
		t.Fatalf("DecrementStock (1st): %v", err)
	}
	if err := rewards.DecrementStock(ctx, reward.ID); err == nil {
		t.Fatal("DecrementStock (2nd): expected ErrOutOfStock, got nil")
	}

	// Redemption idempotency: Create with the same ID twice, second
	// call must report created=false rather than erroring or
	// overwriting.
	redemption := domain.Redemption{
		Model:       domain.Model{ID: "redemption_test_1", CreatedAt: time.Now().UTC()},
		PlayerID:    "player_test_1",
		RewardID:    reward.ID,
		PointsSpent: 30,
	}
	created, err := redemptions.Create(ctx, redemption)
	if err != nil {
		t.Fatalf("Create (1st): %v", err)
	}
	if !created {
		t.Fatal("Create (1st): created = false, want true")
	}

	created, err = redemptions.Create(ctx, redemption)
	if err != nil {
		t.Fatalf("Create (2nd): %v", err)
	}
	if created {
		t.Fatal("Create (2nd): created = true, want false (idempotent replay)")
	}

	byPlayer, err := redemptions.ListByPlayer(ctx, redemption.PlayerID)
	if err != nil {
		t.Fatalf("ListByPlayer: %v", err)
	}
	if len(byPlayer) != 1 {
		t.Fatalf("ListByPlayer() len = %d, want 1", len(byPlayer))
	}
	if byPlayer[0].ID != redemption.ID {
		t.Fatalf("ListByPlayer()[0].ID = %q, want %q", byPlayer[0].ID, redemption.ID)
	}
}
