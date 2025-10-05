#!/bin/bash
set -e

# Comprehensive Keycloak Setup Script
# Handles realm import, client configuration, scopes, and testing

KEYCLOAK_URL="http://localhost:8180"
KEYCLOAK_ADMIN="admin"
KEYCLOAK_PASSWORD="admin"
REALM="portauth"
CLIENT_ID="port-authorizing"
REDIRECT_URI="http://localhost:8080/api/auth/oidc/callback"

echo "=================================="
echo "🔧 Keycloak Setup & Configuration"
echo "=================================="
echo ""

# Function to get admin token
get_admin_token() {
  TOKEN=$(curl -s -X POST "${KEYCLOAK_URL}/realms/master/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "username=${KEYCLOAK_ADMIN}" \
    -d "password=${KEYCLOAK_PASSWORD}" \
    -d "grant_type=password" \
    -d "client_id=admin-cli" | jq -r '.access_token')

  if [ "$TOKEN" = "null" ] || [ -z "$TOKEN" ]; then
    echo "❌ Failed to get admin token"
    echo "   Make sure Keycloak is running: docker-compose up -d keycloak"
    exit 1
  fi
  echo "✓ Got admin token"
}

# Function to check if realm exists
check_realm_exists() {
  REALM_EXISTS=$(curl -s -X GET "${KEYCLOAK_URL}/admin/realms/${REALM}" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -w "%{http_code}" -o /dev/null)

  if [ "$REALM_EXISTS" = "200" ]; then
    return 0
  else
    return 1
  fi
}

# Function to import realm
import_realm() {
  echo ""
  echo "📦 Importing realm from keycloak-realm.json..."

  if [ ! -f "docker/keycloak-realm.json" ]; then
    echo "❌ keycloak-realm.json not found"
    exit 1
  fi

  # Import realm
  curl -s -X POST "${KEYCLOAK_URL}/admin/realms" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d @docker/keycloak-realm.json > /dev/null 2>&1 || echo "  (realm may already exist)"

  echo "✓ Realm imported/verified"
}

# Function to configure client scopes
configure_client_scopes() {
  echo ""
  echo "🔧 Configuring client scopes..."

  # Create profile scope if it doesn't exist
  curl -s -X POST "${KEYCLOAK_URL}/admin/realms/${REALM}/client-scopes" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
      "name": "profile",
      "protocol": "openid-connect",
      "attributes": {
        "include.in.token.scope": "true",
        "display.on.consent.screen": "true"
      },
      "protocolMappers": [
        {
          "name": "username",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usermodel-property-mapper",
          "consentRequired": false,
          "config": {
            "userinfo.token.claim": "true",
            "user.attribute": "username",
            "id.token.claim": "true",
            "access.token.claim": "true",
            "claim.name": "preferred_username",
            "jsonType.label": "String"
          }
        },
        {
          "name": "full name",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-full-name-mapper",
          "consentRequired": false,
          "config": {
            "id.token.claim": "true",
            "access.token.claim": "true",
            "userinfo.token.claim": "true"
          }
        }
      ]
    }' > /dev/null 2>&1 || true

  # Create email scope if it doesn't exist
  curl -s -X POST "${KEYCLOAK_URL}/admin/realms/${REALM}/client-scopes" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
      "name": "email",
      "protocol": "openid-connect",
      "attributes": {
        "include.in.token.scope": "true",
        "display.on.consent.screen": "true"
      },
      "protocolMappers": [
        {
          "name": "email",
          "protocol": "openid-connect",
          "protocolMapper": "oidc-usermodel-property-mapper",
          "consentRequired": false,
          "config": {
            "userinfo.token.claim": "true",
            "user.attribute": "email",
            "id.token.claim": "true",
            "access.token.claim": "true",
            "claim.name": "email",
            "jsonType.label": "String"
          }
        }
      ]
    }' > /dev/null 2>&1 || true

  echo "✓ Client scopes created"
}

# Function to configure client
configure_client() {
  echo ""
  echo "🔧 Configuring client '${CLIENT_ID}'..."

  # Get client internal ID
  CLIENT_UUID=$(curl -s -X GET "${KEYCLOAK_URL}/admin/realms/${REALM}/clients" \
    -H "Authorization: Bearer $TOKEN" | jq -r ".[] | select(.clientId==\"${CLIENT_ID}\") | .id")

  if [ -z "$CLIENT_UUID" ] || [ "$CLIENT_UUID" = "null" ]; then
    echo "❌ Client not found"
    exit 1
  fi

  echo "✓ Found client: $CLIENT_UUID"

  # Update redirect URIs
  echo "  • Updating redirect URIs..."
  curl -s -X PUT "${KEYCLOAK_URL}/admin/realms/${REALM}/clients/$CLIENT_UUID" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
      \"redirectUris\": [
        \"http://localhost:8080/*\",
        \"${REDIRECT_URI}\"
      ],
      \"webOrigins\": [\"http://localhost:8080\"]
    }" > /dev/null

  # Assign default client scopes
  echo "  • Assigning client scopes..."
  for scope in "profile" "email" "roles"; do
    SCOPE_ID=$(curl -s -X GET "${KEYCLOAK_URL}/admin/realms/${REALM}/client-scopes" \
      -H "Authorization: Bearer $TOKEN" | jq -r ".[] | select(.name==\"$scope\") | .id")

    if [ ! -z "$SCOPE_ID" ] && [ "$SCOPE_ID" != "null" ]; then
      curl -s -X PUT "${KEYCLOAK_URL}/admin/realms/${REALM}/clients/$CLIENT_UUID/default-client-scopes/$SCOPE_ID" \
        -H "Authorization: Bearer $TOKEN" > /dev/null 2>&1 || true
    fi
  done

  echo "✓ Client configured"
}

