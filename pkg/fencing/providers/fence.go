package fencing

import (
	"fmt"
	"time"
)

type Action uint8

// Available actions
const (
	// No Action.
	None Action = iota
	// PowerOn Machine, disable port access etc...
	On
	// PowerOff Machine, disable port access etc...
	Off
	// Reboot Machine.
	Reboot
	// Get Machime, port etc... status
	Status
	// List available "ports". A port can be a virtual machine, a power
	// port, a switch port etc... managed by this fence devices
	List
	// Check the health of the fence device itself
	Monitor
)

// Action internal value to name translation. Used for logging and errors.
var ActionMap = map[Action]string{
	None:    "none",
	On:      "on",
	Off:     "off",
	Reboot:  "reboot",
	Status:  "status",
	List:    "list",
	Monitor: "monitor",
}

type DeviceStatus uint8

const (
	Ok DeviceStatus = iota
	Ko
)

type PortList []PortName

type PortName struct {
	Name  string
	Alias string
}

type Fence struct {
	providers map[string]FenceProvider
}

func CreateNewFence() *Fence {
	return &Fence{providers: make(map[string]FenceProvider)}
}

// Register the specified fence provider.
func (f *Fence) RegisterProvider(name string, fenceProvider FenceProvider) error {
	if _, dup := f.providers[name]; dup {
		return fmt.Errorf("Provider \"%s\" already registered", name)
	}
	f.providers[name] = fenceProvider
	return nil
}

// Deregister the specified fence provider.
func (f *Fence) DeregisterProvider(name string) error {
	if _, ok := f.providers[name]; ok {
		return fmt.Errorf("Provider \"%s\" not registered", name)
	}
	delete(f.providers, name)
	return nil
}

// Load all agents of the registered providers.
// If timeout is non-zero than the loading will be stopped and an
// error will be returned.
func (f *Fence) LoadAgents(timeout time.Duration) error {
	t := time.Now()
	nexttimeout := 0 * time.Second
	for name, provider := range f.providers {
		if timeout != 0 {
			nexttimeout = timeout - time.Since(t)
		}
		err := provider.LoadAgents(nexttimeout)
		if err != nil {
			return fmt.Errorf("provider \"%s\": %s", name, err)
		}
	}
	return nil
}

// Returns a slice of registered provider's names
func (f *Fence) GetRegisteredProviders() []string {
	var keys []string
	for k := range f.providers {
		keys = append(keys, k)
	}
	return keys
}

// Returns the provider's agents
func (f *Fence) GetAgents(providerName string) (Agents, error) {
	provider, ok := f.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("Unknown provider \"%s\"", providerName)
	}
	return provider.GetAgents()
}

// Returns the requested provider's agent
func (f *Fence) GetAgent(providerName string, name string) (*Agent, error) {
	provider, ok := f.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("Unknown provider \"%s\"", providerName)
	}
	return provider.GetAgent(name)
}

// Run the specified agentConfig
// If action is None than the agent's default action, if specified, is executed
func (f *Fence) Run(agentConfig *AgentConfig, action Action, timeout time.Duration) error {
	provider, ok := f.providers[agentConfig.ProviderName]
	if !ok {
		return fmt.Errorf("Unknown provider \"%s\"", agentConfig.ProviderName)
	}
	//err := f.VerifyAgentConfig(agentConfig, true)
	//if err != nil {
	//	return err
	//}

	// Verify valid action
	a, err := f.GetAgent(agentConfig.ProviderName, agentConfig.Name)
	if err != nil {
		return err
	}
	if action == None {
		if a.DefaultAction == None {
			return fmt.Errorf("No specified default action")
		}
		action = a.DefaultAction
	} else {
		actionok := false
		for _, a := range a.Actions {
			if a == action {
				actionok = true
			}
		}
		if !actionok {
			return fmt.Errorf("Unsupported agent action: \"%s\"", ActionMap[action])
		}
	}

	return provider.Run(agentConfig, action, timeout)
}

