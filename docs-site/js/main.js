(() => {
  'use strict';

  document.addEventListener('DOMContentLoaded', init);

  function init() {
    initMobileMenu();
    initThemeToggle();
    initSmoothScroll();
    initCopyButtons();
    initTabs();
    initSidebarCollapse();
    initActiveNavHighlight();
    initBackToTop();
    initSearch();
    initKeyboardShortcuts();
  }

  // ========== MOBILE MENU ==========
  function initMobileMenu() {
    document.addEventListener('click', (e) => {
      const hamburger = e.target.closest('.hamburger');
      const mobileNav = document.querySelector('.mobile-nav');
      if (hamburger && mobileNav) {
        hamburger.classList.toggle('active');
        mobileNav.classList.toggle('active');
        document.body.style.overflow = mobileNav.classList.contains('active') ? 'hidden' : '';
      } else if (!e.target.closest('.mobile-nav') && !e.target.closest('.hamburger')) {
        const h = document.querySelector('.hamburger');
        const m = document.querySelector('.mobile-nav');
        if (h && m && m.classList.contains('active')) {
          h.classList.remove('active');
          m.classList.remove('active');
          document.body.style.overflow = '';
        }
      }
    });
  }

  // ========== THEME TOGGLE ==========
  function initThemeToggle() {
    const stored = localStorage.getItem('theme') || 'dark';
    document.documentElement.setAttribute('data-theme', stored);
    updateThemeIcon(stored);

    document.addEventListener('click', (e) => {
      const toggle = e.target.closest('.theme-toggle');
      if (!toggle) return;
      const current = document.documentElement.getAttribute('data-theme') || 'dark';
      const next = current === 'light' ? 'dark' : 'light';
      document.documentElement.setAttribute('data-theme', next);
      localStorage.setItem('theme', next);
      updateThemeIcon(next);
    });
  }

  function updateThemeIcon(theme) {
    document.querySelectorAll('.theme-toggle').forEach(toggle => {
      const sunIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>`;
      const moonIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>`;
      toggle.innerHTML = theme === 'light' ? moonIcon : sunIcon;
      toggle.setAttribute('aria-label', `Switch to ${theme === 'light' ? 'dark' : 'light'} theme`);
    });
  }

  // ========== SMOOTH SCROLL ==========
  function initSmoothScroll() {
    document.addEventListener('click', (e) => {
      const anchor = e.target.closest('a[href^="#"]');
      if (!anchor) return;
      const href = anchor.getAttribute('href');
      if (href === '#' || href === '') return;
      const target = document.querySelector(href);
      if (target) {
        e.preventDefault();
        target.scrollIntoView({ behavior: 'smooth', block: 'start' });
        history.pushState(null, '', href);
      }
    });
  }

  // ========== TABS ==========
  function initTabs() {
    document.addEventListener('click', (e) => {
      const btn = e.target.closest('.tab-btn');
      if (!btn) return;
      const tabContainer = btn.closest('.tabs');
      if (!tabContainer) return;
      const tabId = btn.getAttribute('data-tab');
      if (!tabId) return;

      tabContainer.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
      tabContainer.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

      btn.classList.add('active');
      const targetContent = document.getElementById(tabId) || tabContainer.querySelector(`[data-tab-content="${tabId}"]`);
      if (targetContent) {
        targetContent.classList.add('active');
      }
    });
  }

  // ========== COPY CODE BUTTONS ==========
  function initCopyButtons() {
    document.addEventListener('click', (e) => {
      const btn = e.target.closest('.copy-btn');
      if (!btn) return;
      const block = btn.closest('.code-block') || btn.closest('.hero-code') || btn.closest('.code-content') || btn.parentElement;
      if (!block) return;
      const code = block.querySelector('code') || block.querySelector('pre');
      if (!code) return;

      const text = code.innerText || code.textContent;
      navigator.clipboard.writeText(text.trim()).then(() => {
        const originalHTML = btn.innerHTML;
        btn.innerHTML = `✓ Copied!`;
        btn.classList.add('copied');
        setTimeout(() => {
          btn.innerHTML = originalHTML;
          btn.classList.remove('copied');
        }, 2000);
      }).catch(() => {
        fallbackCopy(text, btn);
      });
    });
  }

  function fallbackCopy(text, btn) {
    const textarea = document.createElement('textarea');
    textarea.value = text.trim();
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    try {
      document.execCommand('copy');
      const originalHTML = btn.innerHTML;
      btn.innerHTML = `✓ Copied!`;
      btn.classList.add('copied');
      setTimeout(() => {
        btn.innerHTML = originalHTML;
        btn.classList.remove('copied');
      }, 2000);
    } catch (err) {
      console.error('Copy failed:', err);
    }
    document.body.removeChild(textarea);
  }

  // ========== SIDEBAR COLLAPSE ==========
  function initSidebarCollapse() {
    document.addEventListener('click', (e) => {
      const title = e.target.closest('.sidebar-section-title');
      if (!title) return;
      const section = title.closest('.sidebar-section');
      if (section) {
        section.classList.toggle('collapsed');
      }
    });
  }

  // ========== ACTIVE NAV HIGHLIGHTING ==========
  function initActiveNavHighlight() {
    const headings = document.querySelectorAll('h2[id], h3[id], section[id]');
    const links = document.querySelectorAll('.sidebar nav a[href^="#"], .sidebar ul a[href^="#"], .sidebar-links a[href^="#"]');

    if (headings.length === 0 || links.length === 0) return;

    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach(entry => {
          if (entry.isIntersecting) {
            const id = entry.target.getAttribute('id');
            links.forEach(link => {
              const active = link.getAttribute('href') === `#${id}`;
              link.classList.toggle('active', active);
            });
          }
        });
      },
      { rootMargin: '-15% 0px -75% 0px', threshold: 0 }
    );

    headings.forEach(h => observer.observe(h));
  }

  // ========== BACK TO TOP ==========
  function initBackToTop() {
    window.addEventListener('scroll', () => {
      const btn = document.querySelector('.back-to-top');
      if (btn) {
        btn.classList.toggle('visible', window.scrollY > 400);
      }
    }, { passive: true });

    document.addEventListener('click', (e) => {
      const btn = e.target.closest('.back-to-top');
      if (btn) {
        window.scrollTo({ top: 0, behavior: 'smooth' });
      }
    });
  }

  // ========== SEARCH ==========
  function initSearch() {
    document.addEventListener('input', (e) => {
      const input = e.target.closest('.navbar-search input');
      if (!input) return;
      const resultsContainer = input.parentElement.querySelector('.search-results');
      if (!resultsContainer) return;

      const query = input.value.trim().toLowerCase();
      if (query.length < 2) {
        resultsContainer.classList.remove('active');
        return;
      }

      const searchData = buildSearchData();
      const matches = searchData.filter(item =>
        item.title.toLowerCase().includes(query) ||
        item.description.toLowerCase().includes(query) ||
        item.keywords.some(k => k.toLowerCase().includes(query))
      ).slice(0, 8);

      renderSearchResults(resultsContainer, matches, query);
    });

    document.addEventListener('click', (e) => {
      if (!e.target.closest('.navbar-search')) {
        document.querySelectorAll('.search-results').forEach(r => r.classList.remove('active'));
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
        url: el.getAttribute('data-search-url') || '#' + (el.id || '')
      });
    });

    if (items.length === 0) {
      document.querySelectorAll('h2[id], h3[id]').forEach(h => {
        items.push({
          title: h.textContent.trim(),
          description: (h.nextElementSibling ? h.nextElementSibling.textContent : '').trim().slice(0, 100),
          keywords: h.textContent.trim().toLowerCase().split(/\s+/),
          url: '#' + h.id
        });
      });
    }

    return items;
  }

  function renderSearchResults(container, matches, query) {
    if (matches.length === 0) {
      container.innerHTML = `<div class="search-no-results">No results found for "${query}"</div>`;
      container.classList.add('active');
      return;
    }

    container.innerHTML = matches.map(item => `
      <a href="${item.url}" class="search-result-item">
        <div class="result-title">${escapeHtml(item.title)}</div>
        <div class="result-desc">${escapeHtml(item.description)}</div>
      </a>
    `).join('');
    container.classList.add('active');
  }

  function escapeHtml(str) {
    return str.replace(/[&<>"']/g, m => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    })[m]);
  }

  // ========== KEYBOARD SHORTCUTS ==========
  function initKeyboardShortcuts() {
    document.addEventListener('keydown', (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault();
        const searchInput = document.querySelector('.navbar-search input');
        if (searchInput) {
          searchInput.focus();
        }
      }
      if (e.key === 'Escape') {
        document.querySelectorAll('.search-results').forEach(r => r.classList.remove('active'));
      }
    });
  }

  // Expose copyCode helper globally for inline onclick handlers
  window.copyCode = function(btn) {
    const block = btn.closest('.code-block') || btn.closest('.hero-code') || btn.closest('.code-content') || btn.parentElement;
    if (!block) return;
    const code = block.querySelector('code') || block.querySelector('pre');
    if (!code) return;
    navigator.clipboard.writeText(code.innerText.trim()).then(() => {
      const orig = btn.innerText;
      btn.innerText = 'Copied!';
      btn.classList.add('copied');
      setTimeout(() => {
        btn.innerText = orig;
        btn.classList.remove('copied');
      }, 2000);
    });
  };
})();
