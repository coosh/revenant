# Revenant

**A ghetto lazy TCP proxy to juggle GPU memory bound apps like Stable Diffusion and Ollama.**  
Don’t use this in production unless you enjoy pain.

## Purpose

This script exists to make it *barely* possible to share a single GPU between multiple heavy apps by:

- Listening on a TCP port
- Spawning the target service on first request
- Forwarding traffic once it's up
- Shutting it down after idle timeout

It’s useful when running things like `stable-diffusion-webui` and `ollama` on one GPU, but only one at a time.

## How it works

1. Waits for a request
2. If the target isn't running, it executes a start command and waits
3. Proxies traffic once ready
4. If no traffic hits it for a while, it runs a stop command

## Build

Very simple app so just need to build binary:

```bash
go build -o revenant
```

## Flags

```bash
./revenant \
  --listen :8080 \
  --target 127.0.0.1:7860 \
  --start-cmd './start.sh' \
  --stop-cmd './stop.sh' \
  --idle-timeout 10m \
  --start-timeout 30s

