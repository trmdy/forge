package workflows

import (
	"fmt"
	"sort"
	"strings"
)

var validStepTypes = map[StepType]struct{}{
	StepTypeAgent:    {},
	StepTypeLoop:     {},
	StepTypeBash:     {},
	StepTypeLogic:    {},
	StepTypeJob:      {},
	StepTypeWorkflow: {},
	StepTypeHuman:    {},
}

// ValidateWorkflow validates and normalizes a workflow.
func ValidateWorkflow(wf *Workflow) (*Workflow, error) {
	if wf == nil {
		list := &ErrorList{}
		list.Add(WorkflowError{Code: ErrCodeMissingField, Message: "workflow is required"})
		return nil, list
	}

	NormalizeWorkflow(wf)
	path := wf.Source
	list := &ErrorList{}

	if wf.Name == "" {
		list.Add(WorkflowError{
			Code:    ErrCodeMissingField,
			Message: "name is required",
			Path:    path,
			Field:   "name",
		})
	}

	if len(wf.Steps) == 0 {
		list.Add(WorkflowError{
			Code:    ErrCodeMissingField,
			Message: "steps are required",
			Path:    path,
			Field:   "steps",
		})
	}

	stepIndex := make(map[string]int)
	for i := range wf.Steps {
		step := &wf.Steps[i]
		index := i + 1

		if step.ID == "" {
			list.Add(WorkflowError{
				Code:    ErrCodeMissingField,
				Message: "step id is required",
				Path:    path,
				Field:   "steps.id",
				Index:   index,
			})
		} else if prev, exists := stepIndex[step.ID]; exists {
			list.Add(WorkflowError{
				Code:    ErrCodeDuplicateStep,
				Message: fmt.Sprintf("duplicate step id %q (also in step %d)", step.ID, prev),
				Path:    path,
				StepID:  step.ID,
				Field:   "steps.id",
				Index:   index,
			})
		} else {
			stepIndex[step.ID] = index
		}

		if step.Type == "" {
			list.Add(WorkflowError{
				Code:    ErrCodeMissingField,
				Message: "step type is required",
				Path:    path,
				StepID:  step.ID,
				Field:   "steps.type",
				Index:   index,
			})
		} else if _, ok := validStepTypes[step.Type]; !ok {
			list.Add(WorkflowError{
				Code:    ErrCodeUnknownType,
				Message: fmt.Sprintf("unknown step type %q", step.Type),
				Path:    path,
				StepID:  step.ID,
				Field:   "steps.type",
				Index:   index,
			})
		}

		validateStepSpecificFields(step, index, path, list)
		validateStopCondition(step, index, path, list)
		validateDependencies(step, index, path, list)
	}

	validateDependencyTargets(wf, stepIndex, list)
	validateLogicTargets(wf, stepIndex, list)
	validateCycles(wf, stepIndex, list)

	if list.Empty() {
		return wf, nil
	}
	return wf, list
}

func validateStepSpecificFields(step *WorkflowStep, index int, path string, list *ErrorList) {
	switch step.Type {
	case StepTypeAgent:
		validatePromptFields(step, index, path, list)
	case StepTypeLoop:
		validatePromptFields(step, index, path, list)
	case StepTypeBash:
		if step.Cmd == "" {
			list.Add(missingFieldError(path, step.ID, index, "cmd"))
		}
	case StepTypeLogic:
		if step.If == "" {
			list.Add(missingFieldError(path, step.ID, index, "if"))
		}
		if len(step.Then) == 0 && len(step.Else) == 0 {
			list.Add(WorkflowError{
				Code:    ErrCodeMissingField,
				Message: "logic step must define then or else targets",
				Path:    path,
				StepID:  step.ID,
				Field:   "then",
				Index:   index,
			})
		}
	case StepTypeJob:
		if step.JobName == "" {
			list.Add(missingFieldError(path, step.ID, index, "job_name"))
		}
	case StepTypeWorkflow:
		if step.WorkflowName == "" {
			list.Add(missingFieldError(path, step.ID, index, "workflow_name"))
		}
	case StepTypeHuman:
		validatePromptFields(step, index, path, list)
	}
}

func validatePromptFields(step *WorkflowStep, index int, path string, list *ErrorList) {
	if step.Prompt == "" && step.PromptPath == "" && step.PromptName == "" {
		list.Add(WorkflowError{
			Code:    ErrCodeMissingField,
			Message: "prompt, prompt_path, or prompt_name is required",
			Path:    path,
			StepID:  step.ID,
			Field:   "prompt",
			Index:   index,
		})
	}
}

