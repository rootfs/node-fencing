package fencing

import "fmt"

type AgentConfig struct {
	ProviderName string
	Name         string
	Port         string
	Parameters   map[string][]interface{}
}

func NewAgentConfig(providerName string, name string) *AgentConfig {
	return &AgentConfig{ProviderName: providerName, Name: name, Parameters: make(map[string][]interface{})}
}

func (ac *AgentConfig) SetPort(port string) {
	ac.Port = port

}
func (ac *AgentConfig) SetParameter(name string, value interface{}) {
	ac.Parameters[name] = append(ac.Parameters[name], value)
}

func (ac *AgentConfig) Parameter(name string) ([]interface{}, error) {
	value, ok := ac.Parameters[name]
	if !ok {
		return nil, fmt.Errorf("Parameter \"%s\"not defined", name)
	}
	return value, nil
}
