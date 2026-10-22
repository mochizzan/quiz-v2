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
  // moves aria-pressed and swaps the btn-solid/btn-outline skin between the
  // two buttons. Inert on pages that don't carry a .role-switch group.
  document.querySelectorAll(".role-switch").forEach((group) => {
    const buttons = Array.from(group.querySelectorAll("button[aria-pressed]"));
    buttons.forEach((btn) => {
      btn.addEventListener("click", () => {
        buttons.forEach((other) => {
          const on = other === btn;
          other.setAttribute("aria-pressed", on ? "true" : "false");
          other.classList.toggle("btn-solid", on);
          other.classList.toggle("btn-outline", !on);
        });
      });
    });
  });
});
