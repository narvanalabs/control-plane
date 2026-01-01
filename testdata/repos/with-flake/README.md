# With Flake

A repository with an existing flake.nix file.

This tests the flake build strategy detection - when a flake.nix exists,
the system should use it directly instead of generating one.

## Build with Nix

```bash
nix build
```

## Run

```bash
./result/bin/with-flake
```

## Development

```bash
nix develop
go run .
```
