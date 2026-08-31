// Shared footer for Automergent docs site
(function() {
  const footerHTML = `
  <footer class="docs-footer">
    <div class="docs-footer-inner">
      <div class="docs-footer-brand">
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
          <polyline points="16 18 22 12 16 6"></polyline>
          <polyline points="8 6 2 12 8 18"></polyline>
        </svg>
        <span>Automergent</span>
      </div>
      <div class="docs-footer-links">
        <a href="https://github.com/iSundram/Automergent">GitHub</a>
        <a href="https://github.com/iSundram/Automergent/releases">Releases</a>
        <a href="https://github.com/iSundram/Automergent/issues">Issues</a>
        <a href="https://github.com/iSundram/Automergent/blob/main/LICENSE">MIT License</a>
      </div>
      <div class="docs-footer-copy">Built by iSundram & contributors</div>
    </div>
  </footer>
  <style>
    .docs-footer {
      background: #161b22; border-top: 1px solid #30363d;
      padding: 32px 24px; margin-top: 64px;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif;
    }
    .docs-footer-inner {
      max-width: 1400px; margin: 0 auto;
      display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 16px;
    }
    .docs-footer-brand {
      display: flex; align-items: center; gap: 8px;
      color: #e6edf3; font-weight: 600; font-size: 14px;
    }
    .docs-footer-brand svg { color: #58a6ff; }
    .docs-footer-links { display: flex; gap: 20px; }
    .docs-footer-links a {
      color: #8b949e; text-decoration: none; font-size: 13px;
      transition: color 0.15s;
    }
    .docs-footer-links a:hover { color: #58a6ff; }
    .docs-footer-copy { color: #6e7681; font-size: 12px; }
    @media (max-width: 640px) {
      .docs-footer-inner { flex-direction: column; text-align: center; }
    }
  </style>`;

  document.body.insertAdjacentHTML('beforeend', footerHTML);
})();
