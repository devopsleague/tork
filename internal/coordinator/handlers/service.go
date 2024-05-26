package handlers

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/runabol/tork"
	"github.com/runabol/tork/datastore"
	"github.com/runabol/tork/internal/uuid"
	"github.com/runabol/tork/middleware/job"
	"github.com/runabol/tork/middleware/service"
	"github.com/runabol/tork/mq"
)

type serviceHandler struct {
	ds     datastore.Datastore
	broker mq.Broker
	onJob  job.HandlerFunc
}

func NewServiceHandler(ds datastore.Datastore, b mq.Broker) service.HandlerFunc {
	h := &serviceHandler{
		ds:     ds,
		broker: b,
		onJob:  NewJobHandler(ds, b),
	}
	return h.handle
}

func (h *serviceHandler) handle(ctx context.Context, et service.EventType, s *tork.Service) error {
	switch s.State {
	case tork.ServiceStatePending:
		return h.startService(ctx, s)
	default:
		return errors.Errorf("invalid service state: %s", s.State)
	}
}

func (h *serviceHandler) startService(ctx context.Context, s *tork.Service) error {
	log.Debug().Msgf("starting service %s", s.ID)
	now := time.Now().UTC()
	ports := make(map[string]*tork.Port, 0)
	for _, p := range s.Ports {
		ports[p.Port] = p
	}
	tasks := []*tork.Task{{
		Name:  s.Name,
		Image: s.Image,
		Run:   s.Run,
		Files: maps.Clone(s.Files),
		Queue: s.Queue,
		Env:   maps.Clone(s.Env),
		Probe: s.Probe.Clone(),
		Ports: ports,
	}}
	j := &tork.Job{
		ID:        uuid.NewUUID(),
		Name:      fmt.Sprintf("Initial deployment of %s", s.Name),
		ServiceID: &s.ID,
		State:     tork.JobStatePending,
		CreatedAt: now,
		TaskCount: len(tasks),
		Tasks:     tasks,
	}
	if err := h.ds.CreateJob(ctx, j); err != nil {
		return err
	}
	if err := h.ds.UpdateService(ctx, s.Namespace, s.Name, func(u *tork.Service) error {
		u.State = tork.ServiceStateScheduled
		return nil
	}); err != nil {
		return err
	}
	return h.onJob(ctx, job.StateChange, j)
}
