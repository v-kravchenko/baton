"use strict";
// Loaded in <head> before first paint: no flash of the wrong theme.
try {
  const t = localStorage.getItem("baton.theme");
  if (t) document.documentElement.dataset.theme = t;
} catch (e) { /* private mode */ }
