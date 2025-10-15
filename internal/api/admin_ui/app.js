// API base URL
const API_BASE = '/admin/api';

// Get token from localStorage
let token = localStorage.getItem('token');

// Auto-refresh intervals
let dashboardRefreshInterval = null;
const DASHBOARD_REFRESH_MS = 5000; // Refresh every 5 seconds

// Helper function for API calls
async function apiCall(endpoint, options = {}) {
    const defaultOptions = {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
        }
    };

    const response = await fetch(`${API_BASE}${endpoint}`, {
        ...defaultOptions,
        ...options,
        headers: {
            ...defaultOptions.headers,
            ...options.headers
        }
    });

    if (response.status === 401 || response.status === 403) {
        showNotification('Session expired or insufficient permissions', 'error');
        setTimeout(() => {
            logout();
        }, 2000);
        throw new Error('Unauthorized');
    }

    if (!response.ok) {
        const error = await response.text();
        throw new Error(error || 'API request failed');
    }

    return response.json();
}

// Show notification
function showNotification(message, type = 'success') {
    const notification = document.getElementById('notification');
    notification.textContent = message;
    notification.className = `notification ${type} show`;
    setTimeout(() => {
        notification.classList.remove('show');
    }, 3000);
}

// Tab management
function showTab(tabName, element) {
    // Hide all tabs
    document.querySelectorAll('.tab-content').forEach(tab => {
        tab.classList.remove('active');
    });

    // Remove active class from all buttons
    document.querySelectorAll('.tab-button').forEach(btn => {
        btn.classList.remove('active');
    });

    // Show selected tab
    document.getElementById(tabName).classList.add('active');

    // Add active class to clicked button
    if (element) {
        element.classList.add('active');
    } else {
        // Find button by tab name if element not provided
        const buttons = document.querySelectorAll('.tab-button');
        buttons.forEach(btn => {
            if (btn.textContent.toLowerCase().includes(tabName)) {
                btn.classList.add('active');
            }
        });
    }

    // Load data for the tab
    loadTabData(tabName);

    // Stop dashboard auto-refresh when switching away
    if (tabName !== 'dashboard' && dashboardRefreshInterval) {
        clearInterval(dashboardRefreshInterval);
        dashboardRefreshInterval = null;
    }

    // Start dashboard auto-refresh when switching to it
    if (tabName === 'dashboard') {
        startDashboardRefresh();
    }
}

// Load data based on tab
async function loadTabData(tabName) {
    switch (tabName) {
        case 'dashboard':
            await loadDashboard();
            break;
        case 'connections':
            await loadConnections();
            break;
        case 'users':
            await loadUsers();
            break;
        case 'policies':
            await loadPolicies();
            break;
        case 'audit':
            await loadAuditLogs();
            break;
        case 'versions':
            await loadVersions();
            break;
    }
}

// Dashboard functions
async function loadDashboard() {
    try {
        console.log('Loading dashboard data...');
        const status = await apiCall('/status');
        console.log('Status:', status);

        const auditStats = await apiCall('/audit/stats');
        console.log('Audit stats:', auditStats);

        document.getElementById('system-status').textContent = status.status || 'unknown';
        document.getElementById('active-connections').textContent = status.active_connections || 0;
        document.getElementById('configured-connections').textContent = status.configured_connections || 0;
        document.getElementById('total-policies').textContent = status.policies || 0;
        document.getElementById('total-users').textContent = status.users || 0;
        document.getElementById('total-events').textContent = auditStats.total_events || 0;

        // Update last refresh time
        const now = new Date().toLocaleTimeString();
        const refreshElement = document.getElementById('last-refresh');
        if (refreshElement) {
            refreshElement.innerHTML = `Last updated: ${now} <span style="color: #4CAF50;">● Auto-refresh: 5s</span>`;
        }

        console.log('Dashboard loaded successfully');
    } catch (error) {
        console.error('Dashboard load error:', error);
        // Don't show notification on auto-refresh errors to avoid spam
        if (!dashboardRefreshInterval) {
            showNotification('Failed to load dashboard: ' + error.message, 'error');
        }

        // Show "Error" or 0 in stats instead of crashing
        document.getElementById('system-status').textContent = 'error';
        document.getElementById('active-connections').textContent = '?';
        document.getElementById('configured-connections').textContent = '?';
        document.getElementById('total-policies').textContent = '?';
        document.getElementById('total-users').textContent = '?';
        document.getElementById('total-events').textContent = '?';
    }
}

