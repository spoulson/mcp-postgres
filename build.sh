#!/usr/bin/env bash
set -euo pipefail

IMAGE="mcp-postgres"

docker buildx build --platform linux/amd64  --tag "$IMAGE:latest" --load .
docker buildx build --platform linux/arm64/v8 --tag "$IMAGE:latest-arm64" --load .
