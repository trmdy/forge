package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/sequences"
	"github.com/tOgg1/forge/internal/templates"
)

var (
	msgNow        bool
	msgNextPrompt string
	msgTemplate   string
	msgSequence   string
	msgVars       []string
	msgPool       string
	msgProfile    string
	msgState      string
	msgTag        string
	msgAll        bool
)

func init() {
	rootCmd.AddCommand(loopMsgCmd)

	loopMsgCmd.Flags().BoolVar(&msgNow, "now", false, "interrupt and restart immediately")
	loopMsgCmd.Flags().StringVar(&msgNextPrompt, "next-prompt", "", "override prompt for next iteration")
	loopMsgCmd.Flags().StringVar(&msgTemplate, "template", "", "message template name")
	loopMsgCmd.Flags().StringVar(&msgSequence, "seq", "", "sequence name")
	loopMsgCmd.Flags().StringSliceVar(&msgVars, "var", nil, "template/sequence variable (key=value)")
	loopMsgCmd.Flags().StringVar(&msgPool, "pool", "", "filter by pool")
	loopMsgCmd.Flags().StringVar(&msgProfile, "profile", "", "filter by profile")
	loopMsgCmd.Flags().StringVar(&msgState, "state", "", "filter by state")
	loopMsgCmd.Flags().StringVar(&msgTag, "tag", "", "filter by tag")
	loopMsgCmd.Flags().BoolVar(&msgAll, "all", false, "target all loops")
}

var loopMsgCmd = &cobra.Command{
	Use:   "msg [loop] [message]",
	Short: "Queue a message for loop(s)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if msgTemplate != "" && msgSequence != "" {
			return fmt.Errorf("use either --template or --seq, not both")
		}

		selector := loopSelector{Pool: msgPool, Profile: msgProfile, State: msgState, Tag: msgTag}
		message := ""

		if msgAll || selector.Pool != "" || selector.Profile != "" || selector.State != "" || selector.Tag != "" {
			message = strings.Join(args, " ")
		} else if len(args) > 0 {
			if len(args) < 2 && msgTemplate == "" && msgSequence == "" && msgNextPrompt == "" {
				return fmt.Errorf("message text required")
			}
			if len(args) > 1 {
				selector.LoopRef = args[0]
				message = strings.Join(args[1:], " ")
			} else if msgTemplate != "" || msgSequence != "" || msgNextPrompt != "" {
				selector.LoopRef = args[0]
			} else {
				return fmt.Errorf("message text required")
			}
		}

		if selector.LoopRef == "" && !msgAll && selector.Pool == "" && selector.Profile == "" && selector.State == "" && selector.Tag == "" {
			return fmt.Errorf("specify a loop or selector")
		}

		vars := parseKeyValuePairs(msgVars)
		repoPath, err := resolveRepoPath("")
		if err != nil {
			return err
		}

		if msgTemplate != "" {
			tmpl, err := loadTemplate(repoPath, msgTemplate)
			if err != nil {
				return err
			}
			message, err = templates.RenderTemplate(tmpl, vars)
			if err != nil {
				return err
			}
		}

		var sequenceItems []*models.LoopQueueItem
		if msgSequence != "" {
			seq, err := loadSequence(repoPath, msgSequence)
			if err != nil {
				return err
			}
			sequenceItems, err = renderLoopSequence(seq, vars)
			if err != nil {
				return err
			}
		}

		if strings.TrimSpace(message) == "" && len(sequenceItems) == 0 && msgNextPrompt == "" {
			return fmt.Errorf("message, --seq, or --next-prompt required")
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		poolRepo := db.NewPoolRepository(database)
		profileRepo := db.NewProfileRepository(database)
		queueRepo := db.NewLoopQueueRepository(database)

		loops, err := selectLoops(context.Background(), loopRepo, poolRepo, profileRepo, selector)
		if err != nil {
			return err
		}
		if len(loops) == 0 {
			return fmt.Errorf("no loops matched")
		}

		for _, loopEntry := range loops {
			items := make([]*models.LoopQueueItem, 0)

			if msgNextPrompt != "" {
				promptPath, _, err := resolvePromptPath(loopEntry.RepoPath, msgNextPrompt)
				if err != nil {
					return err
				}
				payload, _ := json.Marshal(models.NextPromptOverridePayload{Prompt: promptPath, IsPath: true})
				items = append(items, &models.LoopQueueItem{
					Type:    models.LoopQueueItemNextPromptOverride,
					Payload: payload,
				})
			}

			if len(sequenceItems) > 0 {
				items = append(items, sequenceItems...)
			}

			if strings.TrimSpace(message) != "" {
				if msgNow {
					payload, _ := json.Marshal(models.SteerPayload{Message: message})
					items = append(items, &models.LoopQueueItem{Type: models.LoopQueueItemSteerMessage, Payload: payload})
				} else {
					payload, _ := json.Marshal(models.MessageAppendPayload{Text: message})
					items = append(items, &models.LoopQueueItem{Type: models.LoopQueueItemMessageAppend, Payload: payload})
				}
			} else if msgNow {
				payload, _ := json.Marshal(models.SteerPayload{Message: "Operator interrupt"})
				items = append(items, &models.LoopQueueItem{Type: models.LoopQueueItemSteerMessage, Payload: payload})
			}

			if err := queueRepo.Enqueue(context.Background(), loopEntry.ID, items...); err != nil {
				return err
			}
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{"loops": len(loops), "queued": true})
		}

		if IsQuiet() {
			return nil
		}

		fmt.Fprintf(os.Stdout, "Queued message for %d loop(s)\n", len(loops))
		return nil
	},
}

func loadTemplate(repoPath, name string) (*templates.Template, error) {
	items, err := templates.LoadTemplatesFromSearchPaths(repoPath)
	if err != nil {
		return nil, err
	}
	for _, tmpl := range items {
		if tmpl.Name == name {
			return tmpl, nil
		}
	}
	return nil, fmt.Errorf("template %q not found", name)
}

func loadSequence(repoPath, name string) (*sequences.Sequence, error) {
	items, err := sequences.LoadSequencesFromSearchPaths(repoPath)
	if err != nil {
		return nil, err
	}
	for _, seq := range items {
		if seq.Name == name {
			return seq, nil
		}
	}
	return nil, fmt.Errorf("sequence %q not found", name)
}

func renderLoopSequence(seq *sequences.Sequence, vars map[string]string) ([]*models.LoopQueueItem, error) {
	items, err := sequences.RenderSequence(seq, vars)
	if err != nil {
		return nil, err
	}

	loopItems := make([]*models.LoopQueueItem, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case models.QueueItemTypeMessage:
			payload, err := item.GetMessagePayload()
			if err != nil {
				return nil, err
			}
			data, _ := json.Marshal(models.MessageAppendPayload{Text: payload.Text})
			loopItems = append(loopItems, &models.LoopQueueItem{Type: models.LoopQueueItemMessageAppend, Payload: data})
		case models.QueueItemTypePause:
			payload, err := item.GetPausePayload()
			if err != nil {
				return nil, err
			}
			data, _ := json.Marshal(models.LoopPausePayload{DurationSeconds: payload.DurationSeconds, Reason: payload.Reason})
			loopItems = append(loopItems, &models.LoopQueueItem{Type: models.LoopQueueItemPause, Payload: data})
		default:
			return nil, fmt.Errorf("sequence step type %q not supported for loops", item.Type)
		}
	}

	return loopItems, nil
}

func parseKeyValuePairs(pairs []string) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]string)
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		out[parts[0]] = parts[1]
	}
	return out
}
