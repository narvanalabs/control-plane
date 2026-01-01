# Mixed Languages

A repository with multiple languages to test detection priority.

Contains:
- Go backend (go.mod)
- Node.js frontend (package.json)
- Python scripts (requirements.txt)

This tests how the detector handles repositories with multiple languages.
The expected behavior is to detect all languages and either:
1. Return an error asking the user to specify a strategy
2. Use priority-based detection (flake > language > dockerfile)
