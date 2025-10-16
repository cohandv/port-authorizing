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

// Helper function to escape HTML for safe display
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
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

// Reset all forms when switching tabs
function resetAllForms() {
    // Hide and reset all form containers
    const forms = [
        'connection-form',
        'user-form',
        'policy-form',
        'approval-pattern-form',
        'providers-form'
    ];
    
    forms.forEach(formId => {
        const formContainer = document.getElementById(formId);
        if (formContainer) {
            formContainer.style.display = 'none';
        }
    });
    
    // Reset all actual form elements
    document.querySelectorAll('form').forEach(form => {
        if (form.id !== 'loginForm') { // Don't reset login form
            form.reset();
        }
    });
    
    // Hide policy test results
    const policyTestResults = document.getElementById('policy-test-results');
    if (policyTestResults) {
        policyTestResults.style.display = 'none';
    }
    
    // Clear any hidden input fields (like edit mode indicators)
    document.querySelectorAll('input[type="hidden"]').forEach(input => {
        if (input.id !== 'login-username' && input.id !== 'login-password') {
            input.value = '';
        }
    });
}

// Tab management
function showTab(tabName, element, updateURL = true) {
    // Update URL
    if (updateURL && window.history) {
        const url = new URL(window.location);
        url.searchParams.set('tab', tabName);
        window.history.pushState({ tab: tabName }, '', url);
    }

    // Hide and reset all forms
    resetAllForms();

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
        case 'approvals':
            await loadApprovals();
            break;
        case 'audit':
            await loadAuditLogs();
            break;
        case 'versions':
            await loadVersions();
            break;
        case 'policy-tester':
            initPolicyTester();
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
            tbody.innerHTML = '<tr><td colspan="7" style="text-align: center;">No connections configured</td></tr>';
            return;
        }

        connections.forEach(conn => {
            const row = document.createElement('tr');
            const tags = conn.tags ? conn.tags.join(', ') : 'none';
            const duration = conn.duration || 'default';

            row.innerHTML = `
                <td>${conn.name || 'unnamed'}</td>
                <td>${conn.type || 'unknown'}</td>
                <td>${conn.host || '-'}</td>
                <td>${conn.port || '-'}</td>
                <td>${duration}</td>
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
    // Reset field visibility
    updateConnectionFields();
}

function hideConnectionForm() {
    document.getElementById('connection-form').style.display = 'none';
    document.getElementById('connectionForm').reset();
}

// Show/hide fields based on connection type
function updateConnectionFields() {
    const type = document.getElementById('conn-type').value;
    const postgresFields = document.getElementById('postgres-fields');
    
    // Hide all type-specific fields first
    postgresFields.style.display = 'none';
    
    // Show relevant fields based on type
    if (type === 'postgres') {
        postgresFields.style.display = 'block';
    }
    // For http/https, we infer the scheme from the type itself
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
        // Don't populate password field - let user enter new password or leave empty to keep existing
        document.getElementById('conn-backend-password').value = '';
        
        // Populate duration (now comes as string from API)
        document.getElementById('conn-duration').value = conn.duration || '';
        
        // Populate metadata as JSON string
        if (conn.metadata && Object.keys(conn.metadata).length > 0) {
            document.getElementById('conn-metadata').value = JSON.stringify(conn.metadata, null, 2);
        } else {
            document.getElementById('conn-metadata').value = '';
        }
        
        // Update visible fields based on type
        updateConnectionFields();
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

    const connType = document.getElementById('conn-type').value;
    
    const connection = {
        name: document.getElementById('conn-name').value,
        type: connType,
        host: document.getElementById('conn-host').value,
        port: parseInt(document.getElementById('conn-port').value),
        tags: tags
    };

    // Add duration if provided
    const duration = document.getElementById('conn-duration').value.trim();
    if (duration) {
        connection.duration = duration;
    }

    // Add metadata if provided
    const metadataStr = document.getElementById('conn-metadata').value.trim();
    if (metadataStr) {
        try {
            connection.metadata = JSON.parse(metadataStr);
            
            // Validate that metadata has a description field
            if (!connection.metadata.description || typeof connection.metadata.description !== 'string') {
                showNotification('Metadata must include a "description" field with a string value', 'error');
                return;
            }
        } catch (e) {
            showNotification('Invalid JSON in metadata field', 'error');
            return;
        }
    }

    // Add type-specific fields
    if (connType === 'http' || connType === 'https') {
        // Scheme is inferred from type
        connection.scheme = connType;
    } else if (connType === 'postgres') {
        connection.backend_username = document.getElementById('conn-backend-username').value;
        connection.backend_database = document.getElementById('conn-backend-database').value;
        
        // Only include password if it's provided (not empty)
        const password = document.getElementById('conn-backend-password').value;
        if (password) {
            connection.backend_password = password;
        }
    }

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

// Policy Tester Functions

// Load connections for the policy tester dropdown
async function loadPolicyTesterConnections() {
    try {
        const connections = await apiCall('/connections');
        const select = document.getElementById('test-connection');
        select.innerHTML = '<option value="">Select a connection...</option>';

        connections.forEach(conn => {
            const option = document.createElement('option');
            option.value = conn.name;
            option.textContent = `${conn.name} (${conn.type})`;
            option.dataset.connectionType = conn.type; // Store connection type for auto-detection
            select.appendChild(option);
        });
    } catch (error) {
        console.error('Failed to load connections for policy tester:', error);
    }
}

// Auto-detect query type based on connection selection
function autoDetectQueryType() {
    const connectionSelect = document.getElementById('test-connection');
    const queryTypeSelect = document.getElementById('test-query-type');
    const hintElement = document.getElementById('query-type-hint');
    const selectedOption = connectionSelect.options[connectionSelect.selectedIndex];

    if (selectedOption && selectedOption.dataset.connectionType) {
        const connectionType = selectedOption.dataset.connectionType;

        // Auto-detect query type based on connection type
        if (connectionType === 'postgres' || connectionType === 'oracle' || connectionType === 'mysql') {
            queryTypeSelect.value = 'database';
            hintElement.textContent = `Auto-detected: ${connectionType.toUpperCase()} connections use database queries`;
        } else if (connectionType === 'http' || connectionType === 'https') {
            queryTypeSelect.value = 'http';
            hintElement.textContent = `Auto-detected: ${connectionType.toUpperCase()} connections use HTTP requests`;
        } else {
            // For TCP or unknown types, default to database (more common for testing)
            queryTypeSelect.value = 'database';
            hintElement.textContent = `Auto-detected: ${connectionType.toUpperCase()} connections default to database queries`;
        }

        // Update the UI to show appropriate fields
        toggleQueryFields();
    } else {
        hintElement.textContent = '';
    }
}

// Toggle query fields based on query type
function toggleQueryFields() {
    const queryType = document.getElementById('test-query-type').value;
    const httpFields = document.getElementById('http-fields');
    const databaseFields = document.getElementById('database-fields');

    if (queryType === 'http') {
        httpFields.style.display = 'block';
        databaseFields.style.display = 'none';
    } else {
        httpFields.style.display = 'none';
        databaseFields.style.display = 'block';
    }
}

// Test policy access
async function testPolicyAccess(event) {
    event.preventDefault();

    const connection = document.getElementById('test-connection').value;
    const role = document.getElementById('test-role').value;
    const queryType = document.getElementById('test-query-type').value;

    if (!connection || !role) {
        showNotification('Please select a connection and enter a role', 'error');
        return;
    }

    try {
        const testData = {
            connection: connection,
            role: role,
            query_type: queryType
        };

        // Add query-specific fields based on type
        if (queryType === 'http') {
            const method = document.getElementById('test-method').value;
            const path = document.getElementById('test-path').value;
            testData.method = method;
            testData.path = path || '/';
        } else {
            const query = document.getElementById('test-query').value;
            if (!query.trim()) {
                showNotification('Please enter a database query to test', 'error');
                return;
            }
            testData.query = query;
        }

        const result = await apiCall('/policy-test', {
            method: 'POST',
            body: JSON.stringify(testData)
        });

        displayPolicyTestResults(result);
    } catch (error) {
        showNotification('Failed to test policy access: ' + error.message, 'error');
    }
}

// Display policy test results
function displayPolicyTestResults(result) {
    const resultsDiv = document.getElementById('policy-test-results');
    const contentDiv = document.getElementById('test-results-content');

    let html = `
        <div class="test-summary">
            <h4>Access Summary</h4>
            <div class="access-result ${result.hasAccess ? 'allowed' : 'denied'}">
                <strong>${result.hasAccess ? '✅ ALLOWED' : '❌ DENIED'}</strong>
            </div>
        </div>
    `;

    // Add approval requirement information right after access summary
    if (result.requiresApproval !== undefined) {
        html += `
            <div class="approval-info">
                <h4>Approval Requirement</h4>
                <div class="approval-status ${result.requiresApproval ? 'required' : 'not-required'}">
                    <strong>${result.requiresApproval ? '⚠️ APPROVAL REQUIRED' : '✅ NO APPROVAL REQUIRED'}</strong>
                    ${result.requiresApproval ? `
                        <p>⏳ This request will require manual approval before execution.</p>
                        <p><strong>Approval Timeout:</strong> ${result.approvalTimeout || 'N/A'}</p>
                        <p style="margin-top: 10px; font-weight: 500;">The request will be sent to configured approval providers (Webhook/Slack) and will wait for approval before proceeding.</p>
                    ` : `
                        <p>✓ This request does not match any approval patterns and will execute immediately without requiring approval.</p>
                    `}
                </div>
            </div>
        `;
    }

    html += `
        <div class="test-details">
            <h4>Test Details</h4>
            <table class="test-details-table">
                <tr><td><strong>Connection:</strong></td><td>${result.connection}</td></tr>
                <tr><td><strong>Connection Type:</strong></td><td>${result.connectionType || 'unknown'}</td></tr>
                <tr><td><strong>Role:</strong></td><td>${result.role}</td></tr>
                <tr><td><strong>Query Type:</strong></td><td>${result.query_type || 'http'}</td></tr>
                ${result.query_type === 'database' ?
                    `<tr><td><strong>Database Query:</strong></td><td><code style="white-space: pre-wrap; background: #f5f5f5; padding: 5px; display: block;">${result.query || 'N/A'}</code></td></tr>` :
                    `<tr><td><strong>Method:</strong></td><td>${result.method || 'N/A'}</td></tr>
                     <tr><td><strong>Path:</strong></td><td>${result.path || 'N/A'}</td></tr>`
                }
            </table>
        </div>
    `;

    // Add subquery validation results for database queries
    if (result.subquery_validation) {
        const validation = result.subquery_validation;
        html += `
            <div class="subquery-validation">
                <h4>Subquery Validation</h4>
                <div class="validation-summary">
                    <div class="validation-stats">
                        <span class="stat-item ${validation.is_allowed ? 'allowed' : 'blocked'}">
                            <strong>${validation.is_allowed ? '✅ ALL QUERIES ALLOWED' : '❌ SOME QUERIES BLOCKED'}</strong>
                        </span>
                        <span class="stat-item">Total: ${validation.total_queries}</span>
                        <span class="stat-item allowed">Allowed: ${validation.allowed_count}</span>
                        <span class="stat-item blocked">Blocked: ${validation.blocked_count}</span>
                    </div>
                </div>
                <div class="subqueries-list">
        `;

        validation.subqueries.forEach((subquery, index) => {
            const statusClass = subquery.is_allowed ? 'allowed' : 'blocked';
            const riskClass = `risk-${subquery.risk_level}`;

            html += `
                <div class="subquery-item ${statusClass} ${riskClass}">
                    <div class="subquery-header">
                        <span class="subquery-number">#${index + 1}</span>
                        <span class="subquery-type">${subquery.subquery.type.toUpperCase()}</span>
                        <span class="subquery-status ${statusClass}">
                            ${subquery.is_allowed ? '✅ ALLOWED' : '❌ BLOCKED'}
                        </span>
                        <span class="risk-level ${riskClass}">${subquery.risk_level.toUpperCase()}</span>
                    </div>
                    <div class="subquery-content">
                        <code class="subquery-code">${subquery.subquery.query}</code>
                    </div>
                    ${!subquery.is_allowed ? `
                        <div class="subquery-blocked">
                            <strong>Blocked by:</strong> ${subquery.blocked_by || 'No matching pattern'}
                            ${subquery.suggestions && subquery.suggestions.length > 0 ? `
                                <div class="suggestions">
                                    <strong>Suggestions:</strong>
                                    <ul>
                                        ${subquery.suggestions.map(s => `<li>${s}</li>`).join('')}
                                    </ul>
                                </div>
                            ` : ''}
                        </div>
                    ` : `
                        <div class="subquery-allowed">
                            <strong>Matched by:</strong> ${subquery.matched_by || 'No whitelist'}
                        </div>
                    `}
                </div>
            `;
        });

        html += `
                </div>
            </div>
        `;
    }

    if (result.matchingPolicies && result.matchingPolicies.length > 0) {
        html += `
            <div class="matching-policies">
                <h4>Matching Policies (${result.matchingPolicies.length})</h4>
                <div class="policies-list">
        `;

        result.matchingPolicies.forEach(policy => {
            html += `
                <div class="policy-item">
                    <div class="policy-name"><strong>${policy.name}</strong></div>
                    <div class="policy-details">
                        <span class="policy-roles">Roles: ${policy.roles.join(', ')}</span>
                        <span class="policy-tags">Tags: ${policy.tags.length > 0 ? policy.tags.join(', ') : 'none'}</span>
                        <span class="policy-match">Match: ${policy.tagMatch}</span>
                    </div>
                    <div class="policy-whitelist">
                        <strong>Whitelist Rules:</strong>
                        <ul>
                            ${policy.whitelist.map(rule => `<li><code>${rule}</code></li>`).join('')}
                        </ul>
                    </div>
                </div>
            `;
        });

        html += `
                </div>
            </div>
        `;
    } else {
        html += `
            <div class="no-policies">
                <h4>No Matching Policies</h4>
                <p>No policies were found that match the specified role and connection tags.</p>
            </div>
        `;
    }

    if (result.connectionTags && result.connectionTags.length > 0) {
        html += `
            <div class="connection-info">
                <h4>Connection Information</h4>
                <p><strong>Connection Tags:</strong> ${result.connectionTags.join(', ')}</p>
            </div>
        `;
    }

    contentDiv.innerHTML = html;
    resultsDiv.style.display = 'block';
}

// Initialize policy tester when tab is shown
function initPolicyTester() {
    loadPolicyTesterConnections();
}

// ============ Approval Management Functions ============

async function loadApprovals() {
    try {
        const config = await apiCall('/approvals');
        
        // Update enabled checkbox
        document.getElementById('approval-enabled').checked = config.enabled;
        
        // Store config for providers form
        window.currentApprovalConfig = config;
        
        // Load patterns
        const tbody = document.getElementById('approval-patterns-list');
        if (config.patterns.length === 0) {
            tbody.innerHTML = '<tr><td colspan="5">No approval patterns configured</td></tr>';
        } else {
            tbody.innerHTML = config.patterns.map(pattern => `
                <tr>
                    <td><code>${escapeHtml(pattern.pattern)}</code></td>
                    <td>${pattern.tags ? pattern.tags.join(', ') : '<em>All connections</em>'}</td>
                    <td>${pattern.tag_match || 'all'}</td>
                    <td>${pattern.timeout_seconds}s</td>
                    <td>
                        <button onclick="editApprovalPattern(${pattern.index})">Edit</button>
                        <button onclick="deleteApprovalPattern(${pattern.index})" class="danger">Delete</button>
                    </td>
                </tr>
            `).join('');
        }
        
        // Show provider configuration
        const providersInfo = document.getElementById('approval-providers-info');
        let providersHTML = '<h5>Configured Providers:</h5><ul>';
        
        if (config.webhook && config.webhook.url) {
            providersHTML += `<li><strong>Webhook:</strong> ${escapeHtml(config.webhook.url)}</li>`;
        }
        if (config.slack && config.slack.webhook_url) {
            providersHTML += `<li><strong>Slack:</strong> Configured (webhook URL hidden for security)</li>`;
        }
        
        if ((!config.webhook || !config.webhook.url) && (!config.slack || !config.slack.webhook_url)) {
            providersHTML += '<li><em>No approval providers configured. Click "Configure Providers" to add webhook or Slack integration.</em></li>';
        }
        
        providersHTML += '</ul>';
        providersInfo.innerHTML = providersHTML;
    } catch (error) {
        showNotification('Failed to load approvals: ' + error.message, 'error');
    }
}

async function toggleApprovalEnabled() {
    const enabled = document.getElementById('approval-enabled').checked;
    
    try {
        await apiCall('/approvals/enabled', {
            method: 'PUT',
            body: JSON.stringify({ enabled })
        });
        
        showNotification(`Approvals ${enabled ? 'enabled' : 'disabled'} successfully`);
    } catch (error) {
        // Revert checkbox on error
        document.getElementById('approval-enabled').checked = !enabled;
        showNotification('Failed to update approval status: ' + error.message, 'error');
    }
}

function showApprovalPatternForm() {
    document.getElementById('approval-pattern-form').style.display = 'block';
    document.getElementById('approval-pattern-form-title').textContent = 'Add Approval Pattern';
    document.getElementById('approvalPatternForm').reset();
    document.getElementById('approval-pattern-index').value = '';
    document.getElementById('approval-timeout').value = '300';
}

function hideApprovalPatternForm() {
    document.getElementById('approval-pattern-form').style.display = 'none';
    document.getElementById('approvalPatternForm').reset();
}

async function editApprovalPattern(index) {
    try {
        const config = await apiCall('/approvals');
        const pattern = config.patterns.find(p => p.index === index);
        
        if (!pattern) {
            showNotification('Pattern not found', 'error');
            return;
        }
        
        document.getElementById('approval-pattern-form').style.display = 'block';
        document.getElementById('approval-pattern-form-title').textContent = 'Edit Approval Pattern';
        document.getElementById('approval-pattern-index').value = index;
        document.getElementById('approval-pattern').value = pattern.pattern;
        document.getElementById('approval-tags').value = pattern.tags ? pattern.tags.join(', ') : '';
        document.getElementById('approval-tag-match').value = pattern.tag_match || 'all';
        document.getElementById('approval-timeout').value = pattern.timeout_seconds;
    } catch (error) {
        showNotification('Failed to load pattern: ' + error.message, 'error');
    }
}

async function saveApprovalPattern(event) {
    event.preventDefault();
    
    const index = document.getElementById('approval-pattern-index').value;
    const isEdit = index !== '';
    
    const tags = document.getElementById('approval-tags').value
        .split(',')
        .map(t => t.trim())
        .filter(t => t !== '');
    
    const pattern = {
        pattern: document.getElementById('approval-pattern').value,
        tags: tags.length > 0 ? tags : null,
        tag_match: document.getElementById('approval-tag-match').value,
        timeout_seconds: parseInt(document.getElementById('approval-timeout').value)
    };
    
    try {
        if (isEdit) {
            await apiCall(`/approvals/patterns/${index}`, {
                method: 'PUT',
                body: JSON.stringify(pattern)
            });
            showNotification('Approval pattern updated successfully');
        } else {
            await apiCall('/approvals/patterns', {
                method: 'POST',
                body: JSON.stringify(pattern)
            });
            showNotification('Approval pattern created successfully');
        }
        
        hideApprovalPatternForm();
        await loadApprovals();
    } catch (error) {
        showNotification('Failed to save approval pattern: ' + error.message, 'error');
    }
}

async function deleteApprovalPattern(index) {
    if (!confirm('Are you sure you want to delete this approval pattern?')) {
        return;
    }
    
    try {
        await apiCall(`/approvals/patterns/${index}`, {
            method: 'DELETE'
        });
        showNotification('Approval pattern deleted successfully');
        await loadApprovals();
    } catch (error) {
        showNotification('Failed to delete approval pattern: ' + error.message, 'error');
    }
}

function showProvidersForm() {
    const config = window.currentApprovalConfig || {};
    
    // Populate form with current values
    document.getElementById('provider-webhook-url').value = (config.webhook && config.webhook.url) || '';
    document.getElementById('provider-slack-url').value = (config.slack && config.slack.webhook_url) || '';
    
    document.getElementById('providers-form').style.display = 'block';
}

function hideProvidersForm() {
    document.getElementById('providers-form').style.display = 'none';
    document.getElementById('providersForm').reset();
}

async function saveProviders(event) {
    event.preventDefault();
    
    const webhookUrl = document.getElementById('provider-webhook-url').value.trim();
    const slackUrl = document.getElementById('provider-slack-url').value.trim();
    
    const providers = {};
    
    // Only include non-empty URLs
    if (webhookUrl) {
        providers.webhook = { url: webhookUrl };
    } else {
        providers.webhook = { url: '' }; // Empty string to clear
    }
    
    if (slackUrl) {
        providers.slack = { webhook_url: slackUrl };
    } else {
        providers.slack = { webhook_url: '' }; // Empty string to clear
    }
    
    try {
        await apiCall('/approvals/providers', {
            method: 'PUT',
            body: JSON.stringify(providers)
        });
        
        showNotification('Approval providers updated successfully');
        hideProvidersForm();
        await loadApprovals();
    } catch (error) {
        showNotification('Failed to update approval providers: ' + error.message, 'error');
    }
}

// ============ URL Routing and Navigation ============

// Handle browser back/forward buttons
window.addEventListener('popstate', (event) => {
    if (event.state && event.state.tab) {
        showTab(event.state.tab, null, false);
    } else {
        // Check URL parameter
        const urlParams = new URLSearchParams(window.location.search);
        const tab = urlParams.get('tab') || 'dashboard';
        showTab(tab, null, false);
    }
});

// Initialize tab from URL on page load
window.addEventListener('DOMContentLoaded', () => {
    const urlParams = new URLSearchParams(window.location.search);
    const tab = urlParams.get('tab');
    
    if (tab && document.getElementById(tab)) {
        // Show tab from URL
        showTab(tab, null, false);
    } else {
        // Default to dashboard, but set initial history state
        const url = new URL(window.location);
        url.searchParams.set('tab', 'dashboard');
        window.history.replaceState({ tab: 'dashboard' }, '', url);
    }
});

