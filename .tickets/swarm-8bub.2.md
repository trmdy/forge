---
id: swarm-8bub.2
status: closed
deps: []
links: []
created: 2025-12-27T07:06:33.5602076+01:00
type: task
priority: 1
parent: swarm-8bub
---
# Implement sequence storage and parsing

Create the core sequence package for loading and parsing sequences.

## Package: internal/sequences

### Types
```go
type Sequence struct {
    Name        string         `yaml:"name"`
    Description string         `yaml:"description"`
    Steps       []SequenceStep `yaml:"steps"`
    Variables   []SequenceVar  `yaml:"variables,omitempty"`
    Tags        []string       `yaml:"tags,omitempty"`
    Source      string         // file path or "builtin"
}

type SequenceStep struct {
    Type     StepType `yaml:"type"` // message, pause, conditional
    Content  string   `yaml:"content,omitempty"`
    Duration string   `yaml:"duration,omitempty"` // for pause
    When     string   `yaml:"when,omitempty"`     // for conditional: idle, queue-empty
    Reason   string   `yaml:"reason,omitempty"`   // description
}

type StepType string
const (
    StepTypeMessage     StepType = "message"
    StepTypePause       StepType = "pause"
    StepTypeConditional StepType = "conditional"
)
```

### Functions
- LoadSequence(path string) (*Sequence, error)
- LoadSequencesFromDir(dir string) ([]*Sequence, error)
- RenderSequence(s *Sequence, vars map[string]string) ([]QueueItem, error)

### Sequence search paths
Same as templates:
1. .swarm/sequences/ (project)
2. ~/.config/swarm/sequences/ (user)
3. Built-in sequences (embedded)

### Built-in sequences
- bugfix: Find bug → fix → test → commit
- feature: Implement → test → document → commit
- review-loop: Review → address feedback → re-review


