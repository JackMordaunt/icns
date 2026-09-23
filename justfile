# Standard recipes: build (debug), release, clean, test, install.
# check runs the full gate (check.sh: format, vet, staticcheck, modernize, tests)
# and publish wraps release.sh (goreleaser). cmd/preview and cmd/shell-extension
# are separate modules in go.work, built on the platforms that can build them.

bin := "icnsify"

# `just` alone lists the recipes.
default:
    @just --list --unsorted

# Debug build of the library and icnsify: symbols kept.
build:
    go build ./...
    go build -o {{bin}} ./cmd/icnsify

# Optimised icnsify: stripped, reproducible paths.
release:
    go build -trimpath -ldflags='-s -w' -o {{bin}} ./cmd/icnsify

# Vet and run the tests of the root module.
test:
    go vet ./...
    go test ./...

# The full gate CI runs; `just check fix` applies what can be applied first.
check *args:
    ./check.sh {{args}}

# Publish a release with goreleaser: `just publish [tag]` or `just publish mint TAG`.
publish *args:
    ./release.sh {{args}}

# Remove build output and the Go build cache for this module.
clean:
    rm -f {{bin}}
    go clean

# Copy the release binary to a directory on PATH. An installed copy is replaced in
# place; otherwise ~/.local/bin, ~/bin, GOBIN, GOPATH/bin, /usr/local/bin, else the
# first writable PATH entry.
install: release
    #!/usr/bin/env bash
    set -euo pipefail
    dest=""
    if existing=$(command -v {{bin}} 2>/dev/null) && [[ -w ${existing%/*} ]]; then
        dest=${existing%/*}
    fi
    gopath=$(go env GOPATH 2>/dev/null || true); gobin=$(go env GOBIN 2>/dev/null || true)
    for d in "$HOME/.local/bin" "$HOME/bin" "$gobin" "${gopath:+$gopath/bin}" /usr/local/bin; do
        [[ -z $dest && -n $d && -d $d && -w $d && ":$PATH:" == *":$d:"* ]] && dest=$d
    done
    if [[ -z $dest ]]; then
        IFS=: read -ra dirs <<<"$PATH"
        for d in "${dirs[@]}"; do [[ -d $d && -w $d ]] && { dest=$d; break; }; done
    fi
    [[ -n $dest ]] || { echo "install: no writable directory on PATH" >&2; exit 1; }
    install -m 755 {{bin}} "$dest/{{bin}}"
    echo "installed $dest/{{bin}}"
