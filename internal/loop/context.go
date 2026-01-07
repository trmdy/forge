package loop

import (
	"fmt"
	"strings"
	"time"

	"github.com/tOgg1/forge/internal/models"
)

func buildInterruptContext(loop *models.Loop, logLines, ledgerLines int) string {
	builder := strings.Builder{}
	builder.WriteString("## Interrupt Context\n\n")
	builder.WriteString(fmt.Sprintf("Timestamp: %s\n\n", time.Now().UTC().Format(time.RFC3339)))

	if loop.LedgerPath != "" {
		if ledgerTail, err := tailFile(loop.LedgerPath, ledgerLines); err == nil && strings.TrimSpace(ledgerTail) != "" {
			builder.WriteString("### Ledger Tail\n\n```")
			builder.WriteString("\n")
			builder.WriteString(strings.TrimSpace(ledgerTail))
			builder.WriteString("\n```\n\n")
		}
	}

	if loop.LogPath != "" {
		if logTail, err := tailFile(loop.LogPath, logLines); err == nil && strings.TrimSpace(logTail) != "" {
			builder.WriteString("### Log Tail\n\n```")
			builder.WriteString("\n")
			builder.WriteString(strings.TrimSpace(logTail))
			builder.WriteString("\n```\n\n")
		}
	}

	return builder.String()
}
