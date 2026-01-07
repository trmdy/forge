package cli

import "strings"

func agentMailConfigFromEnv() agentMailConfig {
	cfg := agentMailConfig{
		URL:     getEnvWithFallback("FORGE_AGENT_MAIL_URL", "SWARM_AGENT_MAIL_URL"),
		Project: getEnvWithFallback("FORGE_AGENT_MAIL_PROJECT", "SWARM_AGENT_MAIL_PROJECT"),
		Agent:   getEnvWithFallback("FORGE_AGENT_MAIL_AGENT", "SWARM_AGENT_MAIL_AGENT"),
	}

	if value := getEnvWithFallback("FORGE_AGENT_MAIL_TIMEOUT", "SWARM_AGENT_MAIL_TIMEOUT"); value != "" {
		if parsed, ok := parseEnvDuration(value); ok {
			cfg.Timeout = parsed
		}
	}

	cfg.Project = strings.TrimSpace(cfg.Project)
	cfg.Agent = strings.TrimSpace(cfg.Agent)
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultAgentMailTimeout
	}

	return cfg
}
