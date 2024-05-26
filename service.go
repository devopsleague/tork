package tork

import (
	"time"
)

// State defines the list of states that a
// service can be in, at any given moment.
type ServiceState string

const (
	ServiceStatePending   ServiceState = "PENDING"
	ServiceStateScheduled ServiceState = "SCHEDULED"
	ServiceStateRunning   ServiceState = "RUNNING"
	ServiceStateStopped   ServiceState = "STOPPED"
	ServiceStateFailed    ServiceState = "FAILED"
)

const (
	ServiceDefaultNamespace = "default"
)

// Service represents a set of managed, long-running tasks.
type Service struct {
	ID        string            `json:"id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Namespace string            `json:"namespace,omitempty"`
	State     ServiceState      `json:"state,omitempty"`
	CreatedAt time.Time         `json:"createdAt,omitempty"`
	Run       string            `json:"run,omitempty"`
	Image     string            `json:"image,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Files     map[string]string `json:"files,omitempty"`
	Queue     string            `json:"queue,omitempty"`
	Probe     *Probe            `json:"probe,omitempty"`
	Ports     []*Port           `json:"ports,omitempty"`
}

type Probe struct {
	Path     string `json:"path,omitempty" `
	Interval string `json:"interval,omitempty"`
}

func (s *Probe) Clone() *Probe {
	return &Probe{
		Path:     s.Path,
		Interval: s.Interval,
	}
}
