/*-
 * Copyright 2015 Grammarly, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"fmt"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/go-yaml/yaml"
)

// ErrNotRockerCompose error describing that given container was not likely
// to beinitialized by rocker-compose
type ErrNotRockerCompose struct {
	ContainerID string
}

// Error returns string error
func (err ErrNotRockerCompose) Error() string {
	return fmt.Sprintf("Expecting container %.12s to have label 'rocker-compose-config' to parse it", err.ContainerID)
}

// NewFromDocker produces an container spec object from a docker.Container given by go-dockerclient.
func NewFromDocker(apiContainer *docker.Container) (*Container, error) {
	yamlData, ok := apiContainer.Config.Labels["rocker-compose-config"]
	if !ok {
		return nil, ErrNotRockerCompose{apiContainer.ID}
	}

	container := &Container{}

	if err := yaml.Unmarshal([]byte(yamlData), container); err != nil {
		return nil, fmt.Errorf("Failed to parse YAML config for container %s, error: %s", apiContainer.Name, err)
	}

	if container.Labels != nil {
		for k := range container.Labels {
			if strings.HasPrefix(k, "rocker-compose-") {
				delete(container.Labels, k)
			}
		}
	}

	return container, nil
}

// GetAPIConfig as an opposite from NewFromDocker - it returns docker.Config that can be used
// to run containers through the docker api.
func (config *Container) GetAPIConfig() *docker.Config {
	// Copy simple values
	apiConfig := &docker.Config{
		Entrypoint: config.Entrypoint,
		Labels:     config.Labels,
	}
	if config.Cmd != nil {
		apiConfig.Cmd = config.Cmd
	}
	if config.Image != nil {
		apiConfig.Image = *config.Image
	}
	if config.Hostname != nil {
		apiConfig.Hostname = *config.Hostname
	}
	if config.Domainname != nil {
		apiConfig.Domainname = *config.Domainname
	}
	if config.Workdir != nil {
		apiConfig.WorkingDir = *config.Workdir
	}
	if config.User != nil {
		apiConfig.User = *config.User
	}
	if config.Memory != nil {
		apiConfig.Memory = config.Memory.Int64()
	}
	if config.MemorySwap != nil {
		apiConfig.MemorySwap = config.MemorySwap.Int64()
	}
	if config.CpusetCpus != nil {
		apiConfig.CPUSet = *config.CpusetCpus
	}
	if config.CPUShares != nil {
		apiConfig.CPUShares = *config.CPUShares
	}
	if config.NetworkDisabled != nil {
		apiConfig.NetworkDisabled = *config.NetworkDisabled
	}

	// expose
	if len(config.Expose) > 0 || len(config.Ports) > 0 {
		apiConfig.ExposedPorts = map[docker.Port]struct{}{}
		for _, portBinding := range config.Expose {
			port := (docker.Port)(portBinding)
			apiConfig.ExposedPorts[port] = struct{}{}
		}
		// expose publised ports as well
		for _, configPort := range config.Ports {
			port := (docker.Port)(configPort.Port)
			apiConfig.ExposedPorts[port] = struct{}{}
		}
	}

	// env
	if config.Env != nil {
		apiConfig.Env = []string{}
		for key, val := range config.Env {
			apiConfig.Env = append(apiConfig.Env, fmt.Sprintf("%s=%s", key, val))
		}
	}

	// volumes
	if config.Volumes != nil {
		hostVolumes := map[string]struct{}{}
		for _, volume := range config.Volumes {
			if !strings.Contains(volume, ":") {
				hostVolumes[volume] = struct{}{}
			}
		}
		if len(hostVolumes) > 0 {
			apiConfig.Volumes = hostVolumes
		}
	}

	// TODO: SecurityOpts, OnBuild ?

	return apiConfig
}

// GetAPIHostConfig as an opposite from NewFromDocker - it returns docker.HostConfig that can be used
// to run containers through the docker api.
func (config *Container) GetAPIHostConfig() *docker.HostConfig {
	// TODO: CapAdd, CapDrop, LxcConf, Devices, LogConfig, ReadonlyRootfs,
	//       SecurityOpt, CgroupParent, CPUQuota, CPUPeriod
	// TODO: where Memory and MemorySwap should go?
	hostConfig := &docker.HostConfig{
		DNS:           config.DNS,
		ExtraHosts:    config.AddHost,
		RestartPolicy: config.Restart.ToDockerAPI(),
		Memory:        config.Memory.Int64(),
		MemorySwap:    config.MemorySwap.Int64(),
		NetworkMode:   config.Net.String(),
	}

	// if state is "running", then restart policy sould be "always" by default
	if config.State.Bool() && config.Restart == nil {
		hostConfig.RestartPolicy = (&RestartPolicy{"always", 0}).ToDockerAPI()
	}

	if config.Pid != nil {
		hostConfig.PidMode = *config.Pid
	}
	if config.Uts != nil {
		hostConfig.UTSMode = *config.Uts
	}
	if config.CpusetCpus != nil {
		hostConfig.CPUSet = *config.CpusetCpus
	}

	// Binds
	binds := []string{}
	for _, volume := range config.Volumes {
		if strings.Contains(volume, ":") {
			binds = append(binds, volume)
		}
	}
	if len(binds) > 0 {
		hostConfig.Binds = binds
	}

	// Privileged
	if config.Privileged != nil {
		hostConfig.Privileged = *config.Privileged
	}

	// PublishAllPorts
	if config.PublishAllPorts != nil {
		hostConfig.PublishAllPorts = *config.PublishAllPorts
	}

	// PortBindings
	if len(config.Ports) > 0 {
		hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{}
		for _, configPort := range config.Ports {
			key := (docker.Port)(configPort.Port)
			binding := docker.PortBinding{
				HostIP:   configPort.HostIP,
				HostPort: configPort.HostPort,
			}
			if _, ok := hostConfig.PortBindings[key]; !ok {
				hostConfig.PortBindings[key] = []docker.PortBinding{}
			}
			hostConfig.PortBindings[key] = append(hostConfig.PortBindings[key], binding)
		}
	}

	// By default, use "json-file" logger (Docker's default)
	// and also setup log rotation
	if config.LogOpt == nil && config.LogDriver == nil {
		hostConfig.LogConfig.Type = "json-file"
		hostConfig.LogConfig.Config = map[string]string{
			"max-file": "5",
			"max-size": "100m",
		}
	}

	if config.LogDriver != nil {
		hostConfig.LogConfig.Type = *config.LogDriver
	}
	if config.LogOpt != nil {
		if hostConfig.LogConfig.Type == "" {
			hostConfig.LogConfig.Type = "json-file"
		}
		hostConfig.LogConfig.Config = config.LogOpt
	}

	// Links
	if len(config.Links) > 0 {
		hostConfig.Links = []string{}
		for _, link := range config.Links {
			hostConfig.Links = append(hostConfig.Links, link.String())
		}
	}

	// VolumesFrom
	if len(config.VolumesFrom) > 0 {
		hostConfig.VolumesFrom = []string{}
		for _, volume := range config.VolumesFrom {
			hostConfig.VolumesFrom = append(hostConfig.VolumesFrom, volume.String())
		}
	}

	// Ulimits
	if len(config.Ulimits) > 0 {
		hostConfig.Ulimits = []docker.ULimit{}
		for _, ulimit := range config.Ulimits {
			hostConfig.Ulimits = append(hostConfig.Ulimits, docker.ULimit{
				Name: ulimit.Name,
				Soft: ulimit.Soft,
				Hard: ulimit.Hard,
			})
		}
	}

	return hostConfig
}
