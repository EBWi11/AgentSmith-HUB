# Native libraries (librure)

This project uses [rure](https://github.com/rust-lang/regex/tree/master/regex-capi) (Rust regex) via [rure-go](https://github.com/BurntSushi/rure-go). Place the built library here so the Go linker can find it.

## macOS (Darwin)

Put `librure.a`, `librure.dylib`, or `librure.so` in:

```
lib/darwin/
```

Then build or run from repo root:

```bash
# Development run
make dev-backend

# Or build binary
make backend
```

Or manually:

```bash
cd src
CGO_ENABLED=1 CGO_LDFLAGS="-L../lib/darwin -lrure" go build -o ../build/agentsmith-hub .
```

## Linux

Libraries are under `lib/linux/$(arch)` (e.g. `lib/linux/arm64`, `lib/linux/amd64`). See Makefile and CI for details.
