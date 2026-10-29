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

// Cache for K8s resources (ConfigMaps and Secrets)
let k8sResourcesCache = null;

// Fetch available ConfigMaps and Secrets from K8s
async function fetchK8sResources() {
    if (k8sResourcesCache) {
        return k8sResourcesCache;
    }
    
    try {
        const data = await apiCall('/k8s/resources');
        k8sResourcesCache = data;
        return data;
    } catch (error) {
        console.warn('Failed to fetch K8s resources:', error);
        return { configmaps: [], secrets: [], namespace: '' };
    }
}

// Custom autocomplete functionality
function setupAutocomplete(inputId, options) {
    const input = document.getElementById(inputId);
    const dropdown = document.getElementById(`${inputId}-dropdown`);
    if (!input || !dropdown) return;
    
    let selectedIndex = -1;
    let filteredOptions = [...options];
    
    // Function to filter and display options
    function showDropdown(filterText = '') {
        filterText = filterText.toLowerCase();
        filteredOptions = options.filter(opt => opt.toLowerCase().includes(filterText));
        
        dropdown.innerHTML = '';
        
        if (filteredOptions.length === 0) {
            const noResults = document.createElement('div');
            noResults.className = 'autocomplete-option no-results';
            noResults.textContent = 'No matches found';
            dropdown.appendChild(noResults);
        } else {
            filteredOptions.forEach((option, index) => {
                const div = document.createElement('div');
                div.className = 'autocomplete-option';
                div.textContent = option;
                div.addEventListener('click', () => {
                    input.value = option;
                    dropdown.classList.remove('show');
                    input.focus();
                });
                div.addEventListener('mouseenter', () => {
                    selectedIndex = index;
                    updateSelection();
                });
                dropdown.appendChild(div);
            });
        }
        
        selectedIndex = -1;
        updateSelection();
        dropdown.classList.add('show');
    }
    
    function updateSelection() {
        const items = dropdown.querySelectorAll('.autocomplete-option:not(.no-results)');
        items.forEach((item, index) => {
            if (index === selectedIndex) {
                item.classList.add('selected');
            } else {
                item.classList.remove('selected');
            }
        });
    }
    
    // Show dropdown on focus or click
    input.addEventListener('focus', () => {
        showDropdown(input.value);
    });
    
    input.addEventListener('click', () => {
        showDropdown(input.value);
    });
    
    // Filter as user types
    input.addEventListener('input', (e) => {
        showDropdown(e.target.value);
    });
    
    // Keyboard navigation
    input.addEventListener('keydown', (e) => {
        const items = dropdown.querySelectorAll('.autocomplete-option:not(.no-results)');
        
        if (e.key === 'ArrowDown') {
            e.preventDefault();
            if (!dropdown.classList.contains('show')) {
                showDropdown(input.value);
            } else {
                selectedIndex = Math.min(selectedIndex + 1, filteredOptions.length - 1);
                updateSelection();
            }
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            selectedIndex = Math.max(selectedIndex - 1, -1);
            updateSelection();
        } else if (e.key === 'Enter') {
            if (selectedIndex >= 0 && selectedIndex < filteredOptions.length) {
                e.preventDefault();
                input.value = filteredOptions[selectedIndex];
                dropdown.classList.remove('show');
            }
        } else if (e.key === 'Escape') {
            dropdown.classList.remove('show');
        }
    });
    
    // Close dropdown when clicking outside
    document.addEventListener('click', (e) => {
        if (!input.contains(e.target) && !dropdown.contains(e.target)) {
            dropdown.classList.remove('show');
        }
    });
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

        // Update active connections list
        updateActiveConnectionsList(status.active_connections_list || []);

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

// Update active connections list in the dashboard
function updateActiveConnectionsList(connections) {
    const tbody = document.getElementById('active-connections-tbody');
    if (!tbody) return;

    if (!connections || connections.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; color: var(--text-muted);">No active connections</td></tr>';
        return;
    }

    tbody.innerHTML = connections.map(conn => {
        const createdAt = new Date(conn.created_at).toLocaleString();
        const expiresAt = new Date(conn.expires_at).toLocaleString();
        const typeClass = conn.connection_type === 'k8s_services' ? 'type-k8s' : '';
        
        return `
            <tr>
                <td>${escapeHtml(conn.username)}</td>
                <td><strong>${escapeHtml(conn.connection_name)}</strong></td>
                <td><span class="conn-type ${typeClass}">${escapeHtml(conn.connection_type)}</span></td>
                <td>${conn.active_streams}</td>
                <td>${createdAt}</td>
                <td>${expiresAt}</td>
            </tr>
        `;
    }).join('');
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

        // Fetch config errors to show warnings
        let configErrors = null;
        try {
            const errorData = await apiCall('/config/errors');
            configErrors = errorData?.errors?.connections || {};
        } catch (err) {
            console.warn('Failed to fetch config errors:', err);
        }

        connections.forEach(conn => {
            const row = document.createElement('tr');
            const tags = conn.tags ? conn.tags.join(', ') : 'none';
            const duration = conn.duration || 'default';
            
            // Check for missing ConfigMaps/Secrets
            let warningBadge = '';
            if (configErrors && configErrors[conn.name]) {
                const connErrors = configErrors[conn.name];
                if ((connErrors.missing_configmaps && connErrors.missing_configmaps.length > 0) || 
                    (connErrors.missing_secrets && connErrors.missing_secrets.length > 0)) {
                    const missingResources = [
                        ...(connErrors.missing_configmaps || []).map(cm => `ConfigMap: ${cm}`),
                        ...(connErrors.missing_secrets || []).map(s => `Secret: ${s}`)
                    ].join(', ');
                    warningBadge = ` <span class="warning-badge" title="Missing: ${escapeHtml(missingResources)}" style="color: orange; cursor: help;">⚠️</span>`;
                }
            }

            // Handle k8s_services type display differently
            let hostDisplay = escapeHtml(conn.host || '-');
            let portDisplay = conn.port || '-';
            
            if (conn.type === 'k8s_services') {
                // Show namespace info instead of host/port
                if (conn.k8s_include_namespaces && conn.k8s_include_namespaces.length > 0) {
                    hostDisplay = `<span title="Include: ${escapeHtml(conn.k8s_include_namespaces.join(', '))}">☸️ ${conn.k8s_include_namespaces.length} namespace(s)</span>`;
                } else if (conn.k8s_exclude_namespaces && conn.k8s_exclude_namespaces.length > 0) {
                    hostDisplay = `<span title="Exclude: ${escapeHtml(conn.k8s_exclude_namespaces.join(', '))}">☸️ All (excl. ${conn.k8s_exclude_namespaces.length})</span>`;
                } else {
                    hostDisplay = '☸️ All namespaces';
                }
                portDisplay = 'Auto';
            }

            row.innerHTML = `
                <td>${escapeHtml(conn.name || 'unnamed')}${warningBadge}</td>
                <td>${escapeHtml(conn.type || 'unknown')}</td>
                <td>${hostDisplay}</td>
                <td>${portDisplay}</td>
                <td>${escapeHtml(duration)}</td>
                <td>${escapeHtml(tags)}</td>
                <td class="action-buttons">
                    <button onclick="editConnection('${escapeHtml(conn.name)}')">Edit</button>
                    <button class="danger" onclick="deleteConnection('${escapeHtml(conn.name)}')">Delete</button>
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
    document.getElementById('connection-form-modal').style.display = 'flex';
    document.getElementById('connection-form-title').textContent = 'Add Connection';
    document.getElementById('connectionForm').reset();
    document.getElementById('connection-original-name').value = '';
    
    // Reset ConfigSource fields to plain
    document.getElementById('conn-backend-username-type').value = 'plain';
    document.getElementById('conn-backend-password-type').value = 'plain';
    document.getElementById('conn-redis-username-type').value = ''; // None by default
    document.getElementById('conn-redis-password-type').value = 'plain';
    toggleConfigSourceFields('username');
    toggleConfigSourceFields('password');
    toggleConfigSourceFields('redis-username');
    toggleConfigSourceFields('redis-password');
    
    // Enable TLS by default for new connections
    document.getElementById('conn-tls-enabled').checked = true;
    toggleTLSOptions();
    
    // Clear K8s port mappings
    clearK8sPortMappings();
    
    // Reset field visibility
    updateConnectionFields();
    
    // Prevent body scroll
    document.body.style.overflow = 'hidden';
}

// Toggle between plain value and ConfigMap/Secret fields
async function toggleConfigSourceFields(fieldType) {
    // Handle Redis fields separately (different field naming)
    const isRedisField = fieldType === 'redis-password' || fieldType === 'redis-username';
    const typeSelectId = isRedisField
        ? `conn-${fieldType}-type` 
        : `conn-backend-${fieldType}-type`;
    
    const typeSelect = document.getElementById(typeSelectId);
    if (!typeSelect) {
        console.error(`Type select not found: ${typeSelectId}`);
        return;
    }
    
    const type = typeSelect.value;
    const plainFields = document.getElementById(`${fieldType}-plain-fields`);
    const k8sFields = document.getElementById(`${fieldType}-k8s-fields`);
    
    // Handle empty/None value for redis-username (legacy auth)
    if (fieldType === 'redis-username' && (type === '' || type === 'None')) {
        if (plainFields) plainFields.style.display = 'none';
        if (k8sFields) k8sFields.style.display = 'none';
        return;
    }
    
    if (type === 'plain') {
        if (plainFields) plainFields.style.display = 'block';
        if (k8sFields) k8sFields.style.display = 'none';
    } else {
        if (plainFields) plainFields.style.display = 'none';
        if (k8sFields) k8sFields.style.display = 'block';
        
        // Fetch and populate K8s resources for autocomplete
        const resources = await fetchK8sResources();
        // Handle Redis fields differently (uses conn-redis-*-ref instead of conn-backend-*)
        const inputId = isRedisField
            ? `conn-${fieldType}-ref` 
            : `conn-backend-${fieldType}-ref`;
        const options = type === 'configmap' ? resources.configmaps : resources.secrets;
        setupAutocomplete(inputId, options);
    }
}

function hideConnectionForm() {
    document.getElementById('connection-form-modal').style.display = 'none';
    document.getElementById('connectionForm').reset();
    
    // Re-enable body scroll
    document.body.style.overflow = '';
}

// Close modal when clicking outside
document.addEventListener('DOMContentLoaded', function() {
    const connectionModal = document.getElementById('connection-form-modal');
    if (connectionModal) {
        connectionModal.addEventListener('click', function(e) {
            if (e.target === connectionModal) {
                hideConnectionForm();
            }
        });
    }
    
    const providersModal = document.getElementById('providers-form-modal');
    if (providersModal) {
        providersModal.addEventListener('click', function(e) {
            if (e.target === providersModal) {
                hideProvidersForm();
            }
        });
    }
    
    const approvalPatternModal = document.getElementById('approval-pattern-form-modal');
    if (approvalPatternModal) {
        approvalPatternModal.addEventListener('click', function(e) {
            if (e.target === approvalPatternModal) {
                hideApprovalPatternForm();
            }
        });
    }
});

// Show/hide fields based on connection type
function updateConnectionFields() {
    const type = document.getElementById('conn-type').value;
    const postgresFields = document.getElementById('postgres-fields');
    const redisFields = document.getElementById('redis-fields');
    const tlsFields = document.getElementById('tls-fields');
    const hostPortFields = document.getElementById('host-port-fields');
    const k8sServicesFields = document.getElementById('k8s-services-fields');
    
    // Hide all type-specific fields first
    postgresFields.style.display = 'none';
    redisFields.style.display = 'none';
    tlsFields.style.display = 'none';
    k8sServicesFields.style.display = 'none';
    
    // Show/hide host/port based on type
    if (type === 'k8s_services') {
        hostPortFields.style.display = 'none';
        k8sServicesFields.style.display = 'block';
        // Remove required from host/port for k8s_services
        document.getElementById('conn-host').removeAttribute('required');
        document.getElementById('conn-port').removeAttribute('required');
    } else {
        hostPortFields.style.display = 'block';
        // Re-add required for other types
        document.getElementById('conn-host').setAttribute('required', 'required');
        document.getElementById('conn-port').setAttribute('required', 'required');
    }
    
    // Show relevant fields based on type
    if (type === 'postgres') {
        postgresFields.style.display = 'block';
    } else if (type === 'redis') {
        redisFields.style.display = 'block';
        tlsFields.style.display = 'block'; // Show TLS ONLY for redis
    }
    // For http/https, we infer the scheme from the type itself
}

// Toggle TLS options visibility based on checkbox
function toggleTLSOptions() {
    const tlsEnabled = document.getElementById('conn-tls-enabled').checked;
    const tlsOptions = document.getElementById('tls-options');
    
    if (tlsEnabled) {
        tlsOptions.style.display = 'block';
    } else {
        tlsOptions.style.display = 'none';
    }
}

// K8s Port Mappings management
let k8sPortMappingCounter = 0;

function addK8sPortMapping(port = '', protocol = 'http') {
    const container = document.getElementById('k8s-port-mappings-container');
    const id = k8sPortMappingCounter++;
    
    const row = document.createElement('div');
    row.className = 'k8s-port-mapping-row';
    row.id = `k8s-port-mapping-${id}`;
    row.style.cssText = 'display: flex; gap: 10px; align-items: center; margin-bottom: 8px; padding: 8px; background: #1e1e1e; border-radius: 4px;';
    
    row.innerHTML = `
        <input type="number" placeholder="Port (e.g., 8080)" value="${port}" 
               class="k8s-pm-port" style="width: 100px; padding: 6px;">
        <span style="color: #666;">→</span>
        <select class="k8s-pm-protocol" style="padding: 6px;">
            <option value="http" ${protocol === 'http' ? 'selected' : ''}>HTTP</option>
            <option value="https" ${protocol === 'https' ? 'selected' : ''}>HTTPS</option>
            <option value="postgres" ${protocol === 'postgres' ? 'selected' : ''}>PostgreSQL</option>
            <option value="redis" ${protocol === 'redis' ? 'selected' : ''}>Redis</option>
            <option value="tcp" ${protocol === 'tcp' ? 'selected' : ''}>TCP (raw)</option>
        </select>
        <button type="button" onclick="removeK8sPortMapping(${id})" 
                style="background: #dc3545; color: white; border: none; padding: 6px 10px; border-radius: 4px; cursor: pointer;">✕</button>
    `;
    
    container.appendChild(row);
}

function removeK8sPortMapping(id) {
    const row = document.getElementById(`k8s-port-mapping-${id}`);
    if (row) {
        row.remove();
    }
}

function clearK8sPortMappings() {
    const container = document.getElementById('k8s-port-mappings-container');
    container.innerHTML = '';
    k8sPortMappingCounter = 0;
}

function getK8sPortMappings() {
    const container = document.getElementById('k8s-port-mappings-container');
    const rows = container.querySelectorAll('.k8s-port-mapping-row');
    const mappings = [];
    
    rows.forEach(row => {
        const portValue = row.querySelector('.k8s-pm-port').value.trim();
        const protocol = row.querySelector('.k8s-pm-protocol').value;
        
        if (portValue) {
            mappings.push({
                port: parseInt(portValue),
                protocol: protocol
            });
        }
    });
    
    return mappings;
}

function populateK8sPortMappings(mappings) {
    clearK8sPortMappings();
    if (mappings && Array.isArray(mappings)) {
        mappings.forEach(m => {
            addK8sPortMapping(m.port || '', m.protocol || 'http');
        });
    }
}

async function editConnection(name) {
    try {
        const connections = await apiCall('/connections');
        const conn = connections.find(c => c.name === name);

        if (!conn) {
            showNotification('Connection not found', 'error');
            return;
        }

        document.getElementById('connection-form-modal').style.display = 'flex';
        document.getElementById('connection-form-title').textContent = 'Edit Connection';
        document.getElementById('connection-original-name').value = name;
        
        // Prevent body scroll
        document.body.style.overflow = 'hidden';
        document.getElementById('conn-name').value = conn.name;
        document.getElementById('conn-type').value = conn.type;
        document.getElementById('conn-host').value = conn.host;
        document.getElementById('conn-port').value = conn.port;
        document.getElementById('conn-tags').value = conn.tags ? conn.tags.join(', ') : '';
        
        // Update type-specific fields first
        updateConnectionFields();
        
        // Handle type-specific field population
        if (conn.type === 'postgres') {
            // Populate backend username (ConfigSource)
            console.log('Backend username data:', conn.backend_username);
            if (conn.backend_username) {
                if (typeof conn.backend_username === 'string') {
                    // Legacy plain string
                    console.log('backend_username is a string:', conn.backend_username);
                    document.getElementById('conn-backend-username-type').value = 'plain';
                    toggleConfigSourceFields('username');
                    document.getElementById('conn-backend-username').value = conn.backend_username;
                } else if (conn.backend_username.type) {
                    // ConfigSource object
                    console.log('backend_username is ConfigSource, type:', conn.backend_username.type);
                    document.getElementById('conn-backend-username-type').value = conn.backend_username.type || 'plain';
                    toggleConfigSourceFields('username');
                    if (conn.backend_username.type === 'plain' || !conn.backend_username.type) {
                        document.getElementById('conn-backend-username').value = conn.backend_username.value || '';
                    } else {
                        document.getElementById('conn-backend-username-ref').value = conn.backend_username.ref || '';
                        document.getElementById('conn-backend-username-refname').value = conn.backend_username.ref_name || '';
                        document.getElementById('conn-backend-username-fallback').value = conn.backend_username.value || '';
                    }
                } else {
                    console.log('backend_username exists but no type field:', conn.backend_username);
                }
            } else {
                console.log('No backend_username found');
                // Default to plain if no username
                document.getElementById('conn-backend-username-type').value = 'plain';
                toggleConfigSourceFields('username');
            }
            
            // Don't populate password field - let user enter new password or leave empty to keep existing
            // But do show the type if it was a ConfigSource
            if (conn.backend_password && typeof conn.backend_password === 'object' && conn.backend_password.type) {
                document.getElementById('conn-backend-password-type').value = conn.backend_password.type;
                toggleConfigSourceFields('password');
                if (conn.backend_password.type !== 'plain') {
                    document.getElementById('conn-backend-password-ref').value = conn.backend_password.ref || '';
                    document.getElementById('conn-backend-password-refname').value = conn.backend_password.ref_name || '';
                    // Don't show fallback value for security
                }
            } else {
                // Default to plain if no password info
                document.getElementById('conn-backend-password-type').value = 'plain';
                toggleConfigSourceFields('password');
            }
            
            document.getElementById('conn-backend-database').value = conn.backend_database || '';
            document.getElementById('conn-backend-params').value =
                conn.backend_params && Object.keys(conn.backend_params).length > 0
                    ? JSON.stringify(conn.backend_params, null, 2) : '';
        } else if (conn.type === 'redis') {
            // Handle Redis username (Redis 6.0+ ACL)
            if (conn.backend_username) {
                if (typeof conn.backend_username === 'string') {
                    // Legacy plain string
                    document.getElementById('conn-redis-username-type').value = 'plain';
                    toggleConfigSourceFields('redis-username');
                    document.getElementById('conn-redis-username').value = conn.backend_username;
                } else if (conn.backend_username.type) {
                    // ConfigSource object
                    document.getElementById('conn-redis-username-type').value = conn.backend_username.type || 'plain';
                    toggleConfigSourceFields('redis-username');
                    if (conn.backend_username.type === 'plain' || !conn.backend_username.type) {
                        document.getElementById('conn-redis-username').value = conn.backend_username.value || '';
                    } else {
                        document.getElementById('conn-redis-username-ref').value = conn.backend_username.ref || '';
                        document.getElementById('conn-redis-username-refname').value = conn.backend_username.ref_name || '';
                        document.getElementById('conn-redis-username-fallback').value = conn.backend_username.value || '';
                    }
                }
            } else {
                // Default to no username (legacy auth)
                document.getElementById('conn-redis-username-type').value = '';
                toggleConfigSourceFields('redis-username');
            }
            
            // Handle Redis password
            if (conn.backend_password && typeof conn.backend_password === 'object' && conn.backend_password.type) {
                document.getElementById('conn-redis-password-type').value = conn.backend_password.type;
                toggleConfigSourceFields('redis-password');
                if (conn.backend_password.type !== 'plain') {
                    document.getElementById('conn-redis-password-ref').value = conn.backend_password.ref || '';
                    document.getElementById('conn-redis-password-refname').value = conn.backend_password.ref_name || '';
                    // Don't show fallback value for security
                }
            } else {
                // Default to plain if no password info
                document.getElementById('conn-redis-password-type').value = 'plain';
                toggleConfigSourceFields('redis-password');
            }
        }
        
        // Populate TLS configuration (nested structure)
        if (conn.tls && conn.tls.enabled) {
            document.getElementById('conn-tls-enabled').checked = true;
            document.getElementById('conn-tls-server-name').value = conn.tls.server_name || '';
            document.getElementById('conn-tls-skip-verify').checked = conn.tls.skip_verify || false;
            document.getElementById('conn-tls-ca-cert').value = conn.tls.ca_cert || '';
            document.getElementById('conn-tls-client-cert').value = conn.tls.client_cert || '';
            document.getElementById('conn-tls-client-key').value = conn.tls.client_key || '';
        } else {
            document.getElementById('conn-tls-enabled').checked = false;
        }
        toggleTLSOptions();
        
        // Populate K8s Services configuration
        if (conn.type === 'k8s_services') {
            document.getElementById('conn-k8s-include-namespaces').value = 
                conn.k8s_include_namespaces ? conn.k8s_include_namespaces.join(', ') : '';
            document.getElementById('conn-k8s-exclude-namespaces').value = 
                conn.k8s_exclude_namespaces ? conn.k8s_exclude_namespaces.join(', ') : '';
            document.getElementById('conn-k8s-ignore-services').value = 
                conn.k8s_ignore_services ? conn.k8s_ignore_services.join(', ') : '';
            // Populate port mappings
            populateK8sPortMappings(conn.k8s_port_mappings);
        } else {
            // Clear port mappings for non-k8s_services types
            clearK8sPortMappings();
        }
        
        // Populate duration (now comes as string from API)
        document.getElementById('conn-duration').value = conn.duration || '';
        
        // Populate metadata as JSON string
        if (conn.metadata && Object.keys(conn.metadata).length > 0) {
            document.getElementById('conn-metadata').value = JSON.stringify(conn.metadata, null, 2);
        } else {
            document.getElementById('conn-metadata').value = '';
        }
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
        tags: tags
    };
    
    // Add host/port only for non-k8s_services types
    if (connType !== 'k8s_services') {
        connection.host = document.getElementById('conn-host').value;
        connection.port = parseInt(document.getElementById('conn-port').value);
    } else {
        // K8s Services specific configuration
        const includeNamespaces = document.getElementById('conn-k8s-include-namespaces').value
            .split(',')
            .map(ns => ns.trim())
            .filter(ns => ns !== '');
        const excludeNamespaces = document.getElementById('conn-k8s-exclude-namespaces').value
            .split(',')
            .map(ns => ns.trim())
            .filter(ns => ns !== '');
        const ignoreServices = document.getElementById('conn-k8s-ignore-services').value
            .split(',')
            .map(s => s.trim())
            .filter(s => s !== '');
        
        if (includeNamespaces.length > 0) {
            connection.k8s_include_namespaces = includeNamespaces;
        }
        if (excludeNamespaces.length > 0) {
            connection.k8s_exclude_namespaces = excludeNamespaces;
        }
        if (ignoreServices.length > 0) {
            connection.k8s_ignore_services = ignoreServices;
        }
        
        // Add port mappings
        const portMappings = getK8sPortMappings();
        if (portMappings.length > 0) {
            connection.k8s_port_mappings = portMappings;
        }
    }

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
        // Build backend_username ConfigSource
        const usernameType = document.getElementById('conn-backend-username-type').value;
        if (usernameType === 'plain') {
            const username = document.getElementById('conn-backend-username').value;
            if (username) {
                connection.backend_username = {
                    type: 'plain',
                    value: username
                };
            }
        } else {
            connection.backend_username = {
                type: usernameType,
                ref: document.getElementById('conn-backend-username-ref').value,
                ref_name: document.getElementById('conn-backend-username-refname').value,
                value: document.getElementById('conn-backend-username-fallback').value || ''
            };
        }
        
        // Build backend_password ConfigSource
        const passwordType = document.getElementById('conn-backend-password-type').value;
        if (passwordType === 'plain') {
            const password = document.getElementById('conn-backend-password').value;
            // Only include password if it's provided (not empty)
            if (password) {
                connection.backend_password = {
                    type: 'plain',
                    value: password
                };
            }
        } else {
            // For ConfigMap/Secret, always include the reference even if password field is empty
            const ref = document.getElementById('conn-backend-password-ref').value;
            const refName = document.getElementById('conn-backend-password-refname').value;
            if (ref && refName) {
                connection.backend_password = {
                    type: passwordType,
                    ref: ref,
                    ref_name: refName,
                    value: document.getElementById('conn-backend-password-fallback').value || ''
                };
            }
        }
        
        connection.backend_database = document.getElementById('conn-backend-database').value;

        // Always send an object: {} clears params, an omitted field would keep the old ones
        const paramsStr = document.getElementById('conn-backend-params').value.trim();
        try {
            connection.backend_params = paramsStr ? JSON.parse(paramsStr) : {};
        } catch (e) {
            showNotification('Invalid JSON in backend params field', 'error');
            return;
        }
        const params = connection.backend_params;
        if (params === null || typeof params !== 'object' || Array.isArray(params) ||
            Object.values(params).some(v => typeof v !== 'string')) {
            showNotification('Backend params must be a JSON object with string values', 'error');
            return;
        }
    } else if (connType === 'redis') {
        // Build backend_username ConfigSource (Redis 6.0+ ACL, optional)
        const usernameType = document.getElementById('conn-redis-username-type').value;
        if (usernameType && usernameType !== '') {
            if (usernameType === 'plain') {
                const username = document.getElementById('conn-redis-username').value;
                if (username) {
                    connection.backend_username = {
                        type: 'plain',
                        value: username
                    };
                }
            } else {
                const ref = document.getElementById('conn-redis-username-ref').value;
                const refName = document.getElementById('conn-redis-username-refname').value;
                if (ref && refName) {
                    connection.backend_username = {
                        type: usernameType,
                        ref: ref,
                        ref_name: refName,
                        value: document.getElementById('conn-redis-username-fallback').value || ''
                    };
                }
            }
        }
        
        // Build backend_password ConfigSource (Redis password, optional)
        const passwordType = document.getElementById('conn-redis-password-type').value;
        if (passwordType === 'plain') {
            const password = document.getElementById('conn-redis-password').value;
            // Only include password if it's provided (not empty)
            if (password) {
                connection.backend_password = {
                    type: 'plain',
                    value: password
                };
            }
        } else {
            // For ConfigMap/Secret, always include the reference even if password field is empty
            const ref = document.getElementById('conn-redis-password-ref').value;
            const refName = document.getElementById('conn-redis-password-refname').value;
            if (ref && refName) {
                connection.backend_password = {
                    type: passwordType,
                    ref: ref,
                    ref_name: refName,
                    value: document.getElementById('conn-redis-password-fallback').value || ''
                };
            }
        }
    }
    
    // Add TLS configuration (ONLY for Redis) - nested structure
    if (connType === 'redis') {
        const tlsEnabled = document.getElementById('conn-tls-enabled').checked;
        
        if (tlsEnabled) {
            // Build nested TLS object
            const tlsConfig = {
                enabled: true
            };
            
            // Add optional TLS fields if they have values
            const tlsServerName = document.getElementById('conn-tls-server-name').value.trim();
            if (tlsServerName) {
                tlsConfig.server_name = tlsServerName;
            }
            
            const tlsSkipVerify = document.getElementById('conn-tls-skip-verify').checked;
            if (tlsSkipVerify) {
                tlsConfig.skip_verify = true;
            }
            
            const tlsCACert = document.getElementById('conn-tls-ca-cert').value.trim();
            if (tlsCACert) {
                tlsConfig.ca_cert = tlsCACert;
            }
            
            const tlsClientCert = document.getElementById('conn-tls-client-cert').value.trim();
            if (tlsClientCert) {
                tlsConfig.client_cert = tlsClientCert;
            }
            
            const tlsClientKey = document.getElementById('conn-tls-client-key').value.trim();
            if (tlsClientKey) {
                tlsConfig.client_key = tlsClientKey;
            }
            
            connection.tls = tlsConfig;
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

// Normalize ConfigSource objects for consistent comparison
function normalizeConfigSource(obj) {
    if (!obj || typeof obj !== 'object') return obj;
    
    if (Array.isArray(obj)) {
        return obj.map(item => normalizeConfigSource(item));
    }
    
    const normalized = {};
    
    // Sort keys for consistent serialization
    Object.keys(obj).sort().forEach(key => {
        const value = obj[key];
        
        // Normalize ConfigSource objects
        if (value && typeof value === 'object' && 'type' in value && ('value' in value || 'ref' in value)) {
            // This looks like a ConfigSource
            normalized[key] = {
                type: value.type || 'plain',
                ...(value.value !== undefined && value.value !== '' && { value: value.value }),
                ...(value.ref && { ref: value.ref }),
                ...(value.ref_name && { ref_name: value.ref_name })
            };
        } else if (typeof value === 'string' && key.includes('password') || key.includes('secret') || key.includes('webhook')) {
            // Legacy plain string - convert to ConfigSource for comparison
            normalized[key] = {
                type: 'plain',
                value: value
            };
        } else if (value && typeof value === 'object') {
            normalized[key] = normalizeConfigSource(value);
        } else {
            normalized[key] = value;
        }
    });
    
    return normalized;
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

        // Normalize both configs before comparison
        const normalizedA = normalizeConfigSource(configA[section] || {});
        const normalizedB = normalizeConfigSource(configB[section] || {});
        
        const jsonA = JSON.stringify(normalizedA, null, 2);
        const jsonB = JSON.stringify(normalizedB, null, 2);

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

// OIDC Login function
function handleOIDCLogin() {
    // Generate random state for CSRF protection
    const state = Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15);
    localStorage.setItem('oidc_state', state);
    localStorage.setItem('oidc_return_to', 'admin');
    
    // Redirect to OIDC login endpoint with state
    // The callback will redirect back to /admin with token
    window.location.href = `/api/auth/oidc/login?state=${state}&cli_callback=${encodeURIComponent(window.location.origin + '/admin/callback')}`;
}

// Handle OIDC callback (if coming from OIDC provider)
function handleOIDCCallback() {
    const urlParams = new URLSearchParams(window.location.search);
    const tokenData = urlParams.get('token_data');
    const state = urlParams.get('state');
    
    if (!tokenData || !state) {
        return false; // Not an OIDC callback
    }
    
    // Verify state matches
    const savedState = localStorage.getItem('oidc_state');
    if (state !== savedState) {
        showNotification('OIDC state mismatch - possible CSRF attack', 'error');
        return true;
    }
    
    try {
        // Decode token data (base64 URL encoded JSON)
        const decoded = atob(tokenData);
        const loginResp = JSON.parse(decoded);
        
        token = loginResp.token;
        localStorage.setItem('token', token);
        localStorage.removeItem('oidc_state');
        localStorage.removeItem('oidc_return_to');
        
        // Check if user has admin role
        try {
            const payload = JSON.parse(atob(token.split('.')[1]));
            if (!payload.roles || !payload.roles.includes('admin')) {
                throw new Error('Admin role required');
            }
        } catch (e) {
            showNotification('Admin role required to access this area', 'error');
            localStorage.removeItem('token');
            token = null;
            // Clear URL parameters and show login
            window.history.replaceState({}, document.title, '/admin');
            showLogin();
            return true;
        }
        
        // Clear URL parameters
        window.history.replaceState({}, document.title, '/admin');
        
        // Show app
        showApp();
        showNotification('Successfully logged in via OIDC', 'success');
        
        return true;
    } catch (error) {
        console.error('OIDC callback error:', error);
        showNotification('OIDC login failed: ' + error.message, 'error');
        localStorage.removeItem('token');
        localStorage.removeItem('oidc_state');
        localStorage.removeItem('oidc_return_to');
        token = null;
        window.history.replaceState({}, document.title, '/admin');
        showLogin();
        return true;
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
    // Check if this is an OIDC callback
    if (handleOIDCCallback()) {
        return; // OIDC callback handled
    }
    
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
            tbody.innerHTML = '<tr><td colspan="7">No approval patterns configured</td></tr>';
        } else {
            tbody.innerHTML = config.patterns.map(pattern => `
                <tr>
                    <td><code>${escapeHtml(pattern.pattern)}</code></td>
                    <td>${pattern.roles && pattern.roles.length > 0 ? pattern.roles.join(', ') : '<em>All roles</em>'}</td>
                    <td>${pattern.role_match || 'any'}</td>
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
        
        // Helper to display ConfigSource
        const displayConfigSource = (cs) => {
            if (!cs) return null;
            if (typeof cs === 'string') return cs; // Legacy plain string
            if (cs.type === 'plain' || !cs.type) return cs.value;
            if (cs.type === 'secret') return `[From Secret: ${cs.ref || cs.ref_name}]`;
            if (cs.type === 'configmap') return `[From ConfigMap: ${cs.ref || cs.ref_name}]`;
            return cs.value || '[Not configured]';
        };
        
        const webhookURL = config.webhook ? displayConfigSource(config.webhook.url) : null;
        const slackURL = config.slack ? displayConfigSource(config.slack.webhook_url) : null;
        
        if (webhookURL) {
            providersHTML += `<li><strong>Webhook:</strong> ${escapeHtml(webhookURL)}</li>`;
        }
        if (slackURL) {
            providersHTML += `<li><strong>Slack:</strong> ${escapeHtml(slackURL)}</li>`;
        }
        
        if (!webhookURL && !slackURL) {
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
    document.getElementById('approval-pattern-form-title').textContent = 'Add Approval Pattern';
    document.getElementById('approvalPatternForm').reset();
    document.getElementById('approval-pattern-index').value = '';
    document.getElementById('approval-timeout').value = '300';
    document.getElementById('approval-role-match').value = 'any';
    document.getElementById('approval-tag-match').value = 'all';
    
    // Show modal
    document.getElementById('approval-pattern-form-modal').style.display = 'flex';
    
    // Prevent body scroll
    document.body.style.overflow = 'hidden';
}

function hideApprovalPatternForm() {
    document.getElementById('approval-pattern-form-modal').style.display = 'none';
    document.getElementById('approvalPatternForm').reset();
    
    // Re-enable body scroll
    document.body.style.overflow = '';
}

async function editApprovalPattern(index) {
    try {
        const config = await apiCall('/approvals');
        const pattern = config.patterns.find(p => p.index === index);
        
        if (!pattern) {
            showNotification('Pattern not found', 'error');
            return;
        }
        
        document.getElementById('approval-pattern-form-title').textContent = 'Edit Approval Pattern';
        document.getElementById('approval-pattern-index').value = index;
        document.getElementById('approval-pattern').value = pattern.pattern;
        document.getElementById('approval-tags').value = pattern.tags ? pattern.tags.join(', ') : '';
        document.getElementById('approval-tag-match').value = pattern.tag_match || 'all';
        document.getElementById('approval-roles').value = pattern.roles ? pattern.roles.join(', ') : '';
        document.getElementById('approval-role-match').value = pattern.role_match || 'any';
        document.getElementById('approval-timeout').value = pattern.timeout_seconds;
        
        // Show modal
        document.getElementById('approval-pattern-form-modal').style.display = 'flex';
        
        // Prevent body scroll
        document.body.style.overflow = 'hidden';
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
    
    const roles = document.getElementById('approval-roles').value
        .split(',')
        .map(r => r.trim())
        .filter(r => r !== '');
    
    const pattern = {
        pattern: document.getElementById('approval-pattern').value,
        tags: tags.length > 0 ? tags : null,
        tag_match: document.getElementById('approval-tag-match').value,
        roles: roles.length > 0 ? roles : null,
        role_match: document.getElementById('approval-role-match').value,
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

async function toggleProviderConfigSourceFields(provider) {
    const type = document.getElementById(`${provider}-url-type`).value;
    const plainFields = document.getElementById(`${provider}-plain-fields`);
    const k8sFields = document.getElementById(`${provider}-k8s-fields`);
    
    if (type === 'plain') {
        plainFields.style.display = 'block';
        k8sFields.style.display = 'none';
    } else {
        plainFields.style.display = 'none';
        k8sFields.style.display = 'block';
        
        // Fetch and populate K8s resources for autocomplete
        const resources = await fetchK8sResources();
        const inputId = `${provider}-url-ref`;
        const options = type === 'configmap' ? resources.configmaps : resources.secrets;
        setupAutocomplete(inputId, options);
    }
}

function showProvidersForm() {
    const config = window.currentApprovalConfig || {};
    
    // Reset form
    document.getElementById('providersForm').reset();
    
    // Populate Webhook URL fields
    if (config.webhook && config.webhook.url) {
        const webhookCS = config.webhook.url;
        const webhookType = webhookCS.type || 'plain';
        document.getElementById('webhook-url-type').value = webhookType;
        
        if (webhookType === 'plain') {
            document.getElementById('provider-webhook-url').value = webhookCS.value || '';
        } else {
            document.getElementById('webhook-url-ref').value = webhookCS.ref || '';
            document.getElementById('webhook-url-ref-name').value = webhookCS.ref_name || 'url';
        }
        toggleProviderConfigSourceFields('webhook');
    } else {
        document.getElementById('webhook-url-type').value = 'plain';
        document.getElementById('provider-webhook-url').value = '';
        toggleProviderConfigSourceFields('webhook');
    }
    
    // Populate Slack Webhook URL fields
    if (config.slack && config.slack.webhook_url) {
        const slackCS = config.slack.webhook_url;
        const slackType = slackCS.type || 'plain';
        document.getElementById('slack-url-type').value = slackType;
        
        if (slackType === 'plain') {
            document.getElementById('provider-slack-url').value = slackCS.value || '';
        } else {
            document.getElementById('slack-url-ref').value = slackCS.ref || '';
            document.getElementById('slack-url-ref-name').value = slackCS.ref_name || 'webhook_url';
        }
        toggleProviderConfigSourceFields('slack');
    } else {
        document.getElementById('slack-url-type').value = 'plain';
        document.getElementById('provider-slack-url').value = '';
        toggleProviderConfigSourceFields('slack');
    }
    
    // Show modal
    document.getElementById('providers-form-modal').style.display = 'flex';
    
    // Prevent body scroll
    document.body.style.overflow = 'hidden';
}

function hideProvidersForm() {
    document.getElementById('providers-form-modal').style.display = 'none';
    document.getElementById('providersForm').reset();
    
    // Re-enable body scroll
    document.body.style.overflow = '';
}

async function saveProviders(event) {
    event.preventDefault();
    
    const providers = {};
    
    // Build Webhook ConfigSource
    const webhookType = document.getElementById('webhook-url-type').value;
    if (webhookType === 'plain') {
        const webhookUrl = document.getElementById('provider-webhook-url').value.trim();
        providers.webhook = {
            url: {
                type: 'plain',
                value: webhookUrl
            }
        };
    } else {
        const webhookRef = document.getElementById('webhook-url-ref').value.trim();
        const webhookRefName = document.getElementById('webhook-url-ref-name').value.trim();
        providers.webhook = {
            url: {
                type: webhookType,
                ref: webhookRef,
                ref_name: webhookRefName || 'url',
                value: '' // Empty for ConfigMap/Secret types
            }
        };
    }
    
    // Build Slack ConfigSource
    const slackType = document.getElementById('slack-url-type').value;
    if (slackType === 'plain') {
        const slackUrl = document.getElementById('provider-slack-url').value.trim();
        providers.slack = {
            webhook_url: {
                type: 'plain',
                value: slackUrl
            }
        };
    } else {
        const slackRef = document.getElementById('slack-url-ref').value.trim();
        const slackRefName = document.getElementById('slack-url-ref-name').value.trim();
        providers.slack = {
            webhook_url: {
                type: slackType,
                ref: slackRef,
                ref_name: slackRefName || 'webhook_url',
                value: '' // Empty for ConfigMap/Secret types
            }
        };
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

