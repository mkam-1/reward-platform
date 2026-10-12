package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"reward-platform/pkg/domain"
)

func redemptionKey(id string) string { return "REDEMPTION#" + id }
func playerKey(id string) string     { return "PLAYER#" + id }

type redemptionItem struct {
	PK          string `dynamodbav:"PK"`
	SK          string `dynamodbav:"SK"`
	EntityType  string `dynamodbav:"EntityType"`
	ID          string `dynamodbav:"ID"`
	PlayerID    string `dynamodbav:"PlayerID"`
	RewardID    string `dynamodbav:"RewardID"`
	PointsSpent int    `dynamodbav:"PointsSpent"`
	CreatedAt   string `dynamodbav:"CreatedAt"`
	GSI1PK      string `dynamodbav:"GSI1PK"`
	GSI1SK      string `dynamodbav:"GSI1SK"`
}

func toRedemptionItem(r domain.Redemption) redemptionItem {
	createdAt := r.CreatedAt.Format(time.RFC3339Nano)
	return redemptionItem{
		PK:          rewardKey(r.RewardID),
		SK:          redemptionKey(r.ID),
		EntityType:  "Redemption",
		ID:          r.ID,
		PlayerID:    r.PlayerID,
		RewardID:    r.RewardID,
		PointsSpent: r.PointsSpent,
		CreatedAt:   createdAt,
		// GSI1 groups every redemption by player and sorts by time —
		// the key design that turns "list this player's redemptions"
		// into a cheap indexed Query instead of a table Scan.
		GSI1PK: playerKey(r.PlayerID),
		GSI1SK: redemptionKey(createdAt + "#" + r.ID),
	}
}

func (item redemptionItem) toDomain() (domain.Redemption, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, item.CreatedAt)
	if err != nil {
		return domain.Redemption{}, fmt.Errorf("parse CreatedAt: %w", err)
	}
	return domain.Redemption{
		Model:       domain.Model{ID: item.ID, CreatedAt: createdAt},
		PlayerID:    item.PlayerID,
		RewardID:    item.RewardID,
		PointsSpent: item.PointsSpent,
	}, nil
}

// RedemptionRepository is the DynamoDB-backed domain.RedemptionRepository.
type RedemptionRepository struct {
	client *Client
}

func NewRedemptionRepository(client *Client) *RedemptionRepository {
	return &RedemptionRepository{client: client}
}

// Create's ConditionExpression is the idempotency gate Engine.RedeemReward
// relies on: attribute_not_exists(PK) only lets the PutItem through the
// first time a given redemption ID is written. A retried request with
// the same ID lands here again, the condition fails, and Create reports
// "not created" rather than erroring — the caller (Engine) interprets
// that as "this already happened" and returns the existing record.
func (r *RedemptionRepository) Create(ctx context.Context, redemption domain.Redemption) (bool, error) {
	av, err := attributevalue.MarshalMap(toRedemptionItem(redemption))
	if err != nil {
		return false, fmt.Errorf("create redemption: marshal: %w", err)
	}

	_, err = r.client.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.client.table),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var condFailed *types.ConditionalCheckFailedException
		if errors.As(err, &condFailed) {
			return false, nil
		}
		return false, fmt.Errorf("create redemption: %w", err)
	}
	return true, nil
}

// Get needs the reward ID to form the primary key (PK=REWARD#<rewardID>,
// SK=REDEMPTION#<id>), but the domain.RedemptionRepository interface
// only gives us the redemption ID — so this queries GSI1 by scanning for
// a matching SK instead of a direct GetItem. Acceptable at this scale;
// a table with a huge redemption history would want the redemption ID
// alone to be enough, which would mean reshaping this table's keys.
func (r *RedemptionRepository) Get(ctx context.Context, id string) (domain.Redemption, error) {
	out, err := r.client.db.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(r.client.table),
		FilterExpression: aws.String("EntityType = :t AND ID = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":t":  &types.AttributeValueMemberS{Value: "Redemption"},
			":id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return domain.Redemption{}, fmt.Errorf("get redemption: %w", err)
	}
	if len(out.Items) == 0 {
		return domain.Redemption{}, fmt.Errorf("id %q: %w", id, domain.ErrNotFound)
	}

	var item redemptionItem
	if err := attributevalue.UnmarshalMap(out.Items[0], &item); err != nil {
		return domain.Redemption{}, fmt.Errorf("get redemption: unmarshal: %w", err)
	}
	return item.toDomain()
}

func (r *RedemptionRepository) ListByPlayer(ctx context.Context, playerID string) ([]domain.Redemption, error) {
	out, err := r.client.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.client.table),
		IndexName:              aws.String(gsi1Name),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: playerKey(playerID)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list redemptions: %w", err)
	}

	redemptions := make([]domain.Redemption, 0, len(out.Items))
	for _, raw := range out.Items {
		var item redemptionItem
		if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
			return nil, fmt.Errorf("list redemptions: unmarshal: %w", err)
		}
		redemption, err := item.toDomain()
		if err != nil {
			return nil, fmt.Errorf("list redemptions: %w", err)
		}
		redemptions = append(redemptions, redemption)
	}
	return redemptions, nil
}
