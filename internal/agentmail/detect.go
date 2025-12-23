package agentmail

import (
	"os"
	"path/filepath"
	"strings"
)

var configCandidates = []string{
	"mcp.json",
	"mcp.jsonc",
	"mcp.config.json",
	"mcp-config.json",
	".mcp.json",
	".mcp.jsonc",
	filepath.Join(".mcp", "servers.json"),
	filepath.Join(".mcp", "servers.jsonc"),
	filepath.Join(".claude", "mcp.json"),
	filepath.Join(".claude", "mcp.jsonc"),
	filepath.Join(".claude", "mcp-config.json"),
	filepath.Join(".claude", "mcp_config.json"),
}

var agentMailTokens = []string{
	"mcp_agent_mail",
	"agent-mail",
	"agent_mail",
	"agentmail",
	"agent mail",
}

// HasAgentMailConfig reports whether the repo contains an MCP config mentioning Agent Mail.
func HasAgentMailConfig(repoPath string) (bool, error) {
	if strings.TrimSpace(repoPath) == "" {
		return false, nil
	}

	for _, relative := range configCandidates {
		path := filepath.Join(repoPath, relative)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false, err
		}
		if info.IsDir() {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		lower := strings.ToLower(string(data))
		for _, token := range agentMailTokens {
			if strings.Contains(lower, token) {
				return true, nil
			}
		}
	}

	return false, nil
}
