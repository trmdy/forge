// Package workflows defines workflow schemas and helpers.
package workflows

// StepType defines workflow step types.
type StepType string

const (
	StepTypeAgent    StepType = "agent"
	StepTypeLoop     StepType = "loop"
	StepTypeBash     StepType = "bash"
	StepTypeLogic    StepType = "logic"
	StepTypeJob      StepType = "job"
	StepTypeWorkflow StepType = "workflow"
	StepTypeHuman    StepType = "human"
)

// Workflow defines a workflow file model.
type Workflow struct {
	Name        string         `toml:"name"`
	Version     string         `toml:"version"`
	Description string         `toml:"description"`
	Inputs      map[string]any `toml:"inputs"`
	Outputs     map[string]any `toml:"outputs"`
	Steps       []WorkflowStep `toml:"steps"`
	Hooks       *WorkflowHooks `toml:"hooks"`
	Source      string         `toml:"-"`
}

// WorkflowStep defines a single step in a workflow.
type WorkflowStep struct {
	ID        string         `toml:"id"`
	Name      string         `toml:"name"`
	Type      StepType       `toml:"type"`
	DependsOn []string       `toml:"depends_on"`
	When      string         `toml:"when"`
	Inputs    map[string]any `toml:"inputs"`
	Outputs   map[string]any `toml:"outputs"`
	Stop      *StopCondition `toml:"stop"`
	Hooks     *WorkflowHooks `toml:"hooks"`
	AliveWith []string       `toml:"alive_with"`

	Prompt        string         `toml:"prompt"`
	PromptPath    string         `toml:"prompt_path"`
	PromptName    string         `toml:"prompt_name"`
	Profile       string         `toml:"profile"`
	Pool          string         `toml:"pool"`
	MaxRuntime    string         `toml:"max_runtime"`
	Interval      string         `toml:"interval"`
	MaxIterations int            `toml:"max_iterations"`
	Cmd           string         `toml:"cmd"`
	Workdir       string         `toml:"workdir"`
	If            string         `toml:"if"`
	Then          []string       `toml:"then"`
	Else          []string       `toml:"else"`
	JobName       string         `toml:"job_name"`
	WorkflowName  string         `toml:"workflow_name"`
	Params        map[string]any `toml:"params"`
	Timeout       string         `toml:"timeout"`
}

// WorkflowHooks defines pre/post hooks.
type WorkflowHooks struct {
	Pre  []string `toml:"pre"`
	Post []string `toml:"post"`
}

// StopCondition defines stop conditions for loop steps.
type StopCondition struct {
	Expr string    `toml:"expr"`
	Tool *StopTool `toml:"tool"`
	LLM  *StopLLM  `toml:"llm"`
}

// StopTool runs a tool to evaluate a stop condition.
type StopTool struct {
	Name string   `toml:"name"`
	Args []string `toml:"args"`
}

// StopLLM defines a rubric-based stop condition.
type StopLLM struct {
	Rubric string `toml:"rubric"`
	PassIf string `toml:"pass_if"`
}
