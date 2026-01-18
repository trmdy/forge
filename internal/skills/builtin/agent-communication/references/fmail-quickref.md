export FMAIL_AGENT=<your-name>   # Prefer a stable name for the session
fmail register                   # Request a unique name (auto-generated)
fmail send @agent "message"      # Direct message
fmail send topic "message"       # Topic message (e.g., status, editing)
fmail log @agent -n 20           # Read DMs
fmail watch topic --count 1      # Wait for a reply
