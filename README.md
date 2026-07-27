# go-package

[![CI](https://github.com/bobadilla-tech/go-package/actions/workflows/ci.yml/badge.svg)](https://github.com/bobadilla-tech/go-package/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/bobadilla-tech/go-package/graph/badge.svg)](https://codecov.io/gh/bobadilla-tech/go-package)
[![Go Reference](https://pkg.go.dev/badge/github.com/bobadilla-tech/go-package.svg)](https://pkg.go.dev/github.com/bobadilla-tech/go-package)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Minimal, well-tested Go library that sums integers. Meant as a template repo
showing the repo hygiene (CI, coverage, licensing, ownership) expected of
Bobadilla Technologies' Go packages.

The **library is the main artifact**. The bundled CLI under `cmd/sum` is a
showcase / internal-testing tool with its own `go.mod`, so importing this
package never pulls in any CLI-only dependency.

## Install

```sh
go get github.com/bobadilla-tech/go-package
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/bobadilla-tech/go-package"
)

func main() {
	fmt.Println(sum.Sum(1, 2, 3)) // 6
}
```

## CLI (showcase only)

`cmd/sum` is a thin wrapper around the library, kept in its own Go module so its
dependencies never leak into the library's `go.mod`:

```sh
cd cmd/sum
go run . 1 2 3
# 6
```

## License

[MIT](LICENSE)
