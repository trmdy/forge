---
id: swarm-8bub.1
status: closed
deps: []
links: []
created: 2025-12-27T07:06:17.634507913+01:00
type: task
priority: 1
parent: swarm-8bub
---
# Implement template storage and parsing

Create the core template package for loading and parsing templates.

## Package: internal/templates

### Types
```go
type Template struct {
    Name        string            `yaml:"name"`
    Description string            `yaml:"description"`
    Message     string            `yaml:"message"`
    Variables   []TemplateVar     `yaml:"variables,omitempty"`
    Tags        []string          `yaml:"tags,omitempty"`
    Source      string            // file path or "builtin"
}

type TemplateVar struct {
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
    Default     string `yaml:"default,omitempty"`
    Required    bool   `yaml:"required"`
}
```

### Functions
- LoadTemplate(path string) (*Template, error)
- LoadTemplatesFromDir(dir string) ([]*Template, error)
- RenderTemplate(t *Template, vars map[string]string) (string, error)

### Template search paths (in order)
1. .swarm/templates/ (project)
2. ~/.config/swarm/templates/ (user)
3. /usr/share/swarm/templates/ (system, optional)
4. Built-in templates (embedded)

### Variable substitution
Use Go template syntax: {{.var_name}}
Support default values: {{.var_name | default "value"}}

### Built-in templates
- continue: Resume current task
- explain: Ask agent to explain current state
- commit: Ask agent to commit changes
- test: Ask agent to run tests
- review: Request code review


