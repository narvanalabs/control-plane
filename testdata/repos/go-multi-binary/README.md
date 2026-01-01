# Go Multi-Binary

A Go project with multiple binaries in cmd/*.

## Binaries

- `cmd/api` - REST API server
- `cmd/worker` - Background job worker
- `cmd/cli` - Command-line interface tool

## Build

```bash
go build -o bin/api ./cmd/api
go build -o bin/worker ./cmd/worker
go build -o bin/cli ./cmd/cli
```
