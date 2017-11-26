package fencing

type ContentType uint8

const (
	Boolean ContentType = iota
	String
	Select
)

type Agent struct {
	// Agent Name
	Name string

	// Short Description
	ShortDesc string

	// Long Description
	LongDesc string

	// Allowed Parameters
	Parameters map[string]*Parameter

	// The fencing device supports multiple ports (Virtal Machines, switch ports etc...)
	MultiplePorts bool

	// Default fence action
	DefaultAction Action

	// if not None the fence device requires unfencing
	UnfenceAction Action

	// Fence device requires unfencing to be executed on the target node.
	UnfenceOnTarget bool

	// Allowed Device Actions
	Actions []Action
}

type Parameter struct {
	// Parameter Name
	Name string

	// If false the parameter can be specified multiple times
	Unique bool

	// If true the parameter is required
	Required bool

	// Parameter description
	Desc string

	// Parameter Type
	ContentType ContentType

	// Default value. If nil no default values is defined.
	Default interface{}

	// If true the parameter's accepted values are provided by Options.
	HasOptions bool

	// Accepted parameter's values.
	Options []interface{}
}

type Agents map[string]*Agent

func NewAgent() *Agent {
	return &Agent{Parameters: make(map[string]*Parameter)}
}