// Start dashboard auto-refresh
function startDashboardRefresh() {
    // Clear any existing interval
    if (dashboardRefreshInterval) {
        clearInterval(dashboardRefreshInterval);
    }

    // Set up new interval
    dashboardRefreshInterval = setInterval(() => {
        // Only refresh if dashboard tab is still active
        if (document.getElementById('dashboard').classList.contains('active')) {
            loadDashboard();
        } else {
            // Stop refreshing if user switched away
            stopDashboardRefresh();
        }
    }, DASHBOARD_REFRESH_MS);
}

// Stop dashboard auto-refresh
function stopDashboardRefresh() {
    if (dashboardRefreshInterval) {
        clearInterval(dashboardRefreshInterval);
        dashboardRefreshInterval = null;
    }
}

// Manual refresh dashboard
function refreshDashboard() {
    loadDashboard();
    showNotification('Dashboard refreshed', 'success');
}

// Connection functions
async function loadConnections() {
    try {
        const connections = await apiCall('/connections');
        const tbody = document.getElementById('connections-list');
        tbody.innerHTML = '';

        if (!connections || connections.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" style="text-align: center;">No connections configured</td></tr>';
            return;
        }

        connections.forEach(conn => {
            const row = document.createElement('tr');
            const tags = conn.tags ? conn.tags.join(', ') : 'none';

            row.innerHTML = `
                <td>${conn.name || 'unnamed'}</td>
                <td>${conn.type || 'unknown'}</td>
                <td>${conn.host || '-'}</td>
                <td>${conn.port || '-'}</td>
                <td>${tags}</td>
                <td class="action-buttons">
                    <button onclick="editConnection('${conn.name}')">Edit</button>
                    <button class="danger" onclick="deleteConnection('${conn.name}')">Delete</button>
                </td>
            `;
            tbody.appendChild(row);
        });
    } catch (error) {
        console.error('Failed to load connections:', error);
        showNotification('Failed to load connections: ' + error.message, 'error');
    }
}

function showConnectionForm() {
    document.getElementById('connection-form').style.display = 'block';
    document.getElementById('connection-form-title').textContent = 'Add Connection';
    document.getElementById('connectionForm').reset();
    document.getElementById('connection-original-name').value = '';
}

function hideConnectionForm() {
    document.getElementById('connection-form').style.display = 'none';
}

async function editConnection(name) {
    try {
        const connections = await apiCall('/connections');
        const conn = connections.find(c => c.name === name);

        if (!conn) {
            showNotification('Connection not found', 'error');
            return;
        }

        document.getElementById('connection-form').style.display = 'block';
        document.getElementById('connection-form-title').textContent = 'Edit Connection';
        document.getElementById('connection-original-name').value = name;
        document.getElementById('conn-name').value = conn.name;
        document.getElementById('conn-type').value = conn.type;
        document.getElementById('conn-host').value = conn.host;
        document.getElementById('conn-port').value = conn.port;
        document.getElementById('conn-tags').value = conn.tags ? conn.tags.join(', ') : '';
        document.getElementById('conn-backend-username').value = conn.backend_username || '';
        document.getElementById('conn-backend-database').value = conn.backend_database || '';
    } catch (error) {
        showNotification('Failed to load connection: ' + error.message, 'error');
    }
}

async function saveConnection(event) {
    event.preventDefault();

    const originalName = document.getElementById('connection-original-name').value;
    const isEdit = originalName !== '';

    const tags = document.getElementById('conn-tags').value
        .split(',')
        .map(t => t.trim())
        .filter(t => t !== '');

    const connection = {
        name: document.getElementById('conn-name').value,
        type: document.getElementById('conn-type').value,
        host: document.getElementById('conn-host').value,
        port: parseInt(document.getElementById('conn-port').value),
        tags: tags,
        backend_username: document.getElementById('conn-backend-username').value,
        backend_password: document.getElementById('conn-backend-password').value,
        backend_database: document.getElementById('conn-backend-database').value
    };

    try {
        if (isEdit) {
            await apiCall(`/connections/${originalName}`, {
                method: 'PUT',
                body: JSON.stringify(connection)
            });
            showNotification('Connection updated successfully');
        } else {
            await apiCall('/connections', {
                method: 'POST',
                body: JSON.stringify(connection)
            });
            showNotification('Connection created successfully');
        }

        hideConnectionForm();
        await loadConnections();
    } catch (error) {
        showNotification('Failed to save connection: ' + error.message, 'error');
    }
}

