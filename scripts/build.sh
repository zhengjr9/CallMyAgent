#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

REGISTRY="${REGISTRY:-docker.io/library}"
VERSION="${VERSION:-$(date +%Y%m%d-%H%M%S)}"

echo "=========================================="
echo "  CallMyAgent - Build Script"
echo "=========================================="
echo "Registry: $REGISTRY"
echo "Version:  $VERSION"
echo ""

# Step 1: Build Go server binary
echo "[1/4] Building Go server..."
cd "$PROJECT_DIR/backend"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o "$PROJECT_DIR/build/server" .
echo "  -> build/server"

# Step 2: Build server Docker image
echo "[2/4] Building server Docker image..."
cd "$PROJECT_DIR"
docker build -t "$REGISTRY/callmyagent-server:$VERSION" \
    -f docker/Dockerfile.server .
docker tag "$REGISTRY/callmyagent-server:$VERSION" \
    "$REGISTRY/callmyagent-server:latest"
echo "  -> $REGISTRY/callmyagent-server:$VERSION"

# Step 3: Build worker Docker image
echo "[3/4] Building worker Docker image..."
docker build -t "$REGISTRY/callmyagent-worker:$VERSION" \
    -f container/Dockerfile container/
docker tag "$REGISTRY/callmyagent-worker:$VERSION" \
    "$REGISTRY/callmyagent-worker:latest"
echo "  -> $REGISTRY/callmyagent-worker:$VERSION"

# Step 4: Summary
echo ""
echo "=========================================="
echo "  Build Complete"
echo "=========================================="
echo "Images:"
echo "  Server: $REGISTRY/callmyagent-server:$VERSION"
echo "  Worker: $REGISTRY/callmyagent-worker:$VERSION"
echo ""
echo "Next steps:"
echo "  1. Push images:"
echo "     docker push $REGISTRY/callmyagent-server:$VERSION"
echo "     docker push $REGISTRY/callmyagent-worker:$VERSION"
echo ""
echo "  2. Update k8s/secret.yaml with your API key"
echo ""
echo "  3. Deploy:"
echo "     make deploy REGISTRY=$REGISTRY VERSION=$VERSION"
