package pubsubinternal

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/pitabwire/natspubsub" // Register nats:// scheme with JetStream support
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/awssnssqs"   // Register awssns:// and awssnssqs:// schemes
	_ "gocloud.dev/pubsub/kafkapubsub" // Register kafka:// scheme
)

// ManagerResources holds the Go Cloud PubSub resources required by the manager.
type ManagerResources struct {
	TasksTopic      *pubsub.Topic
	WorkerEventsSub *pubsub.Subscription
	APIEventsTopic  *pubsub.Topic
}

// OpenManagerResources initializes Go Cloud resources for the manager.
func OpenManagerResources(ctx context.Context, brokerURL string) (ManagerResources, error) {
	scheme := strings.Split(brokerURL, "://")[0]

	var tasksTopicURL, eventsSubURL, apiEventsTopicURL string

	switch scheme {
	case "nats":
		// NATS JetStream URLs
		tasksTopicURL = brokerURL + "/tasks.new?stream_name=TASKS"
		eventsSubURL = brokerURL + "/events.workers?stream_name=EVENTS&consumer_durable_name=manager-events"
		apiEventsTopicURL = brokerURL + "/events.api?stream_name=EVENTS"

	case "kafka":
		// Kafka URLs (connection from KAFKA_BROKERS env var)
		// For topics, the path is the topic name
		// For subscriptions: kafka://consumer-group?topic=topic-name
		// Separate topics to avoid feedback loop: workers→worker_events, manager→api_events
		tasksTopicURL = "kafka://tasks"
		eventsSubURL = "kafka://manager?topic=worker_events"
		apiEventsTopicURL = "kafka://api_events"

	case "awssqs", "awssnssqs":
		// AWS: SQS for point-to-point, SNS for fanout
		endpoint := os.Getenv("AWS_ENDPOINT_URL") // http://localstack:4566
		region := os.Getenv("AWS_REGION")         // us-east-1

		// Strip http:// or https:// from endpoint for gocloud URLs
		sqsEndpoint := strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")

		// Manager publishes tasks to SQS (workers compete for them)
		tasksTopicURL = fmt.Sprintf("awssqs://%s/000000000000/worker-tasks?region=%s", sqsEndpoint, region)

		// Manager consumes events from SQS (workers publish to it)
		eventsSubURL = fmt.Sprintf("awssqs://%s/000000000000/manager-events?region=%s", sqsEndpoint, region)

		// Manager publishes to SNS for fanout to APIs
		// Note the 3 slashes; ARNs have colons and aren't valid as hostnames
		topicARN := fmt.Sprintf("arn:aws:sns:%s:000000000000:api-events", region)
		apiEventsTopicURL = fmt.Sprintf("awssns:///%s?region=%s&endpoint=%s", topicARN, region, endpoint)

	default:
		return ManagerResources{}, fmt.Errorf("unsupported broker scheme: %s (supported: nats, kafka, awssqs)", scheme)
	}

	tasksTopic, err := pubsub.OpenTopic(ctx, tasksTopicURL)
	if err != nil {
		return ManagerResources{}, err
	}

	eventsSub, err := pubsub.OpenSubscription(ctx, eventsSubURL)
	if err != nil {
		return ManagerResources{}, err
	}

	apiEventsTopic, err := pubsub.OpenTopic(ctx, apiEventsTopicURL)
	if err != nil {
		return ManagerResources{}, err
	}

	return ManagerResources{
		TasksTopic:      tasksTopic,
		WorkerEventsSub: eventsSub,
		APIEventsTopic:  apiEventsTopic,
	}, nil
}

// OpenWorkerResources initializes Go Cloud resources for workers.
func OpenWorkerResources(ctx context.Context, brokerURL string) (*pubsub.Subscription, *pubsub.Topic, error) {
	scheme := strings.Split(brokerURL, "://")[0]

	var tasksSubURL, eventsTopicURL string

	switch scheme {
	case "nats":
		// NATS JetStream URLs
		tasksSubURL = brokerURL + "/tasks.new?stream_name=TASKS&consumer_durable_name=workers"
		eventsTopicURL = brokerURL + "/events.workers?stream_name=EVENTS"

	case "kafka":
		// Kafka URLs (connection from KAFKA_BROKERS env var)
		// For subscriptions: kafka://consumer-group?topic=topic-name
		// Workers share tasks via consumer group "workers"
		tasksSubURL = "kafka://workers?topic=tasks"
		eventsTopicURL = "kafka://worker_events"

	case "awssqs", "awssnssqs":
		// AWS: SQS for both tasks and events
		endpoint := os.Getenv("AWS_ENDPOINT_URL") // http://localstack:4566
		region := os.Getenv("AWS_REGION")         // us-east-1

		// Strip http:// or https:// from endpoint for gocloud URLs
		sqsEndpoint := strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")

		// Workers consume tasks from SQS (competing with other workers)
		tasksSubURL = fmt.Sprintf("awssqs://%s/000000000000/worker-tasks?region=%s", sqsEndpoint, region)

		// Workers publish events to SQS (manager is sole consumer)
		eventsTopicURL = fmt.Sprintf("awssqs://%s/000000000000/manager-events?region=%s", sqsEndpoint, region)

	default:
		return nil, nil, fmt.Errorf("unsupported broker scheme: %s (supported: nats, kafka, awssqs)", scheme)
	}

	tasksSub, err := pubsub.OpenSubscription(ctx, tasksSubURL)
	if err != nil {
		return nil, nil, err
	}

	eventsTopic, err := pubsub.OpenTopic(ctx, eventsTopicURL)
	if err != nil {
		return nil, nil, err
	}

	return tasksSub, eventsTopic, nil
}

// OpenAPIResources initializes Go Cloud resources for API servers.
func OpenAPIResources(ctx context.Context, brokerURL string) (*pubsub.Subscription, error) {
	scheme := strings.Split(brokerURL, "://")[0]

	var eventsSubURL string

	switch scheme {
	case "nats":
		// NATS JetStream URL (ephemeral consumer)
		eventsSubURL = brokerURL + "/events.api?stream_name=EVENTS"

	case "kafka":
		// Each API instance uses its hostname as unique consumer group (auto-scales)
		// Docker/K8s automatically assigns unique hostnames like p06-api-1, p06-api-2, etc.
		hostname, err := os.Hostname()
		if err != nil {
			// Fallback: generate unique ID if hostname fails
			hostname = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		eventsSubURL = fmt.Sprintf("kafka://api-%s?topic=api_events", hostname)

	case "awssqs", "awssnssqs":
		// AWS: dynamically create queue and subscription
		return openAWSAPISubscription(ctx)

	default:
		return nil, fmt.Errorf("unsupported broker scheme: %s (supported: nats, kafka, awssqs)", scheme)
	}

	eventsSub, err := pubsub.OpenSubscription(ctx, eventsSubURL)
	if err != nil {
		return nil, err
	}

	return eventsSub, nil
}
