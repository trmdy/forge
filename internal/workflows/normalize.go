package workflows

import "strings"

// NormalizeWorkflow trims whitespace and normalizes step fields.
func NormalizeWorkflow(wf *Workflow) *Workflow {
	if wf == nil {
		return nil
	}

	wf.Name = strings.TrimSpace(wf.Name)
	wf.Version = strings.TrimSpace(wf.Version)
	wf.Description = strings.TrimSpace(wf.Description)

	for i := range wf.Steps {
		step := &wf.Steps[i]
		step.ID = strings.TrimSpace(step.ID)
		step.Name = strings.TrimSpace(step.Name)
		step.Type = StepType(strings.ToLower(strings.TrimSpace(string(step.Type))))
		step.DependsOn = normalizeStringSlice(step.DependsOn)
		step.AliveWith = normalizeStringSlice(step.AliveWith)
		step.Then = normalizeStringSlice(step.Then)
		step.Else = normalizeStringSlice(step.Else)
		step.When = strings.TrimSpace(step.When)
		step.Prompt = strings.TrimSpace(step.Prompt)
		step.PromptPath = strings.TrimSpace(step.PromptPath)
		step.PromptName = strings.TrimSpace(step.PromptName)
		step.Profile = strings.TrimSpace(step.Profile)
		step.Pool = strings.TrimSpace(step.Pool)
		step.MaxRuntime = strings.TrimSpace(step.MaxRuntime)
		step.Interval = strings.TrimSpace(step.Interval)
		step.Cmd = strings.TrimSpace(step.Cmd)
		step.Workdir = strings.TrimSpace(step.Workdir)
		step.If = strings.TrimSpace(step.If)
		step.JobName = strings.TrimSpace(step.JobName)
		step.WorkflowName = strings.TrimSpace(step.WorkflowName)
		step.Timeout = strings.TrimSpace(step.Timeout)

		if step.Stop != nil {
			step.Stop.Expr = strings.TrimSpace(step.Stop.Expr)
			if step.Stop.Tool != nil {
				step.Stop.Tool.Name = strings.TrimSpace(step.Stop.Tool.Name)
			}
			if step.Stop.LLM != nil {
				step.Stop.LLM.Rubric = strings.TrimSpace(step.Stop.LLM.Rubric)
				step.Stop.LLM.PassIf = strings.TrimSpace(step.Stop.LLM.PassIf)
			}
		}

		if step.Hooks != nil {
			step.Hooks.Pre = normalizeStringSlice(step.Hooks.Pre)
			step.Hooks.Post = normalizeStringSlice(step.Hooks.Post)
		}
	}

	if wf.Hooks != nil {
		wf.Hooks.Pre = normalizeStringSlice(wf.Hooks.Pre)
		wf.Hooks.Post = normalizeStringSlice(wf.Hooks.Post)
	}

	return wf
}

func normalizeStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}

	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}
