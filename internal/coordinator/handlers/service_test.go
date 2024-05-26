package handlers

import (
	"context"
	"testing"

	"github.com/runabol/tork"
	"github.com/runabol/tork/datastore/inmemory"
	"github.com/runabol/tork/internal/uuid"
	"github.com/runabol/tork/middleware/service"
	"github.com/runabol/tork/mq"
	"github.com/stretchr/testify/assert"
)

func Test_handleServices(t *testing.T) {
	ctx := context.Background()
	b := mq.NewInMemoryBroker()

	ds := inmemory.NewInMemoryDatastore()
	handler := NewServiceHandler(ds, b)
	assert.NotNil(t, handler)

	s1 := &tork.Service{
		ID:        uuid.NewUUID(),
		Name:      "test",
		Namespace: "default",
		Probe: &tork.Probe{
			Path: "/",
		},
		Ports: []*tork.Port{{
			Port: "8080",
		}},
		State: tork.ServiceStatePending,
	}

	err := ds.CreateService(ctx, s1)
	assert.NoError(t, err)

	err = handler(ctx, service.StateChange, s1)
	assert.NoError(t, err)

	s2, err := ds.GetService(ctx, s1.Namespace, s1.Name)
	assert.NoError(t, err)
	assert.Equal(t, tork.ServiceStateScheduled, s2.State)
}
