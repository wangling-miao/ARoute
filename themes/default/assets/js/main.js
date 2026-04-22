(function () {
  "use strict";

  // Mobile navigation toggle
  var navToggle = document.getElementById("nav-toggle");
  var mainNav = document.getElementById("main-nav");

  if (navToggle && mainNav) {
    navToggle.addEventListener("click", function () {
      var expanded = navToggle.getAttribute("aria-expanded") === "true";
      navToggle.setAttribute("aria-expanded", String(!expanded));
      mainNav.classList.toggle("open");
    });

    // Close nav when clicking outside
    document.addEventListener("click", function (e) {
      if (
        !navToggle.contains(e.target) &&
        !mainNav.contains(e.target) &&
        mainNav.classList.contains("open")
      ) {
        navToggle.setAttribute("aria-expanded", "false");
        mainNav.classList.remove("open");
      }
    });

    // Close nav on Escape key
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && mainNav.classList.contains("open")) {
        navToggle.setAttribute("aria-expanded", "false");
        mainNav.classList.remove("open");
        navToggle.focus();
      }
    });
  }

  // Sticky header shadow on scroll
  var header = document.getElementById("site-header");

  if (header) {
    var scrolled = false;

    function onScroll() {
      var shouldShadow = window.scrollY > 10;
      if (shouldShadow !== scrolled) {
        scrolled = shouldShadow;
        header.classList.toggle("scrolled", scrolled);
      }
    }

    window.addEventListener("scroll", onScroll, { passive: true });
    onScroll();
  }

  // Smooth scroll for anchor links
  document.querySelectorAll('a[href^="#"]').forEach(function (link) {
    link.addEventListener("click", function (e) {
      var target = document.querySelector(this.getAttribute("href"));
      if (target) {
        e.preventDefault();
        target.scrollIntoView({ behavior: "smooth", block: "start" });
      }
    });
  });

  // Lazy image loading with IntersectionObserver
  if ("IntersectionObserver" in window) {
    var imgObserver = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (entry.isIntersecting) {
            var img = entry.target;
            if (img.dataset.src) {
              img.src = img.dataset.src;
              img.removeAttribute("data-src");
            }
            imgObserver.unobserve(img);
          }
        });
      },
      { rootMargin: "50px" }
    );

    document.querySelectorAll("img[data-src]").forEach(function (img) {
      imgObserver.observe(img);
    });
  }
})();