async function deleteConnection(name) {
    if (!confirm(`Are you sure you want to delete connection "${name}"?`)) {
        return;
    }

    try {
        await apiCall(`/connections/${name}`, {
            method: 'DELETE'
        });
        showNotification('Connection deleted successfully');
        await loadConnections();
    } catch (error) {
        showNotification('Failed to delete connection: ' + error.message, 'error');
    }
}

// User functions
async function loadUsers() {
    try {
        const users = await apiCall('/users');
        const tbody = document.getElementById('users-list');
        tbody.innerHTML = '';

        if (!users || users.length === 0) {
            tbody.innerHTML = '<tr><td colspan="3" style="text-align: center;">No local users configured</td></tr>';
            return;
        }

        users.forEach(user => {
            const row = document.createElement('tr');
            const roles = user.roles ? user.roles.join(', ') : 'none';

            row.innerHTML = `
                <td>${user.username || 'unnamed'}</td>
                <td>${roles}</td>
                <td class="action-buttons">
                    <button onclick="editUser('${user.username}')">Edit</button>
                    <button class="danger" onclick="deleteUser('${user.username}')">Delete</button>
                </td>
            `;
            tbody.appendChild(row);
        });
    } catch (error) {
        console.error('Failed to load users:', error);
        showNotification('Failed to load users: ' + error.message, 'error');
    }
}

function showUserForm() {
    document.getElementById('user-form').style.display = 'block';
    document.getElementById('user-form-title').textContent = 'Add User';
    document.getElementById('userForm').reset();
    document.getElementById('user-original-username').value = '';
}

function hideUserForm() {
    document.getElementById('user-form').style.display = 'none';
}

async function editUser(username) {
    try {
        const users = await apiCall('/users');
        const user = users.find(u => u.username === username);

        if (!user) {
            showNotification('User not found', 'error');
            return;
        }

        document.getElementById('user-form').style.display = 'block';
        document.getElementById('user-form-title').textContent = 'Edit User';
        document.getElementById('user-original-username').value = username;
        document.getElementById('user-username').value = user.username;
        document.getElementById('user-password').value = '';
        document.getElementById('user-roles').value = user.roles ? user.roles.join(', ') : '';
    } catch (error) {
        showNotification('Failed to load user: ' + error.message, 'error');
    }
}

async function saveUser(event) {
    event.preventDefault();

    const originalUsername = document.getElementById('user-original-username').value;
    const isEdit = originalUsername !== '';

    const roles = document.getElementById('user-roles').value
        .split(',')
        .map(r => r.trim())
        .filter(r => r !== '');

    const user = {
        username: document.getElementById('user-username').value,
        roles: roles
    };

    const password = document.getElementById('user-password').value;
    if (password) {
        user.password = password;
    }

    try {
        if (isEdit) {
            await apiCall(`/users/${originalUsername}`, {
                method: 'PUT',
                body: JSON.stringify(user)
            });
            showNotification('User updated successfully');
        } else {
            if (!password) {
                showNotification('Password is required for new users', 'error');
                return;
            }
            await apiCall('/users', {
                method: 'POST',
                body: JSON.stringify(user)
            });
            showNotification('User created successfully');
        }

        hideUserForm();
        await loadUsers();
    } catch (error) {
        showNotification('Failed to save user: ' + error.message, 'error');
    }
}

async function deleteUser(username) {
    if (!confirm(`Are you sure you want to delete user "${username}"?`)) {
        return;
    }

    try {
        await apiCall(`/users/${username}`, {
            method: 'DELETE'
        });
        showNotification('User deleted successfully');
        await loadUsers();
    } catch (error) {
        showNotification('Failed to delete user: ' + error.message, 'error');
    }
}

