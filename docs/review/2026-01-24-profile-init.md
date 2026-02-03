# Review: profile init + default harness detection (2026-01-24)

## Scope
- internal/cli/profile_import_aliases.go
- internal/cli/profile.go
- internal/cli/config.go
- internal/cli/profile_import_aliases_test.go
- README.md
- docs/quickstart.md
- docs/simplification-prd.md
- docs/cli.md (profile init line only)

## Findings
- None.

## Notes
- Added default harness detection when alias files are default and no explicit alias args.
- Kept legacy command name as alias.

## Communication
- None.
