(() => {
  'use strict';

  // ========== DOM READY ==========
  document.addEventListener('DOMContentLoaded', init);

  function init() {
    initMobileMenu();
    initThemeToggle();
    initSmoothScroll();
    initCopyButtons();
    initSidebarCollapse();
    initActiveNavHighlight();
    initBackToTop();
    initSearch();
    initKeyboardShortcuts();
  }

  // ========== MOBILE MENU ==========
  function initMobileMenu() {
    const hamburger = document.querySelector('.hamburger');
    const mobileNav = document.querySelector('.mobile-nav');
    if (!hamburger || !mobileNav) return;

    hamburger.addEventListener('click', () => {
      hamburger.classList.toggle('active');
      mobileNav.classList.toggle('active');
      document.body.style.overflow = mobileNav.classList.contains('active') ? 'hidden' : '';
    });

    mobileNav.querySelectorAll('a').forEach(link => {
      link.addEventListener('click', () => {
        hamburger.classList.remove('active');
        mobileNav.classList.remove('active');
        document.body.style.overflow = '';
      });
    });
  }

  // ========== THEME TOGGLE ==========
  function initThemeToggle() {
    const toggle = document.querySelector('.theme-toggle');
    if (!toggle) return;

    const stored = localStorage.getItem('theme');
    if (stored) {
      document.documentElement.setAttribute('data-theme', stored);
    }

    toggle.addEventListener('click', () => {
      const current = document.documentElement.getAttribute('data-theme');
      const next = current === 'light' ? 'dark' : 'light';
      document.documentElement.setAttribute('data-theme', next);
      localStorage.setItem('theme', next);
      updateThemeIcon(next);
    });

    updateThemeIcon(document.documentElement.getAttribute('data-theme') || 'dark');
  }

  function updateThemeIcon(theme) {
    const toggle = document.querySelector('.theme-toggle');
    if (!toggle) return;
    const sunIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>`;
    const moonIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>`;
    toggle.innerHTML = theme === 'light' ? moonIcon : sunIcon;
    toggle.setAttribute('aria-label', `Switch to ${theme === 'light' ? 'dark' : 'light'} theme`);
  }

  // ========== SMOOTH SCROLL ==========
  function initSmoothScroll() {
    document.querySelectorAll('a[href^="#"]').forEach(anchor => {
      anchor.addEventListener('click', (e) => {
        const href = anchor.getAttribute('href');
        if (href === '#') return;
        const target = document.querySelector(href);
        if (target) {
          e.preventDefault();
          target.scrollIntoView({ behavior: 'smooth', block: 'start' });
          history.pushState(null, '', href);
        }
      });
    });
  }

  // ========== COPY CODE BUTTONS ==========
  function initCopyButtons() {
    document.querySelectorAll('.copy-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const block = btn.closest('.code-block') || btn.closest('.hero-code');
        if (!block) return;
        const code = block.querySelector('code');
        if (!code) return;

        const text = code.textContent;
        navigator.clipboard.writeText(text).then(() => {
          const original = btn.textContent;
          btn.textContent = 'Copied!';
          btn.classList.add('copied');
          setTimeout(() => {
            btn.textContent = original || 'Copy';
            btn.classList.remove('copied');
          }, 2000);
        }).catch(() => {
          fallbackCopy(text, btn);
        });
      });
    });
  }

  function fallbackCopy(text, btn) {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    try {
      document.execCommand('copy');
      const original = btn.textContent;
      btn.textContent = 'Copied!';
      btn.classList.add('copied');
      setTimeout(() => {
        btn.textContent = original || 'Copy';
        btn.classList.remove('copied');
      }, 2000);
    } catch (err) {
      console.error('Copy failed:', err);
    }
    document.body.removeChild(textarea);
  }

  // ========== SIDEBAR COLLAPSE ==========
  function initSidebarCollapse() {
    document.querySelectorAll('.sidebar-section-title').forEach(title => {
      title.addEventListener('click', () => {
        const section = title.closest('.sidebar-section');
        if (section) {
          section.classList.toggle('collapsed');
        }
      });
    });
  }

  // ========== ACTIVE NAV HIGHLIGHTING ==========
  function initActiveNavHighlight() {
    const sections = document.querySelectorAll('section[id]');
    const navLinks = document.querySelectorAll('.navbar-links a[href^="#"]');

    if (sections.length === 0 || navLinks.length === 0) return;

    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach(entry => {
          if (entry.isIntersecting) {
            const id = entry.target.getAttribute('id');
            navLinks.forEach(link => {
              link.classList.toggle('active', link.getAttribute('href') === `#${id}`);
            });
          }
        });
      },
      { rootMargin: '-20% 0px -70% 0px', threshold: 0 }
    );

    sections.forEach(section => observer.observe(section));
  }

  // ========== BACK TO TOP ==========
  function initBackToTop() {
    const btn = document.querySelector('.back-to-top');
    if (!btn) return;

    window.addEventListener('scroll', () => {
      btn.classList.toggle('visible', window.scrollY > 400);
    }, { passive: true });

    btn.addEventListener('click', () => {
      window.scrollTo({ top: 0, behavior: 'smooth' });
    });
  }

  // ========== SEARCH ==========
  function initSearch() {
    const input = document.querySelector('.navbar-search input');
    const resultsContainer = document.querySelector('.search-results');
    if (!input || !resultsContainer) return;

    const searchData = buildSearchData();

    input.addEventListener('input', () => {
      const query = input.value.trim().toLowerCase();
      if (query.length < 2) {
        resultsContainer.classList.remove('active');
        return;
      }

      const matches = searchData.filter(item =>
        item.title.toLowerCase().includes(query) ||
        item.description.toLowerCase().includes(query) ||
        item.keywords.some(k => k.includes(query))
      ).slice(0, 8);

      renderSearchResults(resultsContainer, matches, query);
    });

    input.addEventListener('focus', () => {
      if (input.value.trim().length >= 2) {
        resultsContainer.classList.add('active');
      }
    });

    document.addEventListener('click', (e) => {
      if (!e.target.closest('.navbar-search')) {
        resultsContainer.classList.remove('active');
      }
    });
  }

  function buildSearchData() {
    const items = [];
    document.querySelectorAll('[data-search-title]').forEach(el => {
      items.push({
        title: el.getAttribute('data-search-title'),
        description: el.getAttribute('data-search-desc') || '',
        keywords: (el.getAttribute('data-search-keywords') || '').split(',').map(s => s.trim()),
        url: el.getAttribute('data-search-url') || '#'
      });
    });

    if (items.length === 0) {
      document.querySelectorAll('.feature-card h3, .docs-content h2, .docs-content h3').forEach(heading => {
        items.push({
          title: heading.textContent.trim(),
          description: (heading.nextElementSibling ? heading.nextElementSibling.textContent : '').trim().slice(0, 120),
          keywords: heading.textContent.trim().toLowerCase().split(/\s+/),
          url: '#' + (heading.closest('section')?.id || heading.closest('[id]')?.id || '')
        });
      });
    }

    return items;
  }

  function renderSearchResults(container, matches, query) {
    if (matches.length === 0) {
      container.innerHTML = `<div class="search-no-results">No results for "${escapeHtml(query)}"</div>`;
      container.classList.add('active');
      return;
    }

    container.innerHTML = matches.map(item => `
      <a href="${escapeHtml(item.url)}" class="search-result-item">
        <div class="result-title">${highlightMatch(item.title, query)}</div>
        <div class="result-desc">${escapeHtml(item.description)}</div>
      </a>
    `).join('');

    container.classList.add('active');

    container.querySelectorAll('.search-result-item').forEach(link => {
      link.addEventListener('click', () => {
        container.classList.remove('active');
      });
    });
  }

  function highlightMatch(text, query) {
    const escaped = escapeHtml(text);
    const regex = new RegExp(`(${escapeRegExp(query)})`, 'gi');
    return escaped.replace(regex, '<strong style="color:var(--accent-blue)">$1</strong>');
  }

  function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  function escapeRegExp(str) {
    return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  }

  // ========== KEYBOARD SHORTCUTS ==========
  function initKeyboardShortcuts() {
    document.addEventListener('keydown', (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        const input = document.querySelector('.navbar-search input');
        if (input) {
          input.focus();
          input.select();
        }
      }

      if (e.key === 'Escape') {
        const results = document.querySelector('.search-results');
        if (results) results.classList.remove('active');

        const mobileNav = document.querySelector('.mobile-nav');
        const hamburger = document.querySelector('.hamburger');
        if (mobileNav && mobileNav.classList.contains('active')) {
          mobileNav.classList.remove('active');
          hamburger?.classList.remove('active');
          document.body.style.overflow = '';
        }
      }
    });
  }
})();
