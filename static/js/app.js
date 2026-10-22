// Twisty Quiz application behaviour (ES module). Bootstrap is imported by the
// bundle script tag; this file carries only app-specific wiring.
//
// Task 14 behaviour (plan §14): essay character counter + autosave-indicator
// flash. NO network calls of any kind — fetch / XMLHttpRequest / EventSource /
// sendBeacon are banned here on purpose (the backend wiring arrives with the
// backend plan). Everything is guarded with `if (el)` so the file is inert on
// pages that don't carry these elements.
export {};

document.addEventListener("DOMContentLoaded", () => {
  const essayInput = document.getElementById("essay-input");
  const charCount = document.getElementById("char-count");
  const autosave = document.getElementById("autosave-indicator");

  if (essayInput) {
    essayInput.addEventListener("input", () => {
      if (charCount) {
        charCount.textContent = `${essayInput.value.length} karakter`;
      }
      if (autosave) {
        autosave.classList.add("show");
        window.setTimeout(() => autosave.classList.remove("show"), 1200);
      }
    });
  }

  // Role switch (Guru/Siswa segmented control, plan §15.7): clicking a side
  // moves aria-pressed, swaps the btn-solid/btn-outline skin, and reveals the
  // blocks scoped to that role ([data-role-only]). A ?role=guru|?role=siswa
  // deep link (the login register cards) preselects the side. Inert on pages
  // that don't carry a .role-switch group.
  document.querySelectorAll(".role-switch").forEach((group) => {
    const buttons = Array.from(group.querySelectorAll("button[aria-pressed]"));

    const applyRole = (active) => {
      const role = active.getAttribute("data-role");
      buttons.forEach((btn) => {
        const on = btn === active;
        btn.setAttribute("aria-pressed", on ? "true" : "false");
        btn.classList.toggle("btn-solid", on);
        btn.classList.toggle("btn-outline", !on);
      });
      document.querySelectorAll("[data-role-only]").forEach((block) => {
        block.hidden = block.getAttribute("data-role-only") !== role;
      });
    };

    buttons.forEach((btn) => btn.addEventListener("click", () => applyRole(btn)));

    const wanted = new URLSearchParams(window.location.search).get("role");
    const preset = buttons.find((btn) => btn.getAttribute("data-role") === wanted);
    if (preset) applyRole(preset);
  });

  // Password visibility (login + register eye buttons): flip the field type,
  // the accessible name, and the eye sprite id (i-eye <-> i-eye-off).
  // Inert on pages without a [data-password-toggle] button.
  document.querySelectorAll("[data-password-toggle]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const field = document.getElementById(btn.getAttribute("data-password-toggle"));
      if (!field) return;
      const show = field.type === "password";
      field.type = show ? "text" : "password";
      btn.setAttribute("aria-label", show ? "Sembunyikan kata sandi" : "Tampilkan kata sandi");
      const icon = btn.querySelector("use");
      if (icon) icon.setAttribute("href", show ? "#i-eye-off" : "#i-eye");
    });
  });
});
