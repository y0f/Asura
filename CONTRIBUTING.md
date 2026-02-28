# Contributing to Asura

Thanks for your interest in contributing. This guide covers the basics.

## Running Locally

Requires Go 1.24+ and [templ](https://templ.guide/) CLI:

```bash
go install github.com/a-h/templ/cmd/templ@v0.3.977
```

```bash
# copy and edit config
cp config.example.yaml config.yaml

# generate an API key + hash, paste the hash into config.yaml under auth.api_keys[].hash
go run ./cmd/asura --setup

# set cookie_secure: false in config.yaml (no TLS locally)

# build and run
templ generate
CGO_ENABLED=0 go build -o asura ./cmd/asura
./asura -config config.yaml
```

Open http://localhost:8090 and log in with the key from `--setup`.

`config.yaml` is gitignored — it won't be committed.

## Development Workflow

All commands run from inside the `asura/` directory.

**Linux / macOS / Git Bash:**
```bash
make dev   # terminal 1: watches templates + CSS, rebuilds on save
make run   # terminal 2: builds and starts the server
```

**Windows (PowerShell)** — run each in a separate terminal:
```powershell
# terminal 1: watch templates
templ generate --watch

# terminal 1b: watch CSS (separate terminal)
.\tailwindcss.exe -i web\tailwind.input.css -o web\static\tailwind.css --watch

# terminal 2: build and run
$env:CGO_ENABLED="0"; go build -o asura.exe ./cmd/asura; .\asura.exe -config config.yaml
```

After `.go` changes: stop the server (`Ctrl+C`), rebuild, and restart.

To run tests: `make test` or `go test -race -count=1 ./...`

1. Fork the repo and create a branch from `main`
2. Start the watchers and server as above
3. Make your changes
4. Run tests to verify
5. Commit with a clear message (see below)
6. Open a pull request

## Commit Messages

Keep the subject line under 72 characters.

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