// Policy functions
async function loadPolicies() {
    try {
        const policies = await apiCall('/policies');
        const tbody = document.getElementById('policies-list');
        tbody.innerHTML = '';

        if (!policies || policies.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" style="text-align: center;">No policies configured</td></tr>';
            return;
        }

        policies.forEach(policy => {
            const row = document.createElement('tr');
            const roles = policy.roles ? policy.roles.join(', ') : 'none';
            const tags = policy.tags ? policy.tags.join(', ') : 'none';
            const tagMatch = policy.tag_match || 'all';
            const whitelistCount = policy.whitelist ? policy.whitelist.length : 0;

            row.innerHTML = `
                <td>${policy.name || 'unnamed'}</td>
                <td>${roles}</td>
                <td>${tags}</td>
                <td>${tagMatch}</td>
                <td>${whitelistCount} rules</td>
                <td class="action-buttons">
                    <button onclick="editPolicy('${policy.name}')">Edit</button>
                    <button class="danger" onclick="deletePolicy('${policy.name}')">Delete</button>
                </td>
            `;
            tbody.appendChild(row);
        });
    } catch (error) {
        console.error('Failed to load policies:', error);
        showNotification('Failed to load policies: ' + error.message, 'error');
    }
}

function showPolicyForm() {
    document.getElementById('policy-form').style.display = 'block';
    document.getElementById('policy-form-title').textContent = 'Add Policy';
    document.getElementById('policyForm').reset();
    document.getElementById('policy-original-name').value = '';
}

function hidePolicyForm() {
    document.getElementById('policy-form').style.display = 'none';
}

async function editPolicy(name) {
    try {
        const policies = await apiCall('/policies');
        const policy = policies.find(p => p.name === name);

        if (!policy) {
            showNotification('Policy not found', 'error');
            return;
        }

        document.getElementById('policy-form').style.display = 'block';
        document.getElementById('policy-form-title').textContent = 'Edit Policy';
        document.getElementById('policy-original-name').value = name;
        document.getElementById('policy-name').value = policy.name || '';
        document.getElementById('policy-roles').value = (policy.roles && policy.roles.length > 0) ? policy.roles.join(', ') : '';
        document.getElementById('policy-tags').value = (policy.tags && policy.tags.length > 0) ? policy.tags.join(', ') : '';
        document.getElementById('policy-tag-match').value = policy.tag_match || 'all';
        document.getElementById('policy-whitelist').value = (policy.whitelist && policy.whitelist.length > 0) ? policy.whitelist.join('\n') : '';

        console.log('Editing policy:', policy);
    } catch (error) {
        console.error('Failed to load policy:', error);
        showNotification('Failed to load policy: ' + error.message, 'error');
    }
}

async function savePolicy(event) {
    event.preventDefault();

    const originalName = document.getElementById('policy-original-name').value;
    const isEdit = originalName !== '';

    const roles = document.getElementById('policy-roles').value
        .split(',')
        .map(r => r.trim())
        .filter(r => r !== '');

    const tags = document.getElementById('policy-tags').value
        .split(',')
        .map(t => t.trim())
        .filter(t => t !== '');

    const whitelist = document.getElementById('policy-whitelist').value
        .split('\n')
        .map(w => w.trim())
        .filter(w => w !== '');

    const policy = {
        name: document.getElementById('policy-name').value,
        roles: roles,
        tags: tags,
        tag_match: document.getElementById('policy-tag-match').value,
        whitelist: whitelist
    };

    try {
        if (isEdit) {
            await apiCall(`/policies/${originalName}`, {
                method: 'PUT',
                body: JSON.stringify(policy)
            });
            showNotification('Policy updated successfully');
        } else {
            await apiCall('/policies', {
                method: 'POST',
                body: JSON.stringify(policy)
            });
            showNotification('Policy created successfully');
        }

        hidePolicyForm();
        await loadPolicies();
    } catch (error) {
        showNotification('Failed to save policy: ' + error.message, 'error');
    }
}

async function deletePolicy(name) {
    if (!confirm(`Are you sure you want to delete policy "${name}"?`)) {
        return;
    }

    try {
        await apiCall(`/policies/${name}`, {
            method: 'DELETE'
        });
        showNotification('Policy deleted successfully');
        await loadPolicies();
    } catch (error) {
        showNotification('Failed to delete policy: ' + error.message, 'error');
    }
}

