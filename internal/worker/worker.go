package worker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pkg/errors"
	"github.com/runabol/tork"
	"github.com/runabol/tork/middleware/task"
	"github.com/runabol/tork/mq"

	"github.com/runabol/tork/internal/host"
	"github.com/runabol/tork/internal/syncx"
	"github.com/runabol/tork/runtime"

	"github.com/runabol/tork/internal/uuid"
)

type Worker struct {
	id         string
	name       string
	startTime  time.Time
	runtime    runtime.Runtime
	broker     mq.Broker
	stop       chan any
	queues     map[string]int
	tasks      *syncx.Map[string, runningTask]
	limits     Limits
	api        *api
	taskCount  int32
	middleware []task.MiddlewareFunc
}

type Config struct {
	Name       string
	Address    string
	Broker     mq.Broker
	Runtime    runtime.Runtime
	Queues     map[string]int
	Limits     Limits
	Middleware []task.MiddlewareFunc
}

type Limits struct {
	DefaultCPUsLimit   string
	DefaultMemoryLimit string
	DefaultTimeout     string
}

type runningTask struct {
	cancel context.CancelFunc
	ports  map[string]*tork.Port
}

func NewWorker(cfg Config) (*Worker, error) {
	if len(cfg.Queues) == 0 {
		cfg.Queues = map[string]int{mq.QUEUE_DEFAULT: 1}
	}
	if cfg.Broker == nil {
		return nil, errors.New("must provide broker")
	}
	if cfg.Runtime == nil {
		return nil, errors.New("must provide runtime")
	}
	tasks := new(syncx.Map[string, runningTask])
	w := &Worker{
		id:         uuid.NewShortUUID(),
		name:       cfg.Name,
		startTime:  time.Now().UTC(),
		broker:     cfg.Broker,
		runtime:    cfg.Runtime,
		queues:     cfg.Queues,
		tasks:      tasks,
		limits:     cfg.Limits,
		api:        newAPI(cfg, tasks),
		stop:       make(chan any),
		middleware: cfg.Middleware,
	}
	return w, nil
}

func (w *Worker) cancelTask(t *tork.Task) error {
	rt, ok := w.tasks.Get(t.ID)
	if !ok {
		log.Debug().Msgf("unknown task %s. nothing to cancel", t.ID)
		return nil
	}
	log.Debug().Msgf("cancelling task %s", t.ID)
	rt.cancel()
	w.tasks.Delete(t.ID)
	return nil
}

func (w *Worker) handleTask(t *tork.Task) error {
	started := time.Now().UTC()
	t.StartedAt = &started
	t.NodeID = w.id
	// prepare limits
	if t.Limits == nil && (w.limits.DefaultCPUsLimit != "" || w.limits.DefaultMemoryLimit != "") {
		t.Limits = &tork.TaskLimits{}
	}
	if t.Limits != nil && t.Limits.CPUs == "" {
		t.Limits.CPUs = w.limits.DefaultCPUsLimit
	}
	if t.Limits != nil && t.Limits.Memory == "" {
		t.Limits.Memory = w.limits.DefaultMemoryLimit
	}
	if t.Timeout == "" {
		t.Timeout = w.limits.DefaultTimeout
	}
	adapter := func(ctx context.Context, et task.EventType, t *tork.Task) error {
		return w.runTask(t)
	}
	mw := task.ApplyMiddleware(adapter, w.middleware)
	if err := mw(context.Background(), task.StateChange, t); err != nil {
		now := time.Now().UTC()
		t.Error = err.Error()
		t.FailedAt = &now
		t.State = tork.TaskStateFailed
		return w.broker.PublishTask(context.Background(), mq.QUEUE_ERROR, t)
	}
	return nil
}

func (w *Worker) runTask(t *tork.Task) error {
	if t.Probe != nil {
		return w.runServiceTask(t)
	}
	return w.runRegularTask(t)
}

func (w *Worker) runServiceTask(t *tork.Task) error {
	atomic.AddInt32(&w.taskCount, 1)
	// create a cancellation context in case
	// the coordinator wants to cancel the
	// task later on
	ctx, cancel := context.WithCancel(context.Background())
	w.tasks.Set(t.ID, runningTask{
		cancel: cancel,
	})
	// clone the task so that the runtime
	// process can mutate the task without
	// affecting the original
	rtTask := t.Clone()
	// run the task
	go func() {
		defer func() {
			atomic.AddInt32(&w.taskCount, -1)
		}()
		defer cancel()
		defer w.tasks.Delete(t.ID)
		// actually run the task
		if err := w.runtime.Run(ctx, rtTask); err != nil {
			finished := time.Now().UTC()
			t.FailedAt = &finished
			t.State = tork.TaskStateFailed
			t.Error = err.Error()
		}
		if t.State == tork.TaskStateFailed {
			if err := w.broker.PublishTask(ctx, mq.QUEUE_ERROR, t); err != nil {
				log.Error().Err(err).Msgf("Error publishing service task to error queue")
			}
		}
	}()

	// probe the task for 200 response
	maxAttempts := 30
	for i := 0; i < maxAttempts; i++ {
		for _, port := range rtTask.Ports {
			if port.Address != "" {
				resp, err := http.Get(fmt.Sprintf("http://%s", port.Address))
				if err != nil {
					log.Debug().Msgf("error with service health check: %s", err.Error())
				} else if resp.StatusCode == http.StatusOK {
					w.tasks.Set(t.ID, runningTask{
						cancel: cancel,
						ports:  rtTask.Ports,
					})
					if err := w.broker.PublishTask(ctx, mq.QUEUE_STARTED, t); err != nil {
						return err
					}
					return nil
				} else {
					log.Debug().Msgf("health check failed with: %d", resp.StatusCode)
				}
			}
		}
		time.Sleep(time.Second * 2)
	}

	finished := time.Now().UTC()
	t.FailedAt = &finished
	t.State = tork.TaskStateFailed
	t.Error = "Health check failed"
	if err := w.broker.PublishTask(ctx, mq.QUEUE_ERROR, t); err != nil {
		return err
	}

	return nil
}