func validateStopCondition(step *WorkflowStep, index int, path string, list *ErrorList) {
	if step.Stop == nil {
		return
	}

	if step.Stop.Expr == "" && step.Stop.Tool == nil && step.Stop.LLM == nil {
		list.Add(WorkflowError{
			Code:    ErrCodeMissingField,
			Message: "stop condition requires expr, tool, or llm",
			Path:    path,
			StepID:  step.ID,
			Field:   "stop",
			Index:   index,
		})
	}

	if step.Stop.Tool != nil && step.Stop.Tool.Name == "" {
		list.Add(missingFieldError(path, step.ID, index, "stop.tool.name"))
	}

	if step.Stop.LLM != nil && step.Stop.LLM.Rubric == "" && step.Stop.LLM.PassIf == "" {
		list.Add(WorkflowError{
			Code:    ErrCodeMissingField,
			Message: "stop.llm requires rubric or pass_if",
			Path:    path,
			StepID:  step.ID,
			Field:   "stop.llm",
			Index:   index,
		})
	}
}

func validateDependencies(step *WorkflowStep, index int, path string, list *ErrorList) {
	seen := make(map[string]struct{})
	for _, dep := range step.DependsOn {
		if dep == "" {
			list.Add(WorkflowError{
				Code:    ErrCodeInvalidField,
				Message: "depends_on entries must be non-empty",
				Path:    path,
				StepID:  step.ID,
				Field:   "depends_on",
				Index:   index,
			})
			continue
		}
		if dep == step.ID && step.ID != "" {
			list.Add(WorkflowError{
				Code:    ErrCodeInvalidField,
				Message: "step cannot depend on itself",
				Path:    path,
				StepID:  step.ID,
				Field:   "depends_on",
				Index:   index,
			})
		}
		if _, exists := seen[dep]; exists {
			list.Add(WorkflowError{
				Code:    ErrCodeInvalidField,
				Message: fmt.Sprintf("duplicate dependency %q", dep),
				Path:    path,
				StepID:  step.ID,
				Field:   "depends_on",
				Index:   index,
			})
			continue
		}
		seen[dep] = struct{}{}
	}
}

func validateDependencyTargets(wf *Workflow, stepIndex map[string]int, list *ErrorList) {
	for i := range wf.Steps {
		step := &wf.Steps[i]
		index := i + 1
		for _, dep := range step.DependsOn {
			if dep == "" {
				continue
			}
			if _, exists := stepIndex[dep]; !exists {
				list.Add(WorkflowError{
					Code:    ErrCodeMissingStep,
					Message: fmt.Sprintf("unknown dependency %q", dep),
					Path:    wf.Source,
					StepID:  step.ID,
					Field:   "depends_on",
					Index:   index,
				})
			}
		}
	}
}

func validateLogicTargets(wf *Workflow, stepIndex map[string]int, list *ErrorList) {
	for i := range wf.Steps {
		step := &wf.Steps[i]
		if step.Type != StepTypeLogic {
			continue
		}
		index := i + 1
		for _, target := range append(step.Then, step.Else...) {
			if target == "" {
				continue
			}
			if _, exists := stepIndex[target]; !exists {
				list.Add(WorkflowError{
					Code:    ErrCodeMissingStep,
					Message: fmt.Sprintf("unknown logic target %q", target),
					Path:    wf.Source,
					StepID:  step.ID,
					Field:   "then",
					Index:   index,
				})
			}
		}
	}
}

func validateCycles(wf *Workflow, stepIndex map[string]int, list *ErrorList) {
	if len(stepIndex) == 0 {
		return
	}

	adj := make(map[string][]string)
	inDegree := make(map[string]int)
	for id := range stepIndex {
		inDegree[id] = 0
	}

	for _, step := range wf.Steps {
		if step.ID == "" {
			continue
		}
		for _, dep := range step.DependsOn {
			if _, ok := inDegree[dep]; !ok {
				continue
			}
			adj[dep] = append(adj[dep], step.ID)
			inDegree[step.ID]++
		}
	}

	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	processed := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		processed++
		for _, next := range adj[id] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if processed == len(inDegree) {
		return
	}

	cycle := make([]string, 0)
	for id, deg := range inDegree {
		if deg > 0 {
			cycle = append(cycle, id)
		}
	}
	if len(cycle) == 0 {
		return
	}
	sort.Strings(cycle)

	list.Add(WorkflowError{
		Code:    ErrCodeCycle,
		Message: fmt.Sprintf("cycle detected among steps: %s", strings.Join(cycle, ", ")),
		Path:    wf.Source,
	})
}

func missingFieldError(path, stepID string, index int, field string) WorkflowError {
	return WorkflowError{
		Code:    ErrCodeMissingField,
		Message: fmt.Sprintf("%s is required", field),
		Path:    path,
		StepID:  stepID,
		Field:   field,
		Index:   index,
	}
}
