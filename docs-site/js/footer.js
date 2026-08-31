// Shared footer for Automergent docs site
(function() {
  const currentPath = window.location.pathname;
  const isIndex = currentPath === '/' || currentPath.endsWith('/index.html') || currentPath.endsWith('/docs-site/') || currentPath.endsWith('/docs-site/index.html');
  const logoSrc = isIndex ? 'assets/logo.svg' : '../assets/logo.svg';
  const basePath = isIndex ? '' : '../';

  const footerHTML = `
  <footer class="footer docs-footer">
    <div class="footer-inner docs-footer-inner">
      <div class="footer-brand docs-footer-brand">
        <a href="${isIndex ? 'index.html' : '../index.html'}" class="navbar-logo">
          <img src="${logoSrc}" alt="Automergent" height="26" style="height:26px; width:auto;">
          <span>Auto<span class="logo-accent">mergent</span></span>
        </a>
        <p>Next-gen autonomous coding engineer built on Gemini & Vertex AI with multi-phase context intelligence, subagent orchestration, and deep root-cause error diagnostics.</p>
      </div>

      <div class="footer-col">
        <h4>Documentation</h4>
        <ul>
          <li><a href="${isIndex ? 'pages/getting-started.html' : 'getting-started.html'}">Getting Started</a></li>
          <li><a href="${isIndex ? 'pages/commands.html' : 'commands.html'}">Commands Reference</a></li>
          <li><a href="${isIndex ? 'pages/tools.html' : 'tools.html'}">Tools Reference</a></li>
          <li><a href="${isIndex ? 'pages/configuration.html' : 'configuration.html'}">Configuration</a></li>
        </ul>
      </div>

      <div class="footer-col">
        <h4>Architecture & UI</h4>
        <ul>
          <li><a href="${isIndex ? 'pages/architecture.html' : 'architecture.html'}">Architecture</a></li>
          <li><a href="${isIndex ? 'pages/themes-ui.html' : 'themes-ui.html'}">Themes & UI</a></li>
          <li><a href="${isIndex ? 'pages/advanced-features.html' : 'advanced-features.html'}">Advanced Features</a></li>
          <li><a href="${isIndex ? 'pages/examples.html' : 'examples.html'}">Examples & Tutorials</a></li>
        </ul>
      </div>

      <div class="footer-col">
        <h4>Community</h4>
        <ul>
          <li><a href="https://github.com/iSundram/Automergent" target="_blank" rel="noopener">GitHub Repository</a></li>
          <li><a href="https://github.com/iSundram/Automergent/releases" target="_blank" rel="noopener">Releases & Changelog</a></li>
          <li><a href="https://github.com/iSundram/Automergent/issues" target="_blank" rel="noopener">Report an Issue</a></li>
          <li><a href="${isIndex ? 'pages/developer-guide.html' : 'developer-guide.html'}">Developer Guide</a></li>
        </ul>
      </div>
    </div>

    <div class="footer-bottom">
      <span>&copy; 2026 Automergent. Released under the <a href="https://opensource.org/licenses/MIT" target="_blank" rel="noopener">MIT License</a>.</span>
      <span>Built with pride by <a href="https://github.com/iSundram" target="_blank" rel="noopener">iSundram</a> and contributors.</span>
    </div>
  </footer>

  <button class="back-to-top" aria-label="Back to top" title="Back to top">
    <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
      <polyline points="18 15 12 9 6 15"></polyline>
    </svg>
  </button>
  `;

  const existingFooter = document.querySelector('footer.footer, footer.docs-footer');
  if (!existingFooter) {
    document.body.insertAdjacentHTML('beforeend', footerHTML);
  }
})();
