// Sidebar navigation state
(function() {
  // Toggle nav groups
  document.querySelectorAll('.nav-group-toggle').forEach(toggle => {
    toggle.addEventListener('click', () => {
      const children = toggle.nextElementSibling;
      const isOpen = children.classList.contains('open');

      // Close all other groups
      document.querySelectorAll('.nav-group-children').forEach(c => c.classList.remove('open'));
      document.querySelectorAll('.nav-group-toggle').forEach(t => t.classList.remove('open'));

      if (!isOpen) {
        children.classList.add('open');
        toggle.classList.add('open');
      }
    });
  });

  // Active state from URL
  function setActive() {
    const path = window.location.pathname;
    document.querySelectorAll('.nav-item, .nav-item-child').forEach(el => {
      el.classList.remove('active');
      const href = el.getAttribute('href') || el.dataset.href;
      if (href && path.startsWith(href)) {
        el.classList.add('active');
      }
    });

    // Auto-expand parent group
    const activeChild = document.querySelector('.nav-item-child.active');
    if (activeChild) {
      const group = activeChild.closest('.nav-group-children');
      if (group) {
        group.classList.add('open');
        group.previousElementSibling.classList.add('open');
      }
    }
  }

  setActive();
  window.addEventListener('popstate', setActive);
})();