// Executes On action
func (f *Fence) On(agentConfig *AgentConfig, timeout time.Duration) error {
	return f.Run(agentConfig, On, timeout)
}

// Executes Off action
func (f *Fence) Off(agentConfig *AgentConfig, timeout time.Duration) error {
	return f.Run(agentConfig, Off, timeout)
}

// Executes Reboot action
func (f *Fence) Reboot(agentConfig *AgentConfig, timeout time.Duration) error {
	return f.Run(agentConfig, Reboot, timeout)
}

// Executes Status action
func (f *Fence) Status(agentConfig *AgentConfig, timeout time.Duration) (DeviceStatus, error) {
	provider, ok := f.providers[agentConfig.ProviderName]
	if !ok {
		return 0, fmt.Errorf("Unknown provider \"%s\"", agentConfig.ProviderName)
	}
	err := f.VerifyAgentConfig(agentConfig, true)
	if err != nil {
		return 0, err
	}
	return provider.Status(agentConfig, timeout)
}

// Executes Status action
func (f *Fence) Monitor(agentConfig *AgentConfig, timeout time.Duration) (DeviceStatus, error) {
	provider, ok := f.providers[agentConfig.ProviderName]
	if !ok {
		return 0, fmt.Errorf("Unknown provider \"%s\"", agentConfig.ProviderName)
	}
	err := f.VerifyAgentConfig(agentConfig, false)
	if err != nil {
		return 0, err
	}
	return provider.Status(agentConfig, timeout)
}

// Executes List action
func (f *Fence) List(agentConfig *AgentConfig, timeout time.Duration) (PortList, error) {
	provider, ok := f.providers[agentConfig.ProviderName]
	if !ok {
		return nil, fmt.Errorf("Unknown provider \"%s\"", agentConfig.ProviderName)
	}
	err := f.VerifyAgentConfig(agentConfig, false)
	if err != nil {
		return nil, err
	}

	a, err := f.GetAgent(agentConfig.ProviderName, agentConfig.Name)
	if err != nil {
		return nil, err
	}
	if a.MultiplePorts == false {
		return nil, fmt.Errorf("Agent doesn't support multiple ports")
	}
	return provider.List(agentConfig, timeout)
}

func (f *Fence) VerifyAgentConfig(agentConfig *AgentConfig, checkPort bool) error {
	a, err := f.GetAgent(agentConfig.ProviderName, agentConfig.Name)
	if err != nil {
		return err
	}

	if checkPort && a.MultiplePorts && agentConfig.Port == "" {
		return fmt.Errorf("Port name required")

	}
	for n, values := range agentConfig.Parameters {
		ap, ok := a.Parameters[n]
		if !ok {
			return fmt.Errorf("Unknown parameter \"%s\"", n)
		}
		if len(values) == 0 {
			return fmt.Errorf("Empty parameter's values")
		}
		switch ap.ContentType {
		case Boolean:
			for _, v := range values {
				if _, ok := v.(bool); !ok {
					return fmt.Errorf("Parameter \"%s\" not of boolean type", n)
				}
			}
		case String:
			for _, v := range values {
				if _, ok := v.(string); !ok {
					return fmt.Errorf("Parameter \"%s\" not of string type", n)
				}
			}
		}
		if ap.HasOptions {
			for _, v := range values {
				found := false
				for _, option := range ap.Options {
					if v == option {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("Parameter value \"%v\" not found in possible choices", v)
				}
			}
		}
	}
	return nil
}

func ActionToString(action Action) string {
	switch action {
	case On:
		return "on"
	case Off:
		return "off"
	case Reboot:
		return "reboot"
	case Status:
		return "status"
	case List:
		return "list"
	case Monitor:
		return "monitor"
	}
	return ""
}
func StringToAction(action string) (Action, error) {
	switch action {
	case "on", "enable":
		return On, nil
	case "off", "disable":
		return Off, nil
	case "reboot":
		return Reboot, nil
	case "status":
		return Status, nil
	case "list":
		return List, nil
	case "monitor":
		return Monitor, nil
	}
	return 0, fmt.Errorf("Unknown fence action: %s", action)
}
