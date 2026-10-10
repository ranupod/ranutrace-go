# ranutrace-go

Official RanuPod Go Tracer and Zero-Config APM SDK.

## Features
- **Auto-Discovery**: Automatically uses `OTEL_EXPORTER_OTLP_ENDPOINT` or `DD_AGENT_HOST` injected by RanuPod.
- **Zero-Config**: Works out of the box with standard OpenTelemetry and Datadog APM collectors.
- **HTTP Middleware**: Single-line middleware to trace incoming HTTP requests, status codes, and latency.

## Quick Start

### 1. Installation
```bash
go get github.com/ranupod/ranutrace-go
```

### 2. Usage in `main.go`
```go
package main

import (
    "net/http"
    "github.com/ranupod/ranutrace-go"
)

func main() {
    // 1. Start global tracer (reads RanuPod env variables automatically)
    defer tracer.Start()()

    mux := http.NewServeMux()
    mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello from RanuPod!"))
    })

    // 2. Wrap with RanuPod tracing middleware
    wrappedMux := tracer.Middleware("my-service")(mux)

    http.ListenAndServe(":3000", wrappedMux)
}
```

## Zero-Code Orchestrion Compilation (Datadog-style)
To compile without modifying source code:
```bash
go build -toolexec="orchestrion toolexec" .
```
RanuPod automatically injects this compile flag during Docker builds when Observability is enabled!
