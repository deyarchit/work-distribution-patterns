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
func openAWSAPISubscription(ctx context.Context) (*pubsub.Subscription, error) {
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	region := os.Getenv("AWS_REGION")

	hostname, err := os.Hostname()
	if err != nil {
		hostname = fmt.Sprintf("api-%d", time.Now().UnixNano())
	}
	queueName := fmt.Sprintf("api-events-%s", hostname)

	opts := []func(*config.LoadOptions) error{config.WithRegion(region)}
	if endpoint != "" {
		opts = append(opts, config.WithBaseEndpoint(endpoint))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	sqsClient := sqs.NewFromConfig(cfg)
	snsClient := sns.NewFromConfig(cfg)

	createQueueOut, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		return nil, fmt.Errorf("create queue: %w", err)
	}
	queueURL := *createQueueOut.QueueUrl

	getAttrsOut, err := sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{"QueueArn"},
	})
	if err != nil {
		return nil, fmt.Errorf("get queue arn: %w", err)
	}
	queueARN := getAttrsOut.Attributes["QueueArn"]

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

	topicARN := fmt.Sprintf("arn:aws:sns:%s:000000000000:api-events", region)
	_, err = snsClient.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: aws.String(topicARN),
		Protocol: aws.String("sqs"),
		Endpoint: aws.String(queueARN),
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe to sns: %w", err)
	}

	sqsURL := queueURL
	if strings.HasPrefix(sqsURL, "http://") {
		sqsURL = sqsURL[7:]
	} else if strings.HasPrefix(sqsURL, "https://") {
		sqsURL = sqsURL[8:]
	}

	subURL := fmt.Sprintf("awssqs://%s?region=%s", sqsURL, region)
	return pubsub.OpenSubscription(ctx, subURL)
}