func (w *Worker) runRegularTask(t *tork.Task) error {
	atomic.AddInt32(&w.taskCount, 1)
	defer func() {
		atomic.AddInt32(&w.taskCount, -1)
	}()
	// create a cancellation context in case
	// the coordinator wants to cancel the
	// task later on
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.tasks.Set(t.ID, runningTask{
		cancel: cancel,
	})
	defer w.tasks.Delete(t.ID)
	// let the coordinator know that the task started executing
	if err := w.broker.PublishTask(ctx, mq.QUEUE_STARTED, t); err != nil {
		return err
	}
	rctx := ctx
	if t.Timeout != "" {
		dur, err := time.ParseDuration(t.Timeout)
		if err != nil {
			return errors.Wrapf(err, "invalid timeout duration: %s", t.Timeout)
		}
		tctx, cancel := context.WithTimeout(ctx, dur)
		defer cancel()
		rctx = tctx
	}
	// clone the task so that the runtime
	// process can mutate the task without
	// affecting the original
	rtTask := t.Clone()
	// run the task
	if err := w.runtime.Run(rctx, rtTask); err != nil {
		finished := time.Now().UTC()
		t.FailedAt = &finished
		t.State = tork.TaskStateFailed
		t.Error = err.Error()
	} else {
		finished := time.Now().UTC()
		t.CompletedAt = &finished
		t.State = tork.TaskStateCompleted
		t.Result = rtTask.Result
	}
	switch t.State {
	case tork.TaskStateCompleted:
		if err := w.broker.PublishTask(ctx, mq.QUEUE_COMPLETED, t); err != nil {
			return err
		}
	case tork.TaskStateFailed:
		if err := w.broker.PublishTask(ctx, mq.QUEUE_ERROR, t); err != nil {
			return err
		}
	default:
		return errors.Errorf("unexpected state %s for task %s", t.State, t.ID)
	}
	return nil
}

func (w *Worker) sendHeartbeats() {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		status := tork.NodeStatusUP
		if err := w.runtime.HealthCheck(ctx); err != nil {
			log.Error().Err(err).Msgf("node %s failed health check", w.id)
			status = tork.NodeStatusDown
		}
		hostname, err := os.Hostname()
		if err != nil {
			log.Error().Err(err).Msgf("failed to get hostname for worker %s", w.id)
		}
		cpuPercent := host.GetCPUPercent()
		err = w.broker.PublishHeartbeat(
			context.Background(),
			&tork.Node{
				ID:              w.id,
				Name:            w.name,
				StartedAt:       w.startTime,
				CPUPercent:      cpuPercent,
				Queue:           fmt.Sprintf("%s%s", mq.QUEUE_EXCLUSIVE_PREFIX, w.id),
				Status:          status,
				LastHeartbeatAt: time.Now().UTC(),
				Hostname:        hostname,
				Port:            w.api.port,
				TaskCount:       int(atomic.LoadInt32(&w.taskCount)),
				Version:         tork.Version,
			},
		)
		if err != nil {
			log.Error().
				Err(err).
				Msgf("error publishing heartbeat for %s", w.id)
		}
		select {
		case <-w.stop:
			return
		case <-time.After(tork.HEARTBEAT_RATE):
		}
	}
}

func (w *Worker) Start() error {
	log.Info().Msgf("starting worker %s", w.id)
	if err := w.api.start(); err != nil {
		return err
	}
	// subscribe for a private queue for the node
	if err := w.broker.SubscribeForTasks(fmt.Sprintf("%s%s", mq.QUEUE_EXCLUSIVE_PREFIX, w.id), w.cancelTask); err != nil {
		return errors.Wrapf(err, "error subscribing for queue: %s", w.id)
	}
	// subscribe to shared work queues
	for qname, concurrency := range w.queues {
		if !mq.IsWorkerQueue(qname) {
			continue
		}
		for i := 0; i < concurrency; i++ {
			err := w.broker.SubscribeForTasks(qname, w.handleTask)
			if err != nil {
				return errors.Wrapf(err, "error subscribing for queue: %s", qname)
			}
		}
	}
	go w.sendHeartbeats()
	return nil
}

func (w *Worker) Stop() error {
	log.Debug().Msgf("shutting down worker %s", w.id)
	w.stop <- 1
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := w.broker.Shutdown(ctx); err != nil {
		return errors.Wrapf(err, "error shutting down broker")
	}
	if err := w.api.shutdown(ctx); err != nil {
		return errors.Wrapf(err, "error shutting down worker %s", w.id)
	}
	return nil
}
