set shell := ["bash", "-uc"]
set working-directory := ".."

# Run the service with modd for automatic recompilations while developing locally
watch:
    modd

# Run the service with dev config
run:
    go run cmd/main.go --config configs/dev.toml

# Build the binary with goreleaser
build:
    goreleaser build --snapshot --clean --single-target

# Run tests
test:
    go test ./...

# Lint code
lint:
    golangci-lint run

# Clean all local development data
[confirm]
clean:
    rm -rf restate-data/ cache/
