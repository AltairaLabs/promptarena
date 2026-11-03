package arenaconfig

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Types a persona state variable can have.
const (
	PersonaStateInt    = "int"
	PersonaStateBool   = "bool"
	PersonaStateString = "string"
)

// Targets a persona transition's checks can read.
const (
	// PersonaTargetAgent checks the exchange: the persona's message and the
	// agent's replies to it.
	PersonaTargetAgent = "agent"
	// PersonaTargetPersona checks the persona's own message, presented to the
	// assertions as if it were the reply.
	PersonaTargetPersona = "persona"
)

// PersonaVarState is the template variable holding the persona's whole
// state, one "- name: value" line per variable; PersonaStateVarPrefix
// prefixes the variable holding each one, as in {{state.patience}}.
const (
	PersonaVarState       = "state"
	PersonaStateVarPrefix = "state."
)

// DefaultPersonaStopOutcome labels a stop_when condition without an outcome.
const DefaultPersonaStopOutcome = "stopped"

// PersonaStateVar declares one variable of a persona's state.
type PersonaStateVar struct {
	// Type is int, bool or string.
	Type string `json:"type" yaml:"type" jsonschema:"enum=int,enum=bool,enum=string"`
	// Initial is the value the variable starts a conversation with. Default
	// 0, false or "".
	Initial interface{} `json:"initial,omitempty" yaml:"initial,omitempty"`
	// Min is an int variable's lowest value; a transition that would go
	// below it stops there.
	Min *int `json:"min,omitempty" yaml:"min,omitempty"`
	// Max is an int variable's highest value; a transition that would go
	// above it stops there.
	Max *int `json:"max,omitempty" yaml:"max,omitempty"`
}

// PersonaTransition changes the persona's state after an exchange whose
// checks all pass.
type PersonaTransition struct {
	// When lists the assertions that, all passing on the exchange just run,
	// apply the transition. They see only that exchange.
	When []AssertionConfig `json:"when" yaml:"when"`
	// Target is what the assertions read: "agent" (the default) checks the
	// exchange's messages; "persona" checks the persona's own message, as if
	// it were the reply, so content assertions match what the persona said.
	Target string `json:"target,omitempty" yaml:"target,omitempty" jsonschema:"enum=agent,enum=persona"`
	// Set assigns values, each of its variable's type.
	Set map[string]interface{} `json:"set,omitempty" yaml:"set,omitempty"`
	// Add adds to int variables; a negative amount subtracts.
	Add map[string]int `json:"add,omitempty" yaml:"add,omitempty"`
}

// ChecksPersona reports whether the transition's checks read the persona's
// message.
func (t *PersonaTransition) ChecksPersona() bool {
	return t.Target == PersonaTargetPersona
}

// PersonaStopCondition ends the persona's self-play turn when its state
// matches. Every comparison given must hold.
type PersonaStopCondition struct {
	// State names the variable compared.
	State string `json:"state" yaml:"state"`
	// Equals matches a value of the variable's type.
	Equals interface{} `json:"equals,omitempty" yaml:"equals,omitempty"`
	// AtMost matches an int variable at or below the value.
	AtMost *int `json:"at_most,omitempty" yaml:"at_most,omitempty"`
	// AtLeast matches an int variable at or above the value.
	AtLeast *int `json:"at_least,omitempty" yaml:"at_least,omitempty"`
	// Outcome labels the stop in the report, e.g. abandoned. Default
	// "stopped".
	Outcome string `json:"outcome,omitempty" yaml:"outcome,omitempty"`
	// Fail fails the run when the condition stops the turn. By default the
	// stop is reported without failing.
	Fail bool `json:"fail,omitempty" yaml:"fail,omitempty"`
}

// OutcomeLabel returns the condition's outcome, or the default.
func (c *PersonaStopCondition) OutcomeLabel() string {
	if c.Outcome == "" {
		return DefaultPersonaStopOutcome
	}
	return c.Outcome
}

