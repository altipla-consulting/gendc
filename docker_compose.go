package main

import "fmt"

type dcFile struct {
	Services map[string]*dcService `json:"services"`
}

type dcService struct {
	Image      string            `json:"image"`
	Command    []string          `json:"command,omitempty"`
	Ports      []string          `json:"ports,omitempty"`
	StopSignal string            `json:"stop_signal,omitempty"`
	Env        map[string]string `json:"environment,omitempty"`
	Volumes    []string          `json:"volumes,omitempty"`
	User       string            `json:"user,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	DependsOn  []string          `json:"depends_on,omitempty"`
	EnvFile    []string          `json:"env_file,omitempty"`
}

func (svc *dcService) ExposePort(port int) {
	svc.MapPort(port, port)
}

func (svc *dcService) MapPort(outside, inside int) {
	svc.Ports = append(svc.Ports, fmt.Sprintf("%v:%v", inside, outside))
}

func (svc *dcService) AddVolume(source, dest string) {
	svc.Volumes = append(svc.Volumes, source+":"+dest)
}
