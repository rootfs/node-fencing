package fencing

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fence-executor/utils"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type RHAgentProvider struct {
	config *RHAgentProviderConfig
	Agents map[string]*RHAgent
}

type RHAgentProviderConfig struct {
	Glob string
}

type RHAgent struct {
	Command string
	*utils.Agent
}

func newRHAgent() *RHAgent {
	return &RHAgent{Agent: utils.NewAgent()}
}

const (
	defaultGlob = "/usr/sbin/fence_*"
)

var defaultConfig = &RHAgentProviderConfig{Glob: defaultGlob}

func CreateRHProvider(config *RHAgentProviderConfig) *RHAgentProvider {
	p := &RHAgentProvider{Agents: make(map[string]*RHAgent)}
	if config != nil {
		p.config = config
	} else {
		p.config = defaultConfig
	}
	return p
}

type RHResourceAgent struct {
	Command    string
	XMLName    xml.Name      `xml:"resource-agent"`
	Name       string        `xml:"name,attr"`
	ShortDesc  string        `xml:"shortdesc,attr"`
	LongDesc   string        `xml:"longdesc"`
	VendorUrl  string        `xml:"vendor-url"`
	Parameters []RHParameter `xml:"parameters>parameter"`
	Actions    []RHAction    `xml:"actions>action"`
}

type RHParameter struct {
	Name      string    `xml:"name,attr"`
	Unique    bool      `xml:"unique,attr"`
	Required  bool      `xml:"required,attr"`
	ShortDesc string    `xml:"shortdesc"`
	Content   RHContent `xml:"content"`
}

type RHContent struct {
	ContentType string             `xml:"type,attr"`
	Default     string             `xml:"default,attr"`
	Options     []RHContentOptions `xml:"option"`
}

type RHContentOptions struct {
	Value string `xml:"value,attr"`
}

type RHAction struct {
	Name      string `xml:"name,attr"`
	OnTarget  string `xml:"on_target,attr"`
	Automatic string `xml:"automatic,attr"`
}

