<p align="center">
  <img width="120px" src="floppy.png">

  <h1 align="center">Floppy</h1>
  <p align="center">
    A service orchestration tool for development environments, similar to Docker Compose but designed for local development workflows.
  </p>
</p>


## Features

- **Service Management**: Start, stop, and manage multiple services from a single configuration
- **Bundle Support**: Group related services into bundles for easy management
- **Multiple Service Types**: Support for Python (Poetry), Portal (Bun), and Docker services
- **Database Setup**: Automatic database creation and migration running
- **Port Management**: Automatic port conflict detection
- **Colored Output**: Each service gets its own color for easy log identification
- **Background Mode**: Run services in detached mode

## Requirements

- Go 1.22+
- External tools used at runtime depending on command:
  - `poetry` (Python services)
  - `bun` (portal services)
  - `git` (pull/reset)
  - `psql` (setup)
  - `lsof` (port detection / ps / stop)

## Install dependencies

From `floppy-go/`:

```sh
go mod tidy
```

Note: If your environment blocks network access, `go mod tidy` will fail until network access is allowed.

## Build

```sh
go build ./cmd/floppy
```

This produces a `floppy` binary in the current directory.

## Run

Use `-f` to specify a config file, or rely on context/default search order.

```sh
./floppy -f /path/to/services.yaml up
```

### Default config search order

If `-f` is not provided and no context is set:

- `./services.yaml`
- `./dev-env/services.yaml`
- `../dev-env/services.yaml`
- `$SERVICES_ROOT/dev-env/services.yaml` (if `SERVICES_ROOT` is set)

## Commands

- `up [service-or-bundle ...] [-d] [--force] [--build]`
- `stop [service ...] [--remove]`
- `down [service ...]` (alias of `stop`)
- `ps [-q]`
- `list [--simple]`
- `exec COMMAND [args...] [--type TYPE] [--exclude a,b,c]`
- `pull [service ...]`
- `reset [--type TYPE] [--exclude a,b,c]`
- `update-lib LIB [--type TYPE] [--exclude a,b,c]`
- `add-lib LIB [--type TYPE] [--exclude a,b,c]`
- `setup`
- `logs SERVICE [-f] [--tail N]`
- `set-context [-f PATH] [--show] [--clear]`
- `version`

## Examples

```sh
./floppy up                 # Start all services
./floppy up linden-api      # Start a single service
./floppy up linden-bundle   # Start a bundle
./floppy up -d              # Detached mode
./floppy stop               # Stop running services
./floppy ps                 # List running services
./floppy list --simple      # Flat list
./floppy exec gst           # Run command in each service
./floppy pull               # Git pull/clone
./floppy reset              # Git reset/clean
./floppy setup              # Install deps, create DBs, run migrations
./floppy set-context -f /path/to/services.yaml
./floppy version
```

## Notes

- `up` in non-detached mode launches a full-screen TUI showing logs on the left and service status on the right.
- Port validation uses `lsof`. Use `--force` to kill processes occupying required ports.
- On Windows, PTY support is disabled and logs are not line-buffered.
- If your environment blocks `asdf` shims, you can override tool paths:
  - `FLOPPY_POETRY=/absolute/path/to/poetry`
  - `FLOPPY_BUN=/absolute/path/to/bun`
  - `FLOPPY_PYTHON=/absolute/path/to/python`
- If `FLOPPY_*` is not set, Floppy will try to pick the newest installed version under `~/.asdf/installs/<tool>/`.
- If PTY usage is blocked by your system, run `up` with `--no-pty` or set `FLOPPY_NO_PTY=1`.

## Distribution

### Build locally

```sh
make build
```

### Release artifacts (tarballs + checksums)

```sh
make release
```

Artifacts land in `dist/` as:

- `floppy-darwin-amd64.tar.gz`
- `floppy-darwin-arm64.tar.gz`
- `floppy-linux-amd64.tar.gz`
- `floppy-linux-arm64.tar.gz`
- `floppy-checksums.txt`

### Homebrew

A starter Homebrew formula lives at `packaging/homebrew/floppy.rb`. Update:

- `homepage`
- `version`
- `url` for each platform
- `sha256` (from `dist/floppy-checksums.txt`)

Then publish via a tap repo (e.g. `brew tap yourorg/floppy`).

### Update Homebrew formula

After `make release`:

```sh
scripts/update-homebrew-formula.sh 1.0.0
```

This updates the darwin URLs and `sha256` entries in `packaging/homebrew/floppy.rb` using `dist/floppy-checksums.txt`.