// Audit log functions
async function loadAuditLogs() {
    try {
        const username = document.getElementById('filter-username').value;
        const action = document.getElementById('filter-action').value;
        const connection = document.getElementById('filter-connection').value;

        let endpoint = '/audit/logs?';
        if (username) endpoint += `username=${username}&`;
        if (action) endpoint += `action=${action}&`;
        if (connection) endpoint += `connection=${connection}&`;

        const data = await apiCall(endpoint);
        const container = document.getElementById('audit-logs');
        container.innerHTML = '';

        if (data.logs && data.logs.length > 0) {
            data.logs.forEach(log => {
                const entry = document.createElement('div');
                entry.className = 'log-entry';
                entry.textContent = log;
                container.appendChild(entry);
            });
        } else {
            container.textContent = 'No audit logs found';
        }
    } catch (error) {
        showNotification('Failed to load audit logs: ' + error.message, 'error');
    }
}

// Version functions
let cachedVersions = [];

async function loadVersions() {
    try {
        const versions = await apiCall('/config/versions');
        cachedVersions = versions;

        const tbody = document.getElementById('versions-list');
        tbody.innerHTML = '';

        // Populate version selectors for diff
        const selectA = document.getElementById('diff-version-a');
        const selectB = document.getElementById('diff-version-b');
        selectA.innerHTML = '';
        selectB.innerHTML = '';

        versions.forEach((version, index) => {
            const row = document.createElement('tr');
            const date = new Date(version.timestamp).toLocaleString();
            row.innerHTML = `
                <td>${version.id}</td>
                <td>${date}</td>
                <td>${version.comment || ''}</td>
                <td class="action-buttons">
                    <button onclick="showDiffViewer('${version.id}')">Compare</button>
                    ${version.id !== 'current' ? `<button class="secondary" onclick="rollbackVersion('${version.id}')">Rollback</button>` : ''}
                </td>
            `;
            tbody.appendChild(row);

            // Add to selectors
            const optionA = document.createElement('option');
            optionA.value = version.id;
            optionA.textContent = `${version.id} (${date})`;
            selectA.appendChild(optionA);

            const optionB = document.createElement('option');
            optionB.value = version.id;
            optionB.textContent = `${version.id} (${date})`;
            if (index === 1) optionB.selected = true; // Select second version by default
            selectB.appendChild(optionB);
        });
    } catch (error) {
        console.error('Failed to load versions:', error);
        showNotification('Failed to load versions: ' + error.message, 'error');
    }
}

function showDiffViewer(versionId) {
    document.getElementById('diff-viewer').style.display = 'block';
    document.getElementById('versions-table-container').style.display = 'none';

    // Set the selected version in dropdown
    document.getElementById('diff-version-b').value = versionId;

    // Auto-load diff
    loadDiff();
}

function closeDiffViewer() {
    document.getElementById('diff-viewer').style.display = 'none';
    document.getElementById('versions-table-container').style.display = 'block';
}

async function loadDiff() {
    const versionA = document.getElementById('diff-version-a').value;
    const versionB = document.getElementById('diff-version-b').value;

    try {
        const [configA, configB] = await Promise.all([
            apiCall(`/config/versions/${versionA}`),
            apiCall(`/config/versions/${versionB}`)
        ]);

        displayDiff(configA, configB, versionA, versionB);
    } catch (error) {
        showNotification('Failed to load diff: ' + error.message, 'error');
    }
}