// Matches reports whether value, of the condition's variable, meets every
// comparison.
func (c *PersonaStopCondition) Matches(value interface{}) bool {
	if c.Equals != nil {
		want, ok := normalizeStateValue(c.Equals)
		if !ok || want != value {
			return false
		}
	}
	n, isInt := value.(int)
	if c.AtMost != nil && (!isInt || n > *c.AtMost) {
		return false
	}
	if c.AtLeast != nil && (!isInt || n < *c.AtLeast) {
		return false
	}
	return true
}

// HasState reports whether the persona declares a state.
func (p *UserPersonaPack) HasState() bool {
	return len(p.State) > 0
}

// InitialState returns each state variable's starting value.
func (p *UserPersonaPack) InitialState() map[string]interface{} {
	out := make(map[string]interface{}, len(p.State))
	for name, v := range p.State {
		out[name] = v.initialValue()
	}
	return out
}

// StateStrings returns every string value the persona's state can hold: its
// string initial values and the strings its transitions set. The persona is
// shown these through {{state}}, so they count as given to it.
func (p *UserPersonaPack) StateStrings() []string {
	var out []string
	for _, v := range p.State {
		if s, ok := v.Initial.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	for i := range p.Transitions {
		for _, raw := range p.Transitions[i].Set {
			if s, ok := raw.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// StateVars returns the template variables for state: {{state}} and one
// {{state.<name>}} per variable.
func (p *UserPersonaPack) StateVars(state map[string]interface{}) map[string]string {
	names := make([]string, 0, len(state))
	for name := range state {
		names = append(names, name)
	}
	sort.Strings(names)
	vars := make(map[string]string, len(state)+1)
	lines := make([]string, len(names))
	for i, name := range names {
		v := FormatStateValue(state[name])
		vars[PersonaStateVarPrefix+name] = v
		lines[i] = fmt.Sprintf("- %s: %s", name, v)
	}
	vars[PersonaVarState] = strings.Join(lines, "\n")
	return vars
}

// FormatStateValue renders a state value for a template.
func FormatStateValue(v interface{}) string {
	switch x := v.(type) {
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

// Apply returns state after the transition: Set values first, then Add,
// with int variables kept within their bounds.
func (p *UserPersonaPack) Apply(t *PersonaTransition, state map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(state))
	for k, v := range state {
		out[k] = v
	}
	for name, raw := range t.Set {
		if v, ok := normalizeStateValue(raw); ok {
			out[name] = p.State[name].clamp(v)
		}
	}
	for name, delta := range t.Add {
		if n, ok := out[name].(int); ok {
			out[name] = p.State[name].clamp(n + delta)
		}
	}
	return out
}

func (v PersonaStateVar) initialValue() interface{} {
	if init, ok := normalizeStateValue(v.Initial); ok {
		return v.clamp(init)
	}
	switch v.Type {
	case PersonaStateInt:
		return v.clamp(0)
	case PersonaStateBool:
		return false
	default:
		return ""
	}
}

func (v PersonaStateVar) clamp(value interface{}) interface{} {
	n, ok := value.(int)
	if !ok {
		return value
	}
	if v.Min != nil && n < *v.Min {
		n = *v.Min
	}
	if v.Max != nil && n > *v.Max {
		n = *v.Max
	}
	return n
}

// normalizeStateValue converts a decoded YAML or JSON value to int, bool or
// string. A whole float (how JSON decodes numbers) becomes an int.
func normalizeStateValue(raw interface{}) (interface{}, bool) {
	switch x := raw.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case uint64:
		if x > math.MaxInt {
			return nil, false
		}
		return int(x), true
	case float64:
		if x == math.Trunc(x) {
			return int(x), true
		}
		return nil, false
	case bool, string:
		return x, true
	default:
		return nil, false
	}
}

// stateValueType names the type of a normalized state value.
func stateValueType(v interface{}) string {
	switch v.(type) {
	case int:
		return PersonaStateInt
	case bool:
		return PersonaStateBool
	default:
		return PersonaStateString
	}
}

var reStateName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validatePersonaState checks the state declaration, transitions and stop
// conditions.
func validatePersonaState(p *UserPersonaPack) error {
	if !p.HasState() {
		if len(p.Transitions) > 0 || len(p.StopWhen) > 0 {
			return fmt.Errorf("persona %s: transitions and stop_when need a state", p.ID)
		}
		return nil
	}
	for name, v := range p.State {
		if err := validateStateVar(name, v); err != nil {
			return fmt.Errorf("persona %s: state.%s: %w", p.ID, name, err)
		}
	}
	for i := range p.Transitions {
		if err := p.validateTransition(&p.Transitions[i]); err != nil {
			return fmt.Errorf("persona %s: transitions[%d]: %w", p.ID, i, err)
		}
	}
	for i := range p.StopWhen {
		if err := p.validateStopCondition(&p.StopWhen[i]); err != nil {
			return fmt.Errorf("persona %s: stop_when[%d]: %w", p.ID, i, err)
		}
	}
	return nil
}

func validateStateVar(name string, v PersonaStateVar) error {
	if !reStateName.MatchString(name) {
		return fmt.Errorf("name must be letters, digits and underscores, not starting with a digit")
	}
	switch v.Type {
	case PersonaStateInt:
		if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
			return fmt.Errorf("min %d is greater than max %d", *v.Min, *v.Max)
		}
	case PersonaStateBool, PersonaStateString:
		if v.Min != nil || v.Max != nil {
			return fmt.Errorf("min and max apply only to an int")
		}
	default:
		return fmt.Errorf("type must be int, bool or string, got %q", v.Type)
	}
	if v.Initial != nil {
		init, ok := normalizeStateValue(v.Initial)
		if !ok || stateValueType(init) != v.Type {
			return fmt.Errorf("initial %v is not a %s", v.Initial, v.Type)
		}
		if n, isInt := init.(int); isInt && v.clamp(n) != n {
			return fmt.Errorf("initial %d is outside min/max", n)
		}
	}
	return nil
}

func (p *UserPersonaPack) validateTransition(t *PersonaTransition) error {
	if len(t.When) == 0 {
		return fmt.Errorf("when: at least one assertion is required")
	}
	if t.Target != "" && t.Target != PersonaTargetAgent && t.Target != PersonaTargetPersona {
		return fmt.Errorf("target must be %s or %s, got %q", PersonaTargetAgent, PersonaTargetPersona, t.Target)
	}
	if len(t.Set) == 0 && len(t.Add) == 0 {
		return fmt.Errorf("set or add is required")
	}
	for name, raw := range t.Set {
		decl, ok := p.State[name]
		if !ok {
			return fmt.Errorf("set.%s: not a declared state variable", name)
		}
		v, ok := normalizeStateValue(raw)
		if !ok || stateValueType(v) != decl.Type {
			return fmt.Errorf("set.%s: %v is not a %s", name, raw, decl.Type)
		}
	}
	for name := range t.Add {
		decl, ok := p.State[name]
		if !ok {
			return fmt.Errorf("add.%s: not a declared state variable", name)
		}
		if decl.Type != PersonaStateInt {
			return fmt.Errorf("add.%s: add applies only to an int", name)
		}
	}
	return nil
}

func (p *UserPersonaPack) validateStopCondition(c *PersonaStopCondition) error {
	decl, ok := p.State[c.State]
	if !ok {
		return fmt.Errorf("state %q is not a declared state variable", c.State)
	}
	if c.Equals == nil && c.AtMost == nil && c.AtLeast == nil {
		return fmt.Errorf("one of equals, at_most or at_least is required")
	}
	if c.Equals != nil {
		v, ok := normalizeStateValue(c.Equals)
		if !ok || stateValueType(v) != decl.Type {
			return fmt.Errorf("equals %v is not a %s", c.Equals, decl.Type)
		}
	}
	if (c.AtMost != nil || c.AtLeast != nil) && decl.Type != PersonaStateInt {
		return fmt.Errorf("at_most and at_least apply only to an int")
	}
	return nil
}
