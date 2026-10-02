"use strict";
// Makes the dashboard installable. The page and static files are network
// first with the last copy kept for offline starts; the API is never cached.
const CACHE = "baton-v1";
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => {
  e.waitUntil(caches.keys()
    .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
    .then(() => self.clients.claim()));
});
self.addEventListener("fetch", (e) => {
  const u = new URL(e.request.url);
  if (e.request.method !== "GET" || u.origin !== location.origin || !(u.pathname === "/" || u.pathname.startsWith("/static/"))) return;
  e.respondWith(fetch(e.request).then((res) => {
    // A redirect (to /login) is not the page.
    if (res.ok && !res.redirected) {
      const copy = res.clone();
      caches.open(CACHE).then((c) => c.put(e.request, copy));
    }
    return res;
  }).catch(() => caches.match(e.request).then((res) => res || Response.error())));
});
