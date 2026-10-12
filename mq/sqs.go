// Package mq is the shared SQS client every service that publishes or
// consumes events uses — game-service (publishes SessionCompleted),
// player-service (consumes it, publishes PointsAwarded), rewards-service
// (consumes PointsAwarded). It's its own small module rather than living
// under any one service's internal/, specifically because Go's internal/
// visibility rule would otherwise make it importable only within whichever
// single service happened to own it — three independent service modules
// need this, so it needs to be a real, independently-versioned shared
// module, the same reasoning as the proto module.
package mq

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// Client wraps the AWS SDK's SQS client. Same endpoint-override pattern
// as mq's DynamoDB counterpart (services/*/internal/dynamodb): empty
// endpoint resolves the real regional SQS endpoint (real AWS); non-empty
// points at a local SQS-compatible emulator (ElasticMQ in
// deploy/docker-compose.yml) for local development. No code difference
// between the two beyond which string this constructor receives — the
// AWS SDK itself can't tell ElasticMQ from real SQS.
type Client struct {
	sqs *sqs.Client
}

func NewClient(ctx context.Context, endpoint string) (*Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	svc := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	return &Client{sqs: svc}, nil
}

// EnsureQueue creates the named queue if it doesn't already exist and
// returns its URL. SQS's CreateQueue is itself idempotent for a queue
// with matching attributes, so calling this on every service startup
// (the same convenience pattern as DynamoDB's EnsureTable and Postgres's
// migrations) is safe — real AWS (phase 11) would provision the queue
// via Terraform instead, the same caveat noted for the table/migrations.
func (c *Client) EnsureQueue(ctx context.Context, name string) (string, error) {
	out, err := c.sqs.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(name)})
	if err != nil {
		return "", fmt.Errorf("ensure queue %q: %w", name, err)
	}
	return aws.ToString(out.QueueUrl), nil
}

// Publish sends body (expected to already be a JSON-encoded event) to
// the given queue.
func (c *Client) Publish(ctx context.Context, queueURL, body string) error {
	_, err := c.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(body),
	})
	if err != nil {
		return fmt.Errorf("publish to %s: %w", queueURL, err)
	}
	return nil
}

// Handler processes one message's body. Returning an error leaves the
// message on the queue (it becomes visible again after the visibility
// timeout and gets retried) instead of deleting it — "at least once"
// delivery, which is why CreditPoints/DebitPoints on the consuming side
// have to be idempotent in the first place.
type Handler func(ctx context.Context, body string) error

// Consume long-polls queueURL in a loop until ctx is cancelled, calling
// handler for each message and deleting it only if handler succeeds.
//
// This is deliberately simple: one goroutine, no redrive/dead-letter
// handling, no backoff on repeated handler failure — a message that
// keeps failing just keeps getting redelivered forever. A production
// queue would have a redrive policy (AWS-side config, not application
// code) routing messages that fail N times to a dead-letter queue
// instead. Not implemented here; worth knowing about, not worth the
// scope increase for what this phase is demonstrating.
func (c *Client) Consume(ctx context.Context, queueURL string, logger *slog.Logger, handler Handler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		out, err := c.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     10, // long-poll: avoids a tight empty-queue loop
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Error("sqs receive failed", "error", err)
			continue
		}

		for _, msg := range out.Messages {
			body := aws.ToString(msg.Body)
			if err := handler(ctx, body); err != nil {
				logger.Error("sqs handler failed, message will be redelivered", "error", err)
				continue
			}
			if _, err := c.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(queueURL),
				ReceiptHandle: msg.ReceiptHandle,
			}); err != nil {
				logger.Error("sqs delete message failed", "error", err)
			}
		}
	}
}
