package mq

import (
	"context"

	"github.com/runabol/tork"
)

type Provider func() (Broker, error)

const (
	BROKER_INMEMORY     = "inmemory"
	BROKER_RABBITMQ     = "rabbitmq"
	TOPIC_JOB           = "job.*"
	TOPIC_JOB_COMPLETED = "job.completed"
	TOPIC_JOB_FAILED    = "job.failed"
)

// Broker is the message-queue, pub/sub mechanism used for delivering tasks.
type Broker interface {
	PublishTask(ctx context.Context, qname string, t *tork.Task) error
	SubscribeForTasks(qname string, handler func(t *tork.Task) error) error

	PublishHeartbeat(ctx context.Context, n *tork.Node) error
	SubscribeForHeartbeats(handler func(n *tork.Node) error) error

	PublishJob(ctx context.Context, j *tork.Job) error
	SubscribeForJobs(handler func(j *tork.Job) error) error

	PublishEvent(ctx context.Context, topic string, event any) error
	SubscribeForEvents(ctx context.Context, pattern string, handler func(event any)) error

	PublishTaskLogPart(ctx context.Context, p *tork.TaskLogPart) error
	SubscribeForTaskLogPart(handler func(p *tork.TaskLogPart)) error

	PublishService(ctx context.Context, s *tork.Service) error
	SubscribeForServices(handler func(s *tork.Service) error) error

	Queues(ctx context.Context) ([]QueueInfo, error)
	HealthCheck(ctx context.Context) error
	Shutdown(ctx context.Context) error
}
