# Contributing to Asura

Thanks for your interest in contributing. This guide covers the basics.

## Development Workflow

Requires Go 1.24+ and [templ](https://templ.guide/) CLI.

```bash
go install github.com/a-h/templ/cmd/templ@v0.3.977
```

1. Fork the repo and create a branch from `main`
2. Run `make dev` to watch for template and CSS changes (or run `templ generate` and `make css` manually)
3. Make your changes
4. Run `make test` to verify
5. Commit with a clear message (see below)
6. Open a pull request

## Commit Messages

Use imperative mood, keep the subject line under 72 characters.

```
Add DNS record assertion type
Fix race condition in retention worker
Update SQLite dependency to v1.35
```

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- No dependencies unless truly necessary -- keep the binary small
- Error messages are lowercase, no trailing punctuation
- Table-driven tests where applicable
- Web UI templates use [templ](https://templ.guide/) -- run `templ generate` after editing `.templ` files

## What to Work On

- Check open issues for `good first issue` or `help wanted` labels
- Bug fixes and test coverage improvements are always welcome
- For larger features, open an issue first to discuss the approach

## Reporting Bugs

Open an issue with:
- Go version and OS
- Steps to reproduce
- Expected vs actual behavior
- Relevant log output

## Security

See [SECURITY.md](SECURITY.md) for reporting vulnerabilities.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