func ParseMetadata(mdxml []byte) (*RHResourceAgent, error) {
	v := &RHResourceAgent{}
	err := xml.Unmarshal(mdxml, v)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (r *RHResourceAgent) ToResourceAgent() (*RHAgent, error) {
	a := newRHAgent()

	a.Command = r.Command
	a.Name = r.Name
	a.ShortDesc = r.ShortDesc
	a.LongDesc = r.LongDesc

	for _, mdp := range r.Parameters {
		// If "action" parameter ignore it and set agent's DefaultAction
		if mdp.Name == "action" && mdp.Content.Default != "" {
			fa, err := utils.StringToAction(mdp.Content.Default)
			if err != nil {
				// Ignore bad default action
			} else {
				a.DefaultAction = fa
			}
			continue
		}
		// If "port" parameter ignore it and set agent's DefaultAction
		if mdp.Name == "port" {
			a.MultiplePorts = true
			continue
		}
		// If "port" parameter ignore it and set agent's DefaultAction
		if mdp.Name == "separator" {
			continue
		}
		// TODO. All the metadatas reports unique = "0" but I think they should be unique...
		p := &utils.Parameter{Name: mdp.Name, Unique: mdp.Unique, Required: mdp.Required, Desc: mdp.ShortDesc}
		switch mdp.Content.ContentType {
		case "boolean":
			p.ContentType = utils.Boolean
			if mdp.Content.Default != "" {
				value, err := strconv.ParseBool(mdp.Content.Default)
				if err != nil {
					return nil, err
				}
				p.Default = value
			}
		case "string":
			p.ContentType = utils.String
			if mdp.Content.Default != "" {
				p.Default = mdp.Content.Default
			}
		case "select":
			p.HasOptions = true
			p.ContentType = utils.String
			if mdp.Content.Default != "" {
				p.Default = mdp.Content.Default
			}
		default:
			return nil, fmt.Errorf("Agent: %s, parameter: %s. Wrong content type: %s", a.Name, p.Name, mdp.Content.ContentType)
		}
		for _, o := range mdp.Content.Options {
			p.Options = append(p.Options, o.Value)
		}
		a.Parameters[p.Name] = p
	}
	for _, mda := range r.Actions {
		if mda.Name == "on" {
			if mda.Automatic == "1" {
				a.UnfenceAction = utils.On
			}
			if mda.OnTarget == "1" {
				a.UnfenceOnTarget = true
			}
		}
		fa, err := utils.StringToAction(mda.Name)
		if err != nil {
			// Ignore unknown action
			continue
		}
		a.Actions = append(a.Actions, fa)
	}
	return a, nil
}

func (p *RHAgentProvider) LoadAgents(timeout time.Duration) error {
	p.Agents = make(map[string]*RHAgent)

	files, err := filepath.Glob(p.config.Glob)
	if err != nil {
		return err
	}
	t := time.Now()
	nexttimeout := 0 * time.Second

	// TODO Detected duplicate agents? (agents returning the same name in metadata)
	for _, file := range files {
		if timeout != 0 {
			nexttimeout = timeout - time.Since(t)
			if nexttimeout < 0 {
				return errors.New("timeout")
			}
		}
		a, err := p.LoadAgent(file, nexttimeout)
		if err != nil {
			continue
		}
		p.Agents[a.Name] = a
	}
	return nil
}

func (p *RHAgentProvider) LoadAgent(file string, timeout time.Duration) (*RHAgent, error) {
	cmd := exec.Command(file, "-o", "metadata")
	var b bytes.Buffer
	cmd.Stdout = &b
	err := cmd.Start()
	if err != nil {
		return nil, err
	}
	if timeout == time.Duration(0) {
		err = cmd.Wait()
	} else {
		err = utils.WaitTimeout(cmd, timeout)
	}
	if err != nil {
		return nil, err
	}

	mdxml := b.Bytes()
	mda, err := ParseMetadata(mdxml)
	if err != nil {
		return nil, err
	}

	a, err := mda.ToResourceAgent()
	if err != nil {
		return nil, fmt.Errorf("Agent \"%s\": %s", mda.Name, err)
	}

	a.Command = file

	return a, nil
}

func (p *RHAgentProvider) getRHAgent(name string) (*RHAgent, error) {
	a, ok := p.Agents[name]
	if !ok {
		return nil, fmt.Errorf("Unknown agent: %s", name)
	}
	return a, nil
}

func (p *RHAgentProvider) GetAgents() (utils.Agents, error) {
	fagents := make(utils.Agents)
	for _, a := range p.Agents {
		fagents[a.Name] = a.Agent
	}
	return fagents, nil
}

func (p *RHAgentProvider) GetAgent(name string) (*utils.Agent, error) {
	a, ok := p.Agents[name]
	if !ok {
		return nil, fmt.Errorf("Unknown agent: %s", name)
	}
	return a.Agent, nil
}

func (p *RHAgentProvider) run(ac *utils.AgentConfig, action string, timeout time.Duration) ([]byte, error) {
	var (
		cmdOut []byte
		err    error
	)

	a, err := p.getRHAgent(ac.Name)
	if err != nil {
		return nil, err
	}
	cmdName := a.Command

	cmdArgs := []string{fmt.Sprintf("--action=%s", action)}

	for name, values := range ac.Parameters {
		for _, value := range values {
			cmdArgs = append(cmdArgs, fmt.Sprintf("%s=%s", name, value))
		}
	}
	fmt.Printf("\nrunning %s with args: %s\n", cmdName, cmdArgs)

	if cmdOut, err = exec.Command(cmdName, cmdArgs...).Output(); err != nil {
		fmt.Fprintln(os.Stderr, "There was an error: ", err)
		os.Exit(1)
	}
	fmt.Printf("Action was ended: %s\n", string(cmdOut))

	return cmdOut, nil
}

func (p *RHAgentProvider) Status(ac *utils.AgentConfig, timeout time.Duration) (utils.DeviceStatus, error) {
	_, err := p.run(ac, "status", timeout)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// Process exited with exit code != 0
			return utils.Ko, nil
		}
		return utils.Ko, err
	}

	return utils.Ok, nil
}

func (p *RHAgentProvider) Monitor(ac *utils.AgentConfig, timeout time.Duration) (utils.DeviceStatus, error) {
	_, err := p.run(ac, "monitor", timeout)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// Process exited with exit code != 0
			return utils.Ko, nil
		}
		return utils.Ko, err
	}

	return utils.Ok, nil
}

func (p *RHAgentProvider) List(ac *utils.AgentConfig, timeout time.Duration) (utils.PortList, error) {
	out, err := p.run(ac, "list", timeout)
	if err != nil {
		return nil, err
	}

	portList := make(utils.PortList, 0)
	reader := bufio.NewReader(bytes.NewReader(out))
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		var portName utils.PortName
		line := scanner.Text() // Println will add back the final '\n'
		split := strings.Split(line, ",")
		switch len(split) {
		case 1:
			portName = utils.PortName{Name: split[0]}
		case 2:
			portName = utils.PortName{Name: split[0], Alias: split[1]}
		default:
			return nil, fmt.Errorf("Wrong list format")
		}
		portList = append(portList, portName)
	}

	return portList, nil
}

func (p *RHAgentProvider) Run(ac *utils.AgentConfig, action utils.Action, timeout time.Duration) error {
	// Specify action only if action !- fence.None,
	// elsewhere the agent will run the default action
	var actionstr string
	if action != utils.None {
		actionstr = utils.ActionToString(action)
	}

	_, err := p.run(ac, actionstr, timeout)
	return err
}