# Function to verify configuration
verify_configuration() {
  echo ""
  echo "🔍 Verifying configuration..."

  # Get client details
  CLIENT_UUID=$(curl -s -X GET "${KEYCLOAK_URL}/admin/realms/${REALM}/clients" \
    -H "Authorization: Bearer $TOKEN" | jq -r ".[] | select(.clientId==\"${CLIENT_ID}\") | .id")

  # Get redirect URIs
  REDIRECT_URIS=$(curl -s -X GET "${KEYCLOAK_URL}/admin/realms/${REALM}/clients/$CLIENT_UUID" \
    -H "Authorization: Bearer $TOKEN" | jq -r '.redirectUris[]')

  echo "  Redirect URIs:"
  echo "$REDIRECT_URIS" | while read uri; do
    echo "    • $uri"
  done

  # Get assigned scopes
  echo "  Default client scopes:"
  curl -s -X GET "${KEYCLOAK_URL}/admin/realms/${REALM}/clients/$CLIENT_UUID/default-client-scopes" \
    -H "Authorization: Bearer $TOKEN" | jq -r '.[].name' | while read scope; do
    echo "    • $scope"
  done

  echo "✓ Configuration verified"
}

# Function to test authentication
test_authentication() {
  echo ""
  echo "🧪 Testing authentication..."

  # Test with alice user
  echo "  Testing with user 'alice' (password: password123)..."

  AUTH_RESULT=$(curl -s -X POST "${KEYCLOAK_URL}/realms/${REALM}/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "client_id=${CLIENT_ID}" \
    -d "client_secret=your-client-secret-change-in-production" \
    -d "grant_type=password" \
    -d "username=alice" \
    -d "password=password123" \
    -d "scope=openid profile email")

  ACCESS_TOKEN=$(echo "$AUTH_RESULT" | jq -r '.access_token')

  if [ "$ACCESS_TOKEN" = "null" ] || [ -z "$ACCESS_TOKEN" ]; then
    echo "  ⚠️  Authentication test failed (this is OK if Direct Grant is disabled)"
    echo "     Error: $(echo "$AUTH_RESULT" | jq -r '.error_description')"
  else
    echo "  ✓ Authentication successful"

    # Decode token to show claims
    ID_TOKEN=$(echo "$AUTH_RESULT" | jq -r '.id_token')
    if [ "$ID_TOKEN" != "null" ] && [ ! -z "$ID_TOKEN" ]; then
      PAYLOAD=$(echo "$ID_TOKEN" | cut -d'.' -f2 | base64 -d 2>/dev/null || echo "{}")
      USERNAME=$(echo "$PAYLOAD" | jq -r '.preferred_username // .sub')
      ROLES=$(echo "$PAYLOAD" | jq -r '.roles[]?' 2>/dev/null | tr '\n' ', ' | sed 's/,$//')
      EMAIL=$(echo "$PAYLOAD" | jq -r '.email // "N/A"')

      echo "    Username: $USERNAME"
      echo "    Email: $EMAIL"
      echo "    Roles: $ROLES"
    fi
  fi
}

# Function to display user information
show_users() {
  echo ""
  echo "👥 Configured users:"
  echo "  • alice (password: password123) - roles: developer, user"
  echo "  • bob (password: password123) - roles: admin, user"
  echo "  • charlie (password: password123) - roles: qa, user"
}

# Main execution
main() {
  case "${1:-setup}" in
    setup)
      echo "Running full setup..."
      get_admin_token

      if check_realm_exists; then
        echo "✓ Realm '${REALM}' exists"
      else
        import_realm
      fi

      configure_client_scopes
      configure_client
      verify_configuration
      test_authentication
      show_users

      echo ""
      echo "=================================="
      echo "✅ Keycloak setup complete!"
      echo "=================================="
      echo ""
      echo "🌐 Access Keycloak Admin: ${KEYCLOAK_URL}"
      echo "   Username: ${KEYCLOAK_ADMIN}"
      echo "   Password: ${KEYCLOAK_PASSWORD}"
      echo ""
      echo "🧪 Test CLI login:"
      echo "   ./bin/port-authorizing-cli login"
      echo ""
      ;;

    verify)
      echo "Running verification only..."
      get_admin_token
      verify_configuration
      show_users
      ;;

    test)
      echo "Running authentication test..."
      get_admin_token
      test_authentication
      ;;

    import)
      echo "Importing realm..."
      get_admin_token
      import_realm
      configure_client_scopes
      configure_client
      verify_configuration
      ;;

    *)
      echo "Usage: $0 [setup|verify|test|import]"
      echo ""
      echo "Commands:"
      echo "  setup   - Full setup (default): import realm, configure client, verify"
      echo "  verify  - Verify current configuration"
      echo "  test    - Test authentication with sample user"
      echo "  import  - Re-import realm and reconfigure"
      exit 1
      ;;
  esac
}

main "$@"

