"use strict";
(function () {
  const form = document.getElementById("login");
  const err = document.getElementById("error");
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    err.hidden = true;
    const btn = form.querySelector("button");
    btn.disabled = true;
    try {
      const res = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: document.getElementById("password").value }),
        credentials: "same-origin",
      });
      if (res.ok) {
        location.replace("/");
        return;
      }
      const data = await res.json().catch(() => ({}));
      err.textContent = data.error || "Login failed";
      err.hidden = false;
    } catch (x) {
      err.textContent = "Network error";
      err.hidden = false;
    } finally {
      btn.disabled = false;
    }
  });
})();
