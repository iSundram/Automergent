// Shared navigation for Automergent docs site
(function() {
  const pages = [
    { href: 'index.html', label: 'Home' },
    { href: 'pages/getting-started.html', label: 'Getting Started' },
    { href: 'pages/commands.html', label: 'Commands' },
    { href: 'pages/tools.html', label: 'Tools' },
    { href: 'pages/configuration.html', label: 'Configuration' },
    { href: 'pages/architecture.html', label: 'Architecture' },
    { href: 'pages/themes-ui.html', label: 'Themes & UI' },
    { href: 'pages/advanced-features.html', label: 'Advanced' },
    { href: 'pages/examples.html', label: 'Examples' },
    { href: 'pages/developer-guide.html', label: 'Developer' },
    { href: 'pages/troubleshooting.html', label: 'Troubleshooting' },
  ];

  const currentPath = window.location.pathname;
  const isIndex = currentPath === '/' || currentPath.endsWith('/index.html') || currentPath.endsWith('/docs-site/') || currentPath.endsWith('/docs-site/index.html');
  const basePath = isIndex ? '' : '../';

  function resolveHref(page) {
    if (isIndex) {
      return page.href;
    }
    if (page.href === 'index.html') {
      return '../index.html';
    }
    return page.href.replace('pages/', '');
  }

  function isActive(page) {
    if (isIndex && page.href === 'index.html') return true;
    const pageFilename = page.href.split('/').pop();
    return currentPath.endsWith(pageFilename);
  }

  const logoSrc = isIndex ? 'assets/logo.svg' : '../assets/logo.svg';
  const homeHref = isIndex ? 'index.html' : '../index.html';

  const navHTML = `
  <nav class="navbar docs-nav" role="navigation" aria-label="Main navigation">
    <div class="navbar-inner docs-nav-inner">
      <a href="${homeHref}" class="navbar-logo docs-nav-logo" aria-label="Automergent Home">
        <img src="${logoSrc}" alt="Automergent" height="28" style="height:28px; width:auto;">
        <span>Auto<span class="logo-accent">mergent</span></span>
      </a>

      <div class="navbar-links docs-nav-links">
        ${pages.map(p => `<a href="${resolveHref(p)}" class="${isActive(p) ? 'active' : ''}">${p.label}</a>`).join('')}
      </div>

      <div class="navbar-actions">
        <button class="theme-toggle" aria-label="Toggle theme" title="Toggle theme">
          <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
          </svg>
        </button>
        <a href="https://github.com/iSundram/Automergent" target="_blank" rel="noopener" class="docs-nav-github" aria-label="GitHub Repository" title="GitHub Repository">
          <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
          </svg>
        </a>
        <button class="hamburger" aria-label="Toggle menu" aria-expanded="false">
          <span></span>
          <span></span>
          <span></span>
        </button>
      </div>
    </div>
  </nav>

  <div class="mobile-nav" role="navigation" aria-label="Mobile navigation">
    <ul class="mobile-nav-links">
      ${pages.map(p => `<li><a href="${resolveHref(p)}" class="${isActive(p) ? 'active' : ''}">${p.label}</a></li>`).join('')}
      <li><a href="https://github.com/iSundram/Automergent" target="_blank" rel="noopener">GitHub ↗</a></li>
    </ul>
  </div>
  `;

  // Insert before first element in body or at top
  const existingNav = document.querySelector('nav.navbar, nav.docs-nav');
  if (!existingNav) {
    document.body.insertAdjacentHTML('afterbegin', navHTML);
  }
})();
