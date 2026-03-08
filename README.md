# flagr sdk-go

Go SDK for [Flagr](https://flagr.dev) — feature flag client for Go services.

Connects to the Flagr Evaluation API via SSE, seeds a local in-memory cache from the initial snapshot, and applies real-time updates as flags change. Every flag evaluation is fully local — no network call per check.

## Prerequisites

- Go >= 1.23
- An active Flagr account and an SDK key (`sdk_live_...`) from the [dashboard](https://flagr.dev)

## Install

```bash
go get github.com/flagr-dev/sdk-go
```

## Quickstart

```go
package main

import (
    "context"
    featureflags "github.com/flagr-dev/sdk-go"
)

func main() {
    client := featureflags.NewClient("sdk_live_xxx")

    // Connect to the SSE stream and seed the local cache
    if err := client.Connect(context.Background()); err != nil {
        panic(err)
    }

    provider := featureflags.NewProvider(client)

    // IsEnabled — synchronous local evaluation, no network call
    // TenantID can be a user ID, org ID, account ID, or any entity identifier
    enabled := provider.IsEnabled(
        "new-checkout",                          // flag key
        "a1b2c3d4-e5f6-7890-abcd-ef1234567890", // tenant ID
        false,                                   // default value if flag not found
    )

    if enabled {
        // flag is enabled for this tenant
    }
}
```

## Reactive updates (OnChange)

`OnChange` fires the callback immediately with the current value, then again on every SSE update. No new connection is made — it reuses the existing one.

```go
package main

import (
    "context"
    "fmt"
    featureflags "github.com/flagr-dev/sdk-go"
)

func main() {
    client := featureflags.NewClient("sdk_live_xxx")
    if err := client.Connect(context.Background()); err != nil {
        panic(err)
    }

    provider := featureflags.NewProvider(client)

    sub := provider.OnChange(
        "new-checkout",                          // flag key
        "a1b2c3d4-e5f6-7890-abcd-ef1234567890", // tenant ID
        func(enabled bool) {
            fmt.Println("new-checkout is now", enabled)
        },
        false, // default value if flag not found
    )
    defer sub.Unsubscribe()

    // ... rest of your program
}
```

## Running locally

```bash
# Download dependencies
go mod download

# Build the package
go build ./...
```

## Running tests

The unit tests run fully offline — no server or SDK key required.

```bash
go test ./... -v
```

To run with the race detector (recommended before releasing):

```bash
go test -race ./... -v
```

## Releasing a new version

Go modules are versioned by git tags. There is no build or publish step — `go get` fetches directly from the tag.

1. Make sure all tests pass:
   ```bash
   go test ./...
   ```

2. Create and push a version tag:
   ```bash
   git tag v0.1.1
   git push origin v0.1.1
   ```

3. The new version is immediately available via:
   ```bash
   go get github.com/flagr-dev/sdk-go@v0.1.1
   ```

No CI step is required for Go releases — the tag is the release.
