<!-- Thanks for contributing to Sheeld! -->

## What & why

<!-- What does this change do, and why is it needed? Link any related issue. -->

Closes #

## How it was tested

<!-- Commands run, cases covered. -->

- [ ] `gofmt -l .` is clean
- [ ] `go vet ./...` passes
- [ ] `go test ./...` passes
- [ ] `go test -tags integration ./internal/integration/` passes (if backend behavior changed)
- [ ] `cd web && npm run build` passes (if the dashboard changed)

## Checklist

- [ ] Tests added or updated for the change
- [ ] Docs updated (README / openapi.yaml / CLAUDE.md) where relevant
- [ ] No secrets logged; secret config fields stay redacted in API responses
- [ ] Database changes ship as goose migrations with a `Down` section
