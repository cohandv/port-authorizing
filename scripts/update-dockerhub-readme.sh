#!/bin/bash
set -e

# Update Docker Hub README
# Usage: ./scripts/update-dockerhub-readme.sh

DOCKER_USERNAME="cohandv"
DOCKER_REPO="port-authorizing"
README_FILE="DOCKER_HUB_README.md"

echo "📝 Updating Docker Hub README for ${DOCKER_USERNAME}/${DOCKER_REPO}"
echo ""

# Check if README exists
if [ ! -f "$README_FILE" ]; then
    echo "❌ Error: $README_FILE not found"
    exit 1
fi

# Check if docker is available
if ! command -v docker &> /dev/null; then
    echo "❌ Error: docker command not found"
    exit 1
fi

# Check if logged in
if ! docker info &> /dev/null; then
    echo "⚠️  Not logged into Docker. Please login first:"
    echo "   docker login"
    exit 1
fi

echo "Using README file: $README_FILE"
echo ""

# Method 1: Using Docker Hub API (requires token)
if [ -n "$DOCKERHUB_TOKEN" ]; then
    echo "🔑 Using Docker Hub API with token..."

    README_CONTENT=$(cat "$README_FILE" | jq -Rs .)

    curl -X PATCH \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $DOCKERHUB_TOKEN" \
        -d "{\"full_description\": $README_CONTENT}" \
        "https://hub.docker.com/v2/repositories/${DOCKER_USERNAME}/${DOCKER_REPO}/"

    echo ""
    echo "✅ README updated via API"
else
    echo "⚠️  DOCKERHUB_TOKEN not set"
    echo ""
    echo "To update README automatically, you need a Docker Hub token:"
    echo "1. Go to https://hub.docker.com/settings/security"
    echo "2. Create a new access token"
    echo "3. Export it: export DOCKERHUB_TOKEN='your-token'"
    echo ""
    echo "Or update manually:"
    echo "1. Go to https://hub.docker.com/repository/docker/${DOCKER_USERNAME}/${DOCKER_REPO}/general"
    echo "2. Click 'Edit' in the Description section"
    echo "3. Copy content from: $README_FILE"
    echo ""

    # Open browser to Docker Hub
    if command -v open &> /dev/null; then
        echo "🌐 Opening Docker Hub in browser..."
        open "https://hub.docker.com/repository/docker/${DOCKER_USERNAME}/${DOCKER_REPO}/general"
    fi
fi

echo ""
echo "📋 README preview (first 500 chars):"
echo "---"
head -c 500 "$README_FILE"
echo ""
echo "..."
echo "---"
echo ""
echo "✅ Done!"

