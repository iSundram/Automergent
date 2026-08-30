// Package workflow implements a deterministic multi-agent pipeline engine.
//
// A workflow is a declarative YAML spec of steps. Steps reference each
// other's outputs, run in dependency order, execute in parallel up to a
// concurrency cap, and are journaled: every agent call is keyed by a
// deterministic hash of its prompt and parameters, so an interrupted run
// resumes by replaying journaled results instead of re-running agents.
package workflow

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Step is one agent invocation in the pipeline.
type Step struct {
	ID string `yaml:"id"`
	// Prompt is the instruction; ${other-step} expands to that step's
	// output, $ARGUMENTS to the run arguments.
	Prompt    string   `yaml:"prompt"`
	DependsOn []string `yaml:"dependsOn"`
	// AgentType selects the subagent kind ("explore", "general-purpose", …);
	// empty = general-purpose.
	AgentType string `yaml:"agentType"`
	// Model overrides the subagent model; empty = default.
	Model string `yaml:"model"`
}

// Spec is a parsed workflow definition.
type Spec struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Budget caps total output tokens across the run; 0 = unlimited.
	Budget int `yaml:"budget"`
	// Concurrency caps parallel steps; 0 = DefaultConcurrency.
	Concurrency int    `yaml:"concurrency"`
	Steps       []Step `yaml:"steps"`
}

// DefaultConcurrency bounds parallel agent calls when the spec sets none.
const DefaultConcurrency = 4

// ParseSpec decodes and validates a workflow definition.
func ParseSpec(data []byte) (*Spec, error) {
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse workflow yaml: %w", err)
	}
	if s.Name == "" {
		return nil, fmt.Errorf("workflow needs a name")
	}
	if len(s.Steps) == 0 {
		return nil, fmt.Errorf("workflow %q has no steps", s.Name)
	}
	seen := map[string]bool{}
	for i, st := range s.Steps {
		if st.ID == "" {
			return nil, fmt.Errorf("step %d needs an id", i)
		}
		if st.Prompt == "" {
			return nil, fmt.Errorf("step %q needs a prompt", st.ID)
		}
		if seen[st.ID] {
			return nil, fmt.Errorf("duplicate step id %q", st.ID)
		}
		seen[st.ID] = true
	}
	for _, st := range s.Steps {
		for _, dep := range st.DependsOn {
			if !seen[dep] {
				return nil, fmt.Errorf("step %q depends on unknown step %q", st.ID, dep)
			}
		}
	}
	if err := detectCycle(s.Steps); err != nil {
		return nil, err
	}
	if s.Concurrency <= 0 {
		s.Concurrency = DefaultConcurrency
	}
	return &s, nil
}

// detectCycle rejects dependency cycles (a workflow must be a DAG).
func detectCycle(steps []Step) error {
	byID := map[string]*Step{}
	for i := range steps {
		byID[steps[i].ID] = &steps[i]
	}
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[string]int{}
	var visit func(*Step) error
	visit = func(st *Step) error {
		switch color[st.ID] {
		case grey:
			return fmt.Errorf("dependency cycle through step %q", st.ID)
		case black:
			return nil
		}
		color[st.ID] = grey
		for _, dep := range st.DependsOn {
			if err := visit(byID[dep]); err != nil {
				return err
			}
		}
		color[st.ID] = black
		return nil
	}
	for i := range steps {
		if err := visit(&steps[i]); err != nil {
			return err
		}
	}
	return nil
}

// expandPrompt substitutes ${step} references and $ARGUMENTS in a step
// prompt using completed outputs.
func expandPrompt(prompt, arguments string, outputs map[string]string) string {
	out := prompt
	for id, output := range outputs {
		out = strings.ReplaceAll(out, "${"+id+"}", output)
	}
	return strings.ReplaceAll(out, "$ARGUMENTS", arguments)
}
