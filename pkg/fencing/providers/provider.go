package fencing

import (
	"time"
)

// FenceProvider holds and manage fence agents
type FenceProvider interface {
	// LoadAgents asks the provider to load all agents. For example it can
	// find fence agents on the localdisk and parse their metadata.
	// It should be called before trying to execute other actions or it
	// will be called by fence.LoadAgents.
	// It should be called only one time. Multiple calls or concurrent
	// calls effects are unspecified.
	// If timeout is non-zero than the loading will be stopped and an
	// error will be returned.
	LoadAgents(timeout time.Duration) error

	// GetAgents returns all the loaded agents.
	GetAgents() (Agents, error)

	// GetAgent returns the agent with the requested name or an error if it doesn't exists.
	GetAgent(name string) (*Agent, error)

	// Run executes the requested AgentConfig.
	// If action is FenceAction.None than the default action, if specified, will be executed.
	// The accepted actions are On, Off, Reboot
	// If timeout is non-zero than the agent execution will be stopped and an
	// error will be returned.
	Run(agentConfig *AgentConfig, action Action, timeout time.Duration) error

	// Returns the status of the device(s) managed by the fence device.
	Status(agentConfig *AgentConfig, timeout time.Duration) (DeviceStatus, error)

	// Returns the status of the fence device itself.
	Monitor(agentConfig *AgentConfig, timeout time.Duration) (DeviceStatus, error)

	// If the fence device supports multiple ports it returns a list of the
	// available ports.
	List(agentConfig *AgentConfig, timeout time.Duration) (PortList, error)
}
