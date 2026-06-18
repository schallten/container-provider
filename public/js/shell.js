// Shared sidebar + topbar injection
(function() {
  const sidebarHTML = `
  <nav class="sidebar" id="sidebar">
    <div class="nav-section">
      <div class="nav-section-label">General</div>
      <a href="/pages/dashboard.html" class="nav-item">
        <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>
        Dashboard
      </a>
    </div>

    <div class="nav-section">
      <div class="nav-section-label">Services</div>

      <div class="nav-group-toggle" data-group="compute">
        <div style="display:flex;align-items:center;gap:8px;">
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
          Compute
        </div>
        <span class="arrow">&#9656;</span>
      </div>
      <div class="nav-group-children" id="nav-compute">
        <a href="/pages/compute.html" class="nav-item-child">
          <span class="dot"></span>
          Overview
        </a>
        <a href="/pages/tempdev.html" class="nav-item-child">
          <span class="dot"></span>
          TempDev
        </a>
      </div>

      <div class="nav-group-toggle" data-group="networking">
        <div style="display:flex;align-items:center;gap:8px;">
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
          Networking
        </div>
        <span class="arrow">&#9656;</span>
      </div>
      <div class="nav-group-children" id="nav-networking">
        <a class="nav-item-child" style="opacity:0.4;pointer-events:none;">
          <span class="dot"></span>
          Tunnels
        </a>
        <a class="nav-item-child" style="opacity:0.4;pointer-events:none;">
          <span class="dot"></span>
          Load Balancers
        </a>
      </div>

      <div class="nav-group-toggle" data-group="storage">
        <div style="display:flex;align-items:center;gap:8px;">
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/></svg>
          Storage
        </div>
        <span class="arrow">&#9656;</span>
      </div>
      <div class="nav-group-children" id="nav-storage">
        <a class="nav-item-child" style="opacity:0.4;pointer-events:none;">
          <span class="dot"></span>
          Object Store
        </a>
      </div>
    </div>

    <div class="nav-section">
      <div class="nav-section-label">Management</div>
      <a href="/pages/logs.html" class="nav-item">
        <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/></svg>
        Logs
      </a>
      <a href="/pages/costs.html" class="nav-item">
        <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/></svg>
        Cost Explorer
      </a>
      <a class="nav-item" onclick="document.getElementById('settings-overlay').classList.add('open')">
        <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
        Settings
      </a>
      <a href="/pages/billing.html" class="nav-item">
        <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="1" y="4" width="22" height="16" rx="2"/><line x1="1" y1="10" x2="23" y2="10"/></svg>
        Billing
      </a>
    </div>
  `;

  const topbarHTML = `
  <nav class="topbar">
    <a href="/pages/dashboard.html" class="topbar-logo">
      <div class="topbar-logo-text"><span>temp</span>dev</div>
    </a>
    <div class="topbar-center">
      <div class="topbar-search">
        <svg class="icon" viewBox="0 0 16 16" fill="currentColor"><path d="M11.742 10.344a6.5 6.5 0 1 0-1.397 1.398l3.85 3.85a1 1 0 0 0 1.415-1.414l-3.868-3.834zm-5.242.156a5 5 0 1 1 0-10 5 5 0 0 1 0 10z"/></svg>
        <input type="text" placeholder="Search" />
      </div>
    </div>
    <div class="topbar-right">
      <span class="region-badge" id="region-badge">loading...</span>
      <a href="/pages/dashboard.html">Dashboard</a>
    </div>
  </nav>
  `;

  // Inject topbar
  const topbarTarget = document.getElementById('topbar-root');
  if (topbarTarget) topbarTarget.innerHTML = topbarHTML;

  // Inject sidebar
  const sidebarTarget = document.getElementById('sidebar-root');
  if (sidebarTarget) sidebarTarget.innerHTML = sidebarHTML;

  // Inject settings overlay (if not already present)
  if (!document.getElementById('settings-overlay')) {
    const overlay = document.createElement('div');
    overlay.className = 'settings-overlay';
    overlay.id = 'settings-overlay';
    overlay.onclick = (e) => { if (e.target === overlay) overlay.classList.remove('open'); };
    overlay.innerHTML = `
      <div class="settings-panel">
        <div class="settings-header">
          <h3>Settings</h3>
          <button class="settings-close" onclick="document.getElementById('settings-overlay').classList.remove('open')">&times;</button>
        </div>
        <div class="settings-body">
          <div class="setting-row">
            <div>
              <div class="setting-label">Disable idle timeout</div>
              <div class="setting-desc">Keep environment alive even when idle (default: 15m)</div>
            </div>
            <label class="toggle">
              <input type="checkbox" id="idle-toggle" onchange="toggleSetting('no-idle-timeout', this.checked)">
              <span class="toggle-slider"></span>
            </label>
          </div>
          <div class="setting-row">
            <div>
              <div class="setting-label">Bypass max lifetime</div>
              <div class="setting-desc">Run indefinitely (default: 12h limit)</div>
            </div>
            <label class="toggle">
              <input type="checkbox" id="max-lifetime-toggle" onchange="toggleSetting('no-max-lifetime', this.checked)">
              <span class="toggle-slider"></span>
            </label>
          </div>
          <div class="setting-row">
            <div>
              <div class="setting-label">Location</div>
              <div class="setting-desc" id="settings-location">Detecting...</div>
            </div>
          </div>
        </div>
      </div>`;
    document.body.appendChild(overlay);
  }

  // Settings toggle function (overridden by tempdev page)
  window.toggleSetting = async function(action, enabled) {};
  window._currentEnvId = null;

  // Load settings when env is active
  window.loadSettings = async function(envId) {
    window._currentEnvId = envId;
    try {
      const r = await fetch(`/settings/${envId}/get`);
      const d = await r.json();
      const idleEl = document.getElementById('idle-toggle');
      const maxEl = document.getElementById('max-lifetime-toggle');
      if (idleEl) idleEl.checked = d.no_idle_timeout || false;
      if (maxEl) maxEl.checked = d.no_max_lifetime || false;
    } catch (e) {}
  };
})();
