# Admin Web UI Guide

## Overview

The Port Authorizing admin web UI provides a browser-based interface for managing configurations, users, policies, connections, and viewing audit logs.

## Accessing the Admin UI

The admin UI is accessible at: `http://localhost:8080/admin`

**Requirements:**
- You must be logged in with a valid JWT token
- Your user must have the `admin` role

## Features

### 1. Dashboard
View system status at a glance:
- Active connections count
- Configured connections
- Total policies
- Total users
- Audit event count

### 2. Connection Management
- **List all connections** with their details (name, type, host, port, tags)
- **Add new connections** via a form interface
- **Edit existing connections**
- **Delete connections**

All changes are saved to the configuration file and hot-reloaded without server restart.

### 3. User & Role Management
- **List all local users** (only for local auth provider)
- **Create new users** with username, password, and roles
- **Edit user roles** and update passwords
- **Delete users**

Note: This only manages users in the local auth provider. Users from OIDC, LDAP, or SAML2 are managed externally.

### 4. Policy Management
- **List all policies** with roles, tags, and whitelist rules
- **Create new policies** with role-based access control
- **Edit policies** including tags, tag matching (all/any), and whitelist patterns
- **Delete policies**

### 5. Audit Log Viewer
- **View recent audit logs** (last 100 entries)
- **Filter logs** by username, action, or connection
- Real-time log viewing in a console-style interface

### 6. Configuration Versions
- **View configuration history** with timestamps and comments
- **Rollback to previous versions** with one click
- Automatically keeps the last 5 versions (configurable)

## Configuration Storage

The admin UI supports two storage backends:

### File Backend (Default)
```yaml
storage:
  type: file
  path: config.yaml
  versions: 5  # Keep last 5 versions
```

Features:
- Saves to local YAML file
- Automatic versioning with rotation
- Backup files: `config.yaml.v1`, `config.yaml.v2`, etc.
- Metadata files: `config.yaml.v1.meta` (contains timestamp, comment, author)

### Kubernetes Backend (Optional)
```yaml
storage:
  type: kubernetes
  namespace: default
  resource_type: configmap  # or secret
  resource_name: port-authorizing-config
```

Features:
- Stores configuration in Kubernetes ConfigMap or Secret
- Version history in resource annotations
- Requires building with `-tags k8s`:
  ```bash
  go build -tags k8s -o port-authorizing cmd/port-authorizing/main.go
  ```

## Hot Reload

All configuration changes made through the admin UI are hot-reloaded:
- ✅ Auth provider changes
- ✅ User additions/modifications
- ✅ Policy changes
- ✅ Connection updates
- ✅ Approval workflow changes

**What's preserved during reload:**
- Active proxy connections (users stay connected)
- Connection manager state

**What's reloaded:**
- Authorization rules
- Authentication configuration
- Approval patterns

## Security

### Authentication
- All admin endpoints require a valid JWT token
- Token must be included in `Authorization: Bearer <token>` header
- The admin UI stores the token in `localStorage`

### Authorization
- Only users with the `admin` role can access the admin UI
- Admin middleware checks role on every request
- Regular users cannot access `/admin` routes even if authenticated

### Audit Trail
Every admin action is logged:
- User additions/deletions
- Connection modifications
- Policy changes
- Configuration updates with author and timestamp

## API Endpoints

All admin API endpoints are prefixed with `/admin/api`:

### Configuration
- `GET /admin/api/config` - Get current config (sanitized)
- `PUT /admin/api/config?comment=...` - Update config
- `GET /admin/api/config/versions` - List versions
- `GET /admin/api/config/versions/:id` - Get specific version
- `POST /admin/api/config/rollback/:id` - Rollback to version

### Connections
- `GET /admin/api/connections` - List all
- `POST /admin/api/connections` - Create new
- `PUT /admin/api/connections/:name` - Update
- `DELETE /admin/api/connections/:name` - Delete

### Users
- `GET /admin/api/users` - List all (local only)
- `POST /admin/api/users` - Create new
- `PUT /admin/api/users/:username` - Update
- `DELETE /admin/api/users/:username` - Delete

### Policies
- `GET /admin/api/policies` - List all
- `POST /admin/api/policies` - Create new
- `PUT /admin/api/policies/:name` - Update
- `DELETE /admin/api/policies/:name` - Delete

### Audit & Status
- `GET /admin/api/audit/logs?username=&action=&connection=` - Get logs with filters
- `GET /admin/api/audit/stats` - Get statistics
- `GET /admin/api/status` - Get system status

## Usage Example

1. **Login** to the main application:
   ```bash
   curl -X POST http://localhost:8080/api/login \
     -H "Content-Type: application/json" \
     -d '{"username": "admin", "password": "admin123"}'
   ```

2. **Access admin UI**:
   - Navigate to `http://localhost:8080/admin` in your browser
   - The UI will use the token from localStorage

3. **Add a new connection**:
   - Click "Connections" tab
   - Click "Add Connection"
   - Fill in the form (name, type, host, port, tags)
   - Click "Save"
   - Connection is immediately available

4. **Create a policy**:
   - Click "Policies" tab
   - Click "Add Policy"
   - Specify roles, tags, and whitelist patterns
   - Click "Save"

5. **View audit logs**:
   - Click "Audit Logs" tab
   - Use filters to search specific events
   - View in real-time console format

## Embedded UI

The admin UI is embedded directly in the Go binary:
- No external file dependencies
- HTML, CSS, and JavaScript bundled using `go:embed`
- Single binary deployment
- Files located in `internal/api/admin_ui/`

## Development

To modify the admin UI:

1. Edit files in `internal/api/admin_ui/`:
   - `index.html` - Structure and layout
   - `styles.css` - Styling
   - `app.js` - Logic and API calls

2. Rebuild the binary:
   ```bash
   go build -o port-authorizing cmd/port-authorizing/main.go
   ```

3. The new UI will be embedded automatically

## Troubleshooting

### "Admin role required" error
- Ensure your user has the `admin` role in the configuration
- Check JWT token claims

### "Session expired" error
- Token has expired (default: 24 hours)
- Login again to get a new token

### Changes not reflecting
- Check browser console for API errors
- Verify the configuration was saved successfully
- Check server logs for reload errors

### Version rollback fails
- Ensure the version ID exists
- Check file permissions on configuration directory
- Verify storage backend is accessible

## Best Practices

1. **Always use comments** when updating config:
   - Describe what changed and why
   - Include ticket/issue references

2. **Test changes in staging** before production

3. **Backup before major changes**:
   - Versions are automatic, but manual backups are recommended

4. **Use strong passwords** for admin users:
   - Minimum 12 characters
   - Use bcrypt hashing in production

5. **Monitor audit logs** regularly:
   - Review admin actions
   - Watch for unauthorized access attempts

6. **Limit admin role** to necessary users only

## Future Enhancements

Potential features for future development:
- Role-based admin access (read-only admins)
- Bulk operations (import/export connections)
- Configuration diffing between versions
- Real-time notifications for config changes
- Multi-user collaboration with locking
- Advanced audit log analytics