function displayDiff(configA, configB, versionA, versionB) {
    const diffContainer = document.getElementById('diff-content');
    diffContainer.innerHTML = '';

    // Compare main sections
    const sections = ['server', 'auth', 'connections', 'policies', 'security', 'logging', 'approval', 'storage'];

    sections.forEach(section => {
        if (!configA[section] && !configB[section]) return;

        const sectionDiv = document.createElement('div');
        sectionDiv.className = 'diff-section';

        const title = document.createElement('div');
        title.className = 'diff-section-title';
        title.textContent = section.toUpperCase();
        sectionDiv.appendChild(title);

        const jsonA = JSON.stringify(configA[section] || {}, null, 2);
        const jsonB = JSON.stringify(configB[section] || {}, null, 2);

        if (jsonA === jsonB) {
            const unchanged = document.createElement('div');
            unchanged.className = 'diff-line unchanged';
            unchanged.textContent = '  (no changes)';
            sectionDiv.appendChild(unchanged);
        } else {
            // Simple line-by-line diff
            const linesA = jsonA.split('\n');
            const linesB = jsonB.split('\n');
            const maxLines = Math.max(linesA.length, linesB.length);

            for (let i = 0; i < maxLines; i++) {
                const lineA = linesA[i] || '';
                const lineB = linesB[i] || '';

                if (lineA === lineB) {
                    const line = document.createElement('div');
                    line.className = 'diff-line unchanged';
                    line.textContent = '  ' + lineA;
                    sectionDiv.appendChild(line);
                } else {
                    if (lineA) {
                        const line = document.createElement('div');
                        line.className = 'diff-line removed';
                        line.textContent = '- ' + lineA;
                        sectionDiv.appendChild(line);
                    }
                    if (lineB) {
                        const line = document.createElement('div');
                        line.className = 'diff-line added';
                        line.textContent = '+ ' + lineB;
                        sectionDiv.appendChild(line);
                    }
                }
            }
        }

        diffContainer.appendChild(sectionDiv);
    });
}

async function rollbackVersion(id) {
    if (!confirm(`Are you sure you want to rollback to version "${id}"? This will reload the server with the old configuration.`)) {
        return;
    }

    try {
        await apiCall(`/config/rollback/${id}`, {
            method: 'POST'
        });
        showNotification('Configuration rolled back successfully');
        await loadVersions();
        await loadDashboard();
    } catch (error) {
        showNotification('Failed to rollback: ' + error.message, 'error');
    }
}

// Login function
async function handleLogin(event) {
    event.preventDefault();

    const username = document.getElementById('login-username').value;
    const password = document.getElementById('login-password').value;
    const errorDiv = document.getElementById('login-error');

    errorDiv.classList.remove('show');

    try {
        const response = await fetch('/api/login', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ username, password })
        });

        if (!response.ok) {
            throw new Error('Invalid credentials');
        }

        const data = await response.json();
        token = data.token;
        localStorage.setItem('token', token);

        // Check if user has admin role
        try {
            const payload = JSON.parse(atob(token.split('.')[1]));
            if (!payload.roles || !payload.roles.includes('admin')) {
                throw new Error('Admin role required');
            }
        } catch (e) {
            errorDiv.textContent = 'Admin role required to access this area';
            errorDiv.classList.add('show');
            localStorage.removeItem('token');
            return;
        }

        // Show app, hide login
        showApp();
    } catch (error) {
        errorDiv.textContent = error.message || 'Login failed. Please check your credentials.';
        errorDiv.classList.add('show');
    }
}

// Logout function
function logout() {
    // Stop any auto-refresh
    stopDashboardRefresh();

    localStorage.removeItem('token');
    token = null;
    showLogin();
}

// Show login form
function showLogin() {
    document.getElementById('login-container').style.display = 'flex';
    document.getElementById('app-container').style.display = 'none';
}

// Show main app
async function showApp() {
    document.getElementById('login-container').style.display = 'none';
    document.getElementById('app-container').style.display = 'block';

    // Try to get username from token
    try {
        const payload = JSON.parse(atob(token.split('.')[1]));
        document.getElementById('current-user').textContent = payload.username || 'Admin';
    } catch (e) {
        document.getElementById('current-user').textContent = 'Admin';
    }

    // Make sure dashboard tab is active and load its data
    document.getElementById('dashboard').classList.add('active');
    document.querySelector('.tab-button').classList.add('active');

    // Load dashboard data and start auto-refresh
    await loadDashboard();
    startDashboardRefresh();
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', () => {
    if (!token) {
        showLogin();
    } else {
        // Verify token is valid by trying to access a protected endpoint
        fetch(API_BASE + '/status', {
            headers: {
                'Authorization': `Bearer ${token}`
            }
        }).then(response => {
            if (response.ok) {
                showApp();
            } else {
                localStorage.removeItem('token');
                token = null;
                showLogin();
            }
        }).catch(() => {
            localStorage.removeItem('token');
            token = null;
            showLogin();
        });
    }
});

