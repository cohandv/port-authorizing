#!/bin/bash

# Script to import Keycloak realm with test users
# This handles the import manually since auto-import can be unreliable

set -e

echo "=== Keycloak Realm Import Script ==="
echo ""

# Check if Keycloak is running
if ! docker ps | grep -q port-auth-keycloak; then
    echo "❌ Keycloak container is not running"
    echo "Start it with: docker compose up -d keycloak"
    exit 1
fi

echo "✓ Keycloak container is running"
echo ""

# Wait for Keycloak to be ready
echo "Waiting for Keycloak to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:8180/health/ready > /dev/null 2>&1; then
        echo "✓ Keycloak is ready"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "❌ Keycloak did not become ready in time"
        exit 1
    fi
    sleep 2
done
echo ""

# Get admin token
echo "Getting admin access token..."
ADMIN_TOKEN=$(curl -s -X POST http://localhost:8180/realms/master/protocol/openid-connect/token \
    -d "client_id=admin-cli" \
    -d "username=admin" \
    -d "password=admin" \
    -d "grant_type=password" | jq -r '.access_token')

if [ "$ADMIN_TOKEN" == "null" ] || [ -z "$ADMIN_TOKEN" ]; then
    echo "❌ Failed to get admin token"
    exit 1
fi

echo "✓ Got admin token"
echo ""

# Check if realm already exists
echo "Checking if 'portauth' realm exists..."
REALM_EXISTS=$(curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
    http://localhost:8180/admin/realms/portauth 2>/dev/null || echo "not_found")

if [[ "$REALM_EXISTS" != "not_found" ]] && [[ "$REALM_EXISTS" == *"realm"* ]]; then
    echo "⚠️  Realm 'portauth' already exists"
    read -p "Do you want to delete and recreate it? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "Deleting existing realm..."
        curl -s -X DELETE -H "Authorization: Bearer $ADMIN_TOKEN" \
            http://localhost:8180/admin/realms/portauth
        echo "✓ Deleted existing realm"
    else
        echo "Keeping existing realm. Exiting."
        exit 0
    fi
fi

# Import realm
echo ""
echo "Importing realm from keycloak-realm.json..."

IMPORT_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d @docker/keycloak-realm.json \
    http://localhost:8180/admin/realms)

if [ -z "$IMPORT_RESPONSE" ]; then
    echo "✓ Realm imported successfully!"
else
    echo "❌ Import failed. Response: $IMPORT_RESPONSE"
    exit 1
fi

echo ""
echo "Verifying users..."

# Get new token for portauth realm
sleep 2

# Check if users exist by trying to login
for user in alice bob charlie; do
    TOKEN=$(curl -s -X POST http://localhost:8180/realms/portauth/protocol/openid-connect/token \
        -d "client_id=admin-cli" \
        -d "username=$user" \
        -d "password=password123" \
        -d "grant_type=password" 2>/dev/null | jq -r '.access_token' || echo "null")

    if [ "$TOKEN" != "null" ] && [ -n "$TOKEN" ] && [ "$TOKEN" != "" ]; then
        echo "✓ User '$user' can login"
    else
        echo "⚠️  User '$user' login failed (might need manual setup)"
    fi
done

echo ""
echo "=== Import Complete ==="
echo ""
echo "Keycloak Admin Console: http://localhost:8180"
echo "Username: admin"
echo "Password: admin"
echo ""
echo "Test Users (all password: password123):"
echo "  - alice (developer role)"
echo "  - bob (admin role)"
echo "  - charlie (qa role)"
echo ""
echo "If users don't work, see docker/keycloak-setup.md for manual setup instructions."

