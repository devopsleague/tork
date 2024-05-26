package input

import (
	"github.com/runabol/tork"
)

type Service struct {
	Name        string            `json:"name,omitempty" yaml:"name,omitempty" validate:"servicename,required,max=32"`
	Namespace   string            `json:"namespace,omitempty" yaml:"namespace,omitempty" validate:"servicename,max=32"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Ports       []Port            `json:"ports,omitempty" yaml:"ports,omitempty" validate:"required"`
	Probe       *Probe            `json:"probe,omitempty" yaml:"probe,omitempty" validate:"required"`
	Run         string            `json:"run,omitempty" yaml:"run,omitempty"`
	Image       string            `json:"image,omitempty" yaml:"image,omitempty" validate:"required"`
	Registry    *Registry         `json:"registry,omitempty" yaml:"registry,omitempty"`
	Env         map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Files       map[string]string `json:"files,omitempty" yaml:"files,omitempty"`
	Queue       string            `json:"queue,omitempty" yaml:"queue,omitempty" validate:"queue"`
}

type Probe struct {
	Path     string `json:"path,omitempty" yaml:"path,omitempty" validate:"required"`
	Interval string `json:"interval,omitempty" yaml:"interval,omitempty"`
}

type Port struct {
	Port string `json:"port,omitempty" yaml:"port,omitempty" validate:"required"`
}

func (i *Service) ToService() *tork.Service {
	ports := make([]*tork.Port, len(i.Ports))
	for i, p := range i.Ports {
		ports[i] = &tork.Port{
			Port: p.Port,
		}
	}
	return &tork.Service{
		Name:      i.Name,
		Namespace: i.Namespace,
		Run:       i.Run,
		Image:     i.Image,
		Env:       i.Env,
		Files:     i.Files,
		Queue:     i.Queue,
		Ports:     ports,
		Probe: &tork.Probe{
			Path:     i.Probe.Path,
			Interval: i.Probe.Interval,
		},
	}
}
