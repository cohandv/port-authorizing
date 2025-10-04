# Keycloak Setup Guide

## Quick Setup (Manual Import)

Since Keycloak's auto-import can be tricky, here's how to manually set up the test realm with users:

### Option 1: Import via Admin Console (Recommended)

1. **Access Keycloak Admin Console:**
   ```
   URL: http://localhost:8180
   Username: admin
   Password: admin
   ```

2. **Create the Realm:**
   - Click dropdown in top-left (says "master")
   - Click "Create Realm"
   - Click "Browse" and select `docker/keycloak-realm.json`
   - Click "Create"

3. **Verify Users:**
   - Select "portauth" realm from dropdown
   - Go to "Users" in left menu
   - You should see: alice, bob, charlie

### Option 2: Import via CLI

```bash
# Copy realm file into container
docker cp docker/keycloak-realm.json port-auth-keycloak:/tmp/realm.json

# Import using Keycloak CLI
docker exec port-auth-keycloak /opt/keycloak/bin/kc.sh import \
  --file /tmp/realm.json

# Restart Keycloak
docker restart port-auth-keycloak
```

### Option 3: Manual User Creation

If import doesn't work, create users manually:

1. **Create Realm:**
   - Realm name: `portauth`

2. **Create Roles:**
   - Go to Realm roles
   - Add roles: `admin`, `developer`, `qa`, `user`

3. **Create Users:**

   **Alice (Developer):**
   - Username: `alice`
   - Email: `alice@portauth.local`
   - First name: `Alice`
   - Last name: `Developer`
   - Click "Create"
   - Go to "Credentials" tab
   - Set password: `password123`
   - Temporary: OFF
   - Go to "Role mapping" tab
   - Assign roles: `developer`, `user`

   **Bob (Admin):**
   - Username: `bob`
   - Email: `bob@portauth.local`
   - First name: `Bob`
   - Last name: `Admin`
   - Password: `password123`
   - Roles: `admin`, `user`

   **Charlie (QA):**
   - Username: `charlie`
   - Email: `charlie@portauth.local`
   - First name: `Charlie`
   - Last name: `QA`
   - Password: `password123`
   - Roles: `qa`, `user`

4. **Create Client:**
   - Go to "Clients"
   - Click "Create client"
   - Client ID: `port-authorizing`
   - Client Protocol: `openid-connect`
   - Click "Next"
   - Client authentication: ON
   - Click "Save"
   - Go to "Credentials" tab
   - Copy client secret (or set it to: `your-client-secret-change-in-production`)
   - Go to "Settings" tab
   - Valid redirect URIs: `http://localhost:8080/*`
   - Web origins: `http://localhost:8080`
   - Click "Save"

5. **Configure Client Scopes:**
   - In client settings, go to "Client scopes" tab
   - Click "Add client scope"
   - Select "roles"
   - Click "Add" > "Default"

6. **Add Roles to Token:**
   - Go to "Client scopes" in left menu
   - Click on the client scope (e.g., `port-authorizing-dedicated`)
   - Go to "Mappers" tab
   - Click "Add mapper" > "By configuration"
   - Select "User Realm Role"
   - Name: `realm roles`
   - Token Claim Name: `roles`
   - Claim JSON Type: String
   - Add to ID token: ON
   - Add to access token: ON
   - Add to userinfo: ON
   - Click "Save"

## Test Users

All users have the same password: `password123`

| Username | Email | Roles | Use Case |
|----------|-------|-------|----------|
| alice | alice@portauth.local | developer, user | Developer with limited prod access |
| bob | bob@portauth.local | admin, user | Admin with full access |
| charlie | charlie@portauth.local | qa, user | QA with test access |

## Testing Authentication

### Test via Direct Password Flow

```bash
# Get access token for alice
curl -X POST http://localhost:8180/realms/portauth/protocol/openid-connect/token \
  -d "client_id=port-authorizing" \
  -d "client_secret=your-client-secret-change-in-production" \
  -d "grant_type=password" \
  -d "username=alice" \
  -d "password=password123" \
  -d "scope=openid profile email roles"

# You should get back an access_token and id_token
```

### Test via Port Authorizing API

First, enable OIDC in `config.yaml`:

```yaml
auth:
  providers:
    - name: keycloak
      type: oidc
      enabled: true
      config:
        issuer: "http://localhost:8180/realms/portauth"
        client_id: "port-authorizing"
        client_secret: "your-client-secret-change-in-production"
        redirect_url: "http://localhost:8080/auth/callback/oidc"
        roles_claim: "roles"
        username_claim: "preferred_username"
```

Then restart the API and login:

```bash
# Login as alice
./bin/port-authorizing-cli login -u alice -p password123

# The API will authenticate via Keycloak!
```

## Troubleshooting

### Realm Not Found

If you get "Realm does not exist" errors:
- Verify realm was created/imported
- Check realm name is exactly `portauth`
- Make sure you're using the correct realm in URLs

### Users Not Found

If users don't appear:
- Check you're viewing the `portauth` realm (not `master`)
- Verify realm was imported correctly
- Try manual user creation

### Client Authentication Failed

If you get "Invalid client credentials":
- Verify client secret matches config
- Ensure "Client authentication" is ON in client settings
- Check client ID is exactly `port-authorizing`

### Roles Not in Token

If roles don't appear in JWT:
- Add realm roles mapper to client scope
- Ensure roles are assigned to users
- Check token in JWT debugger (jwt.io)

## Useful Admin URLs

- Admin Console: http://localhost:8180
- Realm Settings: http://localhost:8180/admin/master/console/#/portauth
- Users: http://localhost:8180/admin/master/console/#/portauth/users
- Clients: http://localhost:8180/admin/master/console/#/portauth/clients

## Keycloak Admin Credentials

- Username: `admin`
- Password: `admin`

**Note:** These are development credentials only. Change them in production!

