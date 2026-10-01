# baton

Go CLI (one static binary) that stores agent session handoffs as Markdown.
README.md is the specification.

## Layout

- `cmd/baton/`: command dispatch (`main.go`), one `*_cmds.go` per area,
  e2e scripts in `testdata/script/*.txtar` (testscript).
- `internal/config`: config + paths parsers, `BATON_*` overrides.
- `internal/store`: project key, frontmatter parser, tasks/history/archive.
- `internal/gitinfo`, `internal/tips`, `internal/agent`, `internal/dashboard`
  (API, auth, `web/` embedded), `internal/service`, `internal/update`,
  `internal/fsutil` (atomic write, locks).
- `integrations/`: embedded skills/commands; register new ones in
  `integrations.go`.

## Rules

- Go 1.22 is the minimum: do not raise `go` in go.mod. `go get` of new
  module versions may try to; pin versions that support 1.22.
- Single runtime dependency: `golang.org/x/crypto/scrypt`. No YAML library.
- OS-specific code goes into `*_windows.go` / `*_unix.go`.
- Report commands (`show`, `tasks`, `stale`, `tips search`) exit 0 on
  "nothing found" states.
- Dashboard JS is vanilla, no build step, never uses innerHTML for content.

## Checks

```sh
gofmt -l . ; go vet ./... ; GOOS=windows go vet ./...
go test ./...
node --check internal/dashboard/web/*.js
```
