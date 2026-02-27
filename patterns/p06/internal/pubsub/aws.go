package pubsubinternal

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"gocloud.dev/pubsub"
)

// openAWSAPISubscription dynamically creates an SQS queue for the API instance,
// subscribes it to the api-events SNS topic, and returns a gocloud Subscription.
// This allows API instances to scale dynamically without pre-creating queues.
func openAWSAPISubscription(ctx context.Context) (*pubsub.Subscription, error) {
	endpoint := os.Getenv("AWS_ENDPOINT_URL") // http://localstack:4566
	region := os.Getenv("AWS_REGION")         // us-east-1

	// Generate unique queue name based on hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("api-%d", time.Now().UnixNano())
	}
	queueName := fmt.Sprintf("api-events-%s", hostname)

	// Configure AWS SDK
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	// Use custom endpoint for LocalStack if provided
	if endpoint != "" {
		opts = append(opts, config.WithBaseEndpoint(endpoint))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	sqsClient := sqs.NewFromConfig(cfg)
	snsClient := sns.NewFromConfig(cfg)

	// 1. Create SQS queue (idempotent)
	createQueueOut, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		return nil, fmt.Errorf("create queue: %w", err)
	}
	queueURL := *createQueueOut.QueueUrl

	// 2. Get queue ARN for SNS subscription
	getAttrsOut, err := sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{"QueueArn"},
	})
	if err != nil {
		return nil, fmt.Errorf("get queue arn: %w", err)
	}
	queueARN := getAttrsOut.Attributes["QueueArn"]

	// 3. Set queue policy to allow SNS to publish
	policy := fmt.Sprintf(`{
		"Statement": [{
			"Effect": "Allow",
			"Principal": {"Service": "sns.amazonaws.com"},
			"Action": "sqs:SendMessage",
			"Resource": "%s",
			"Condition": {
				"ArnEquals": {"aws:SourceArn": "arn:aws:sns:%s:000000000000:api-events"}
			}
		}]
	}`, queueARN, region)

	_, err = sqsClient.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl:   aws.String(queueURL),
		Attributes: map[string]string{"Policy": policy},
	})
	if err != nil {
		return nil, fmt.Errorf("set queue policy: %w", err)
	}

	// 4. Subscribe queue to SNS topic (idempotent)
	topicARN := fmt.Sprintf("arn:aws:sns:%s:000000000000:api-events", region)
	_, err = snsClient.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: aws.String(topicARN),
		Protocol: aws.String("sqs"),
		Endpoint: aws.String(queueARN),
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe to sns: %w", err)
	}

	// 5. Open gocloud subscription
	// The awssnssqs package provides the awssqs:// scheme for subscriptions
	// queueURL is like: http://sqs.us-east-1.localhost.localstack.cloud:4566/000000000000/queue-name
	// We need: awssqs://sqs.us-east-1.localhost.localstack.cloud:4566/000000000000/queue-name?region=us-east-1
	// Strip the http:// prefix from queueURL
	sqsURL := queueURL
	if strings.HasPrefix(sqsURL, "http://") {
		sqsURL = sqsURL[7:] // Remove "http://"
	} else if strings.HasPrefix(sqsURL, "https://") {
		sqsURL = sqsURL[8:] // Remove "https://"
	}

	subURL := fmt.Sprintf("awssqs://%s?region=%s", sqsURL, region)
	return pubsub.OpenSubscription(ctx, subURL)
}
