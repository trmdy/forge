package loop

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/tOgg1/forge/internal/db"
)

const defaultMemoryMaxChars = 6000

func buildLoopMemory(ctx context.Context, database *db.DB, loopID string, maxChars int) (string, error) {
	if database == nil || strings.TrimSpace(loopID) == "" {
		return "", nil
	}
	if maxChars <= 0 {
		maxChars = defaultMemoryMaxChars
	}

	workRepo := db.NewLoopWorkStateRepository(database)
	kvRepo := db.NewLoopKVRepository(database)

	kv, err := kvRepo.ListByLoop(ctx, loopID)
	if err != nil {
		return "", err
	}

	current, err := workRepo.GetCurrent(ctx, loopID)
	if err != nil && !errors.Is(err, db.ErrLoopWorkStateNotFound) {
		return "", err
	}
	recent, err := workRepo.ListByLoop(ctx, loopID, 8)
	if err != nil {
		return "", err
	}
	sort.Slice(recent, func(i, j int) bool {
		if recent[i].IsCurrent != recent[j].IsCurrent {
			return recent[i].IsCurrent
		}
		return recent[i].UpdatedAt.After(recent[j].UpdatedAt)
	})

	sort.Slice(kv, func(i, j int) bool { return kv[i].Key < kv[j].Key })

	builder := strings.Builder{}
	builder.WriteString("\n\n## Loop Context (persistent)\n\n")

	if current != nil {
		builder.WriteString("Current:\n")
		line := "- " + current.TaskID + " [" + current.Status + "] agent=" + current.AgentID
		line += " iter=" + itoa(current.LoopIteration)
		if !current.UpdatedAt.IsZero() {
			line += " updated=" + current.UpdatedAt.UTC().Format(time.RFC3339)
		}
		if strings.TrimSpace(current.Detail) != "" {
			line += " | " + strings.TrimSpace(current.Detail)
		}
		builder.WriteString(line)
		builder.WriteString("\n\n")
	}

	if len(recent) > 0 {
		builder.WriteString("Recent:\n")
		for _, it := range recent {
			prefix := "- "
			if it.IsCurrent {
				prefix = "- * "
			}
			line := prefix + it.TaskID + " [" + it.Status + "]"
			if strings.TrimSpace(it.Detail) != "" {
				line += " | " + strings.TrimSpace(it.Detail)
			}
			builder.WriteString(line)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	if len(kv) > 0 {
		builder.WriteString("Mem:\n")
		for _, it := range kv {
			builder.WriteString("- ")
			builder.WriteString(it.Key)
			builder.WriteString(": ")
			builder.WriteString(it.Value)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("CLI:\n")
	builder.WriteString("- forge work set <task-id> --status in_progress --detail \"...\"  (defaults to $FORGE_LOOP_ID)\n")
	builder.WriteString("- forge work current\n")
	builder.WriteString("- forge work clear\n")
	builder.WriteString("- forge mem set <key> \"<value>\"  (defaults to $FORGE_LOOP_ID)\n")

	out := builder.String()
	if len(out) > maxChars {
		out = out[:maxChars]
		out = strings.TrimRight(out, "\n") + "\n(truncated)\n"
	}
	return out, nil
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [32]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + (v % 10))
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
