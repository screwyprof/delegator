# Contributing

Setup and running the stack are in the [README](README.md); this covers how changes get in.

## Tooling

Nothing to install beyond Go and Docker. The Makefile pins every tool (golangci-lint,
govulncheck, prettycov, go-cover-treemap) and runs it with `go run pkg@version`, never from your
`PATH`: a pin that defers to whatever is installed is a pin that lies. The first run compiles
each tool; later runs are cached.

The nix flake is a convenience, not a requirement. `nix develop` (or direnv's `use flake`) pins
Go and adds gopls, delve and pre-commit, and installs the git hooks on entry. Without nix,
`make hooks` installs them (it needs `pre-commit`).

## Gates

| Command | What it checks |
| --- | --- |
| `make check` | format, lint (the strict `.golangci.yml`, including depguard layering), and every test, acceptance tests included. Needs Postgres: `docker compose up -d postgres`. |
| `make coverage` | coverage per package, and fails below `COVERAGE_FLOOR`. Raise the floor when coverage rises, so it ratchets rather than drifts. |
| `make vulns` | govulncheck: vulnerabilities reachable from this code, in dependencies and in the standard library of the Go running it. |
| `docker compose build` | the service images. |

CI runs all four on every push to a branch, and on pull requests. Acceptance tests also call the
live TzKT API, so a TzKT outage fails CI. A separate workflow runs `make vulns` daily, since an
advisory can apply without any code changing.

Work happens on a branch; once CI is green it is squash-merged into `main`.

## Dependencies

[Renovate](renovate.json) proposes updates weekly. Once a release is three days old it automerges
whatever CI actually exercises (Go modules, the Makefile's tool pins, GitHub Actions, Docker
images), but only when every check is green. Majors, `flake.lock`, the devcontainer and security
updates open a pull request for a human instead.

## Upgrading Go

Renovate deliberately leaves the toolchain alone: it lives in several places that move in a
deliberate order, and patch releases have mattered here (1.27.0/1.27.1 miscount coverage,
golang/go#80974; 1.27.4 fixes a `net/http` server crash, GO-2026-6859).

1. **What ships**, as soon as the release is out:
   - `Dockerfile.migrator`, `Dockerfile.scraper`, `Dockerfile.web`: the `golang:<version>-alpine`
     builder image.
   - `.github/workflows/ci.yml` and `.github/workflows/vulns.yml`: `go-version`.
2. **The dev shell**, once nixpkgs has the release: `nix flake update`, then check
   `nix eval --raw .#devShells.<system>.default` resolves, and that
   `nix develop --command go version` reports the new patch.
3. **The minimum**, together with step 2: the `go` line in `go.work` and in all five `go.mod` files
   (`.`, `migrator`, `pkg`, `scraper`, `web`). Keep it at or below what the dev shell provides, or
   `GOTOOLCHAIN=auto` will download a newer Go behind nix's back.

After each step: `make check`, `make coverage`, `make vulns` and `docker compose build`. Until step 2
lands, `make vulns` in the dev shell may report standard-library vulnerabilities that the images,
built with the newer Go, don't have; CI is the authority.
