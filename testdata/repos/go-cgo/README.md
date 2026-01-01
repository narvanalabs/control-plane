# Go CGO

A Go application that uses CGO for native C bindings.

## Dependencies

- SQLite3 (via go-sqlite3 which requires CGO)
- Custom C code

## Build

```bash
CGO_ENABLED=1 go build -o app .
```

## Notes

This application requires CGO to be enabled and a C compiler to be available.
