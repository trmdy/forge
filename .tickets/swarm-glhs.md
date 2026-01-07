---
id: swarm-glhs
status: closed
deps: [swarm-hnyo]
links: []
created: 2025-12-23T15:09:02.316427233+01:00
type: task
priority: 0
---
# Remove CAAM references from codebase

Remove all CAAM-related code and references from the codebase.

Files to delete:
- internal/account/caam/parser.go
- internal/account/caam/parser_test.go
- internal/account/caam/ directory

Code to remove from internal/account/service.go:
- resolveCaamCredential() function (~70 lines)
- caam: prefix handling in ResolveCredential()

Code to remove from internal/cli/accounts.go:
- accountsImportCaamCmd command and all related code
- accountsImportCaamPath, accountsImportCaamProvider, accountsImportCaamDryRun flags
- showCaamImportPreview() function
- CaamImportResult type
- All 'import-caam' subcommand registration

Documentation updates:
- Remove any CAAM references from docs/
- Update config.example.yaml if it mentions caam:

Test updates:
- Remove CAAM-related test cases
- Update any tests that use caam: credential references

Verification:
- grep -r 'caam' should return no results in internal/
- go build ./... should pass
- go test ./... should pass


