// Package dynamodb is player-service's DynamoDB-backed implementation of
// domain.RewardRepository and domain.RedemptionRepository.
//
// Both entity types share one table (single-table design — the standard
// DynamoDB pattern of storing dissimilar item types together, keyed so
// the access patterns that matter stay cheap):
//
//	Reward item:      PK="REWARD#<id>"      SK="REWARD#<id>"
//	Redemption item:  PK="REWARD#<id>"      SK="REDEMPTION#<id>"
//	                  GSI1PK="PLAYER#<id>"  GSI1SK="REDEMPTION#<createdAt>#<id>"
//
// A Redemption's primary key groups it under the reward it's for; its
// GSI1 key lets "list this player's redemption history" run as an
// indexed Query instead of a table Scan. Listing rewards uses a Scan
// with a filter on EntityType instead of a second index — reward
// catalogs are small, bounded, admin-managed lists, which is exactly the
// case a Scan is still a reasonable choice for, in contrast to
// redemptions, which grow unboundedly and genuinely need the index.
package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const gsi1Name = "GSI1"

// Client wraps the AWS SDK's DynamoDB client plus the table name every
// query targets.
type Client struct {
	db    *dynamodb.Client
	table string
}

// NewClient builds a Client. If endpoint is non-empty, it overrides the
// SDK's default AWS endpoint resolution — that's what points this at
// DynamoDB Local for development instead of the real AWS DynamoDB.
func NewClient(ctx context.Context, endpoint, table string) (*Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	db := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	return &Client{db: db, table: table}, nil
}

// EnsureTable creates the table (with its one GSI) if it doesn't already
// exist. Convenient for local development and DynamoDB Local; real AWS
// (phase 11) provisions this with Terraform instead, the same way phase
// 4 noted for Postgres migrations — a table is infrastructure, and
// infrastructure a whole fleet of instances would otherwise all race to
// create on startup belongs in an explicit, auditable IaC step, not
// application boot code.
func (c *Client) EnsureTable(ctx context.Context) error {
	_, err := c.db.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(c.table),
	})
	if err == nil {
		return nil // already exists
	}
	var notFound *types.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return fmt.Errorf("describe table: %w", err)
	}

	_, err = c.db.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(c.table),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("SK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI1PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI1SK"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String(gsi1Name),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("GSI1PK"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("GSI1SK"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	waiter := dynamodb.NewTableExistsWaiter(c.db)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(c.table)}, 30*time.Second); err != nil {
		return fmt.Errorf("wait for table: %w", err)
	}
	return nil
}
