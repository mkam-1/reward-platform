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

func rewardKey(id string) string { return "REWARD#" + id }

// rewardItem is this table's on-disk shape for a Reward — kept separate
// from domain.Reward for the same reason the HTTP layer's DTOs are kept
// separate from domain types: the storage shape and the domain shape are
// allowed to evolve independently.
type rewardItem struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	EntityType string `dynamodbav:"EntityType"`
	ID         string `dynamodbav:"ID"`
	Name       string `dynamodbav:"Name"`
	CostPoints int    `dynamodbav:"CostPoints"`
	Stock      int    `dynamodbav:"Stock"`
	CreatedAt  string `dynamodbav:"CreatedAt"`
}

func toRewardItem(r domain.Reward) rewardItem {
	return rewardItem{
		PK:         rewardKey(r.ID),
		SK:         rewardKey(r.ID),
		EntityType: "Reward",
		ID:         r.ID,
		Name:       r.Name,
		CostPoints: r.CostPoints,
		Stock:      r.Stock,
		CreatedAt:  r.CreatedAt.Format(time.RFC3339Nano),
	}
}

func (item rewardItem) toDomain() (domain.Reward, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, item.CreatedAt)
	if err != nil {
		return domain.Reward{}, fmt.Errorf("parse CreatedAt: %w", err)
	}
	return domain.Reward{
		Model:      domain.Model{ID: item.ID, CreatedAt: createdAt},
		Name:       item.Name,
		CostPoints: item.CostPoints,
		Stock:      item.Stock,
	}, nil
}

// RewardRepository is the DynamoDB-backed domain.RewardRepository.
type RewardRepository struct {
	client *Client
}

func NewRewardRepository(client *Client) *RewardRepository {
	return &RewardRepository{client: client}
}

func (r *RewardRepository) Put(ctx context.Context, reward domain.Reward) error {
	av, err := attributevalue.MarshalMap(toRewardItem(reward))
	if err != nil {
		return fmt.Errorf("put reward: marshal: %w", err)
	}
	_, err = r.client.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.client.table),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("put reward: %w", err)
	}
	return nil
}

func (r *RewardRepository) Get(ctx context.Context, id string) (domain.Reward, error) {
	out, err := r.client.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.client.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: rewardKey(id)},
			"SK": &types.AttributeValueMemberS{Value: rewardKey(id)},
		},
	})
	if err != nil {
		return domain.Reward{}, fmt.Errorf("get reward: %w", err)
	}
	if out.Item == nil {
		return domain.Reward{}, fmt.Errorf("id %q: %w", id, domain.ErrNotFound)
	}

	var item rewardItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return domain.Reward{}, fmt.Errorf("get reward: unmarshal: %w", err)
	}
	return item.toDomain()
}

// List Scans the table and filters to Reward items. A Scan reads every
// item in the table, not just rewards — acceptable here because this
// table only ever holds a small, catalog-sized number of rewards plus
// however many redemptions exist; it would stop being acceptable long
// before a real production catalog got large, at which point the fix is
// the same GSI-by-type trick GSI1 already uses for redemptions.
func (r *RewardRepository) List(ctx context.Context) ([]domain.Reward, error) {
	out, err := r.client.db.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(r.client.table),
		FilterExpression: aws.String("EntityType = :t"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":t": &types.AttributeValueMemberS{Value: "Reward"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list rewards: %w", err)
	}

	rewards := make([]domain.Reward, 0, len(out.Items))
	for _, raw := range out.Items {
		var item rewardItem
		if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
			return nil, fmt.Errorf("list rewards: unmarshal: %w", err)
		}
		reward, err := item.toDomain()
		if err != nil {
			return nil, fmt.Errorf("list rewards: %w", err)
		}
		rewards = append(rewards, reward)
	}
	return rewards, nil
}

// DecrementStock is a conditional UpdateItem: it only succeeds if Stock
// is currently greater than zero, and the check-and-decrement happens
// as one atomic operation on DynamoDB's side. That's what makes this
// safe across any number of player-service instances sharing this
// table — no in-process mutex required, unlike the in-memory
// implementation.
func (r *RewardRepository) DecrementStock(ctx context.Context, id string) error {
	_, err := r.client.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.client.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: rewardKey(id)},
			"SK": &types.AttributeValueMemberS{Value: rewardKey(id)},
		},
		UpdateExpression:    aws.String("SET Stock = Stock - :one"),
		ConditionExpression: aws.String("Stock > :zero"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one":  &types.AttributeValueMemberN{Value: "1"},
			":zero": &types.AttributeValueMemberN{Value: "0"},
		},
	})
	if err != nil {
		var condFailed *types.ConditionalCheckFailedException
		if errors.As(err, &condFailed) {
			return fmt.Errorf("reward %q: %w", id, domain.ErrOutOfStock)
		}
		return fmt.Errorf("decrement stock: %w", err)
	}
	return nil
}
