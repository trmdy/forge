---
id: swarm-hnyo
status: closed
deps: [swarm-v419]
links: []
created: 2025-12-23T15:08:33.418237743+01:00
type: task
priority: 0
---
# Implement vault: credential reference resolver

Update account service to support vault: credential references.

Changes to internal/account/service.go:
1. Add vault: prefix handler in ResolveCredential()
2. Format: vault:provider/profile (e.g., vault:anthropic/work)
3. Reads API key from vault profile's auth files
4. Extract key from JSON using provider-specific field names

Credential resolution priority:
1. env:VAR_NAME - Environment variable
2. $VAR_NAME - Environment variable shorthand
3. file:/path - Read from file
4. vault:provider/profile - Read from Swarm vault (NEW)
5. Literal value - Direct value (legacy, discouraged)

Key extraction per provider:
- Anthropic: Look for 'api_key', 'apiKey', 'token' in auth.json
- OpenAI: Look for 'api_key', 'token' in auth.json  
- Google: Look for 'api_key' in settings.json
- Generic fallback: Try common field names

Error handling:
- Profile not found: clear error message with vault path
- Auth file missing: suggest running 'swarm vault backup'
- No API key in file: list available fields for debugging


