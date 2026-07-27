# go-package

Minimal template Go library: sums integers. Public API is just
`Sum(nums ...int) int` in the root `sum` package.

## Layout

- Root module (`github.com/bobadilla-tech/go-package`) — the library, zero
  dependencies.
- `cmd/sum` — showcase CLI, its **own separate Go module** with a `replace`
  directive back to the root. This keeps any CLI-only dependency out of the
  library's `go.mod`/`go.sum`. Most users should never need to touch `cmd/sum`.

## Commands

```sh
go test ./...                # run tests (root module)
go test ./... -covermode=atomic -coverprofile=coverage.out && go tool cover -func=coverage.out
go vet ./...
gofmt -l .
```

For the CLI module:

```sh
cd cmd/sum && go build ./... && go run . 1 2 3
```

No Makefile, no lint config beyond `go vet` + `gofmt` — matches the rest of the
bobadilla-tech Go packages.
