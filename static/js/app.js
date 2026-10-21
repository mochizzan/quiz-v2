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
});
