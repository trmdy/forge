package fmail

import (
	"fmt"
	"regexp"
	"strings"
)

var namePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// NormalizeTopic lowercases and validates a topic name.
func NormalizeTopic(topic string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(topic))
	if normalized == "" || !namePattern.MatchString(normalized) {
		return "", ErrInvalidTopic
	}
	return normalized, nil
}

// ValidateTopic enforces topic naming rules without modification.
func ValidateTopic(topic string) error {
	value := strings.TrimSpace(topic)
	if value == "" || value != strings.ToLower(value) || !namePattern.MatchString(value) {
		return ErrInvalidTopic
	}
	return nil
}

// NormalizeAgentName lowercases and validates an agent name.
func NormalizeAgentName(name string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" || !namePattern.MatchString(normalized) {
		return "", ErrInvalidAgent
	}
	return normalized, nil
}

// ValidateAgentName enforces agent naming rules without modification.
func ValidateAgentName(name string) error {
	value := strings.TrimSpace(name)
	if value == "" || value != strings.ToLower(value) || !namePattern.MatchString(value) {
		return ErrInvalidAgent
	}
	return nil
}

// NormalizeTarget returns the normalized target and whether it is a DM.
func NormalizeTarget(target string) (string, bool, error) {
	raw := strings.TrimSpace(target)
	if raw == "" {
		return "", false, ErrInvalidTarget
	}
	if strings.HasPrefix(raw, "@") {
		agent, err := NormalizeAgentName(strings.TrimPrefix(raw, "@"))
		if err != nil {
			return "", false, err
		}
		return "@" + agent, true, nil
	}
	topic, err := NormalizeTopic(raw)
	if err != nil {
		return "", false, err
	}
	return topic, false, nil
}

// ValidateTarget checks whether a target is a topic or direct message.
func ValidateTarget(target string) error {
	raw := strings.TrimSpace(target)
	if raw == "" {
		return ErrInvalidTarget
	}
	if strings.HasPrefix(raw, "@") {
		return ValidateAgentName(strings.TrimPrefix(raw, "@"))
	}
	if err := ValidateTopic(raw); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidTarget, raw)
	}
	return nil
}
