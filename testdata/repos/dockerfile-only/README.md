# Dockerfile Only

A repository with only a Dockerfile, no language-specific files.

This tests the dockerfile build strategy detection.

## Build

```bash
docker build -t dockerfile-only .
```

## Run

```bash
docker run -p 8080:8080 dockerfile-only
```
