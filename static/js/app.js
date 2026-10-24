// Twisty Quiz application behaviour (ES module). Bootstrap is imported by the
// bundle script tag; this file carries only app-specific wiring.
//
// NO network calls of any kind — fetch / XMLHttpRequest / EventSource /
// sendBeacon are banned here on purpose (the backend wiring arrives later).
// Everything is guarded so the file is inert on pages that do not carry the
// relevant elements.
export {};

document.addEventListener("DOMContentLoaded", () => {
  // ---- Password visibility -------------------------------------------------
  document.querySelectorAll("[data-toggle-password]").forEach((button) => {
    button.addEventListener("click", () => {
      const input = document.getElementById(button.dataset.togglePassword);
      if (!input) return;
      const show = input.type === "password";
      input.type = show ? "text" : "password";
      button.setAttribute("aria-pressed", show ? "true" : "false");
      button.setAttribute("aria-label", show ? "Sembunyikan kata sandi" : "Tampilkan kata sandi");
      const use = button.querySelector("use");
      if (use) use.setAttribute("href", show ? "#i-eye-off" : "#i-eye");
    });
  });

  // ---- Private quiz audience fields ---------------------------------------
  const visibility = document.querySelector("[data-visibility]");
  const privateFields = document.querySelector("[data-private-fields]");
  if (visibility && privateFields) {
    const syncVisibility = () => {
      const isPrivate = visibility.value === "private";
      privateFields.hidden = !isPrivate;
      privateFields.querySelectorAll("select, input").forEach((field) => {
        field.disabled = !isPrivate;
      });
    };
    visibility.addEventListener("change", syncVisibility);
    syncVisibility();
  }

  // ---- Question type editor ------------------------------------------------
  const typeInputs = document.querySelectorAll("[data-question-type]");
  const objectiveBlock = document.querySelector("[data-objective-only]");
  const essayBlock = document.querySelector("[data-essay-only]");
  const syncType = () => {
    const selected = document.querySelector("[data-question-type]:checked");
    const type = selected ? selected.value : "single";
    if (objectiveBlock) objectiveBlock.hidden = type === "essay";
    if (essayBlock) essayBlock.hidden = type !== "essay";
    document.querySelectorAll("[data-correct-mode]").forEach((el) => {
      el.hidden = type !== "multiple";
    });
    document.querySelectorAll("[data-single-mode]").forEach((el) => {
      el.hidden = type !== "single";
    });
  };
  typeInputs.forEach((input) => input.addEventListener("change", syncType));
  if (typeInputs.length) syncType();

  // ---- Repeatable options --------------------------------------------------
  const optionList = document.querySelector("[data-options]");
  const addOption = document.querySelector("[data-add-option]");
  if (optionList && addOption) {
    const refreshLetters = () => {
      optionList.querySelectorAll("[data-option-row]").forEach((row, index) => {
        const letter = row.querySelector("[data-option-letter]");
        if (letter) letter.textContent = String.fromCharCode(65 + index);
        const input = row.querySelector("input[type=text]");
        if (input) input.placeholder = `Pilihan ${String.fromCharCode(65 + index)}`;
      });
    };
    addOption.addEventListener("click", () => {
      const rows = optionList.querySelectorAll("[data-option-row]");
      if (!rows.length) return;
      const copy = rows[rows.length - 1].cloneNode(true);
      copy.querySelectorAll("input").forEach((input) => {
        if (input.type === "text") input.value = "";
        if (input.type === "radio" || input.type === "checkbox") input.checked = false;
      });
      optionList.appendChild(copy);
      refreshLetters();
    });
    optionList.addEventListener("click", (event) => {
      const remove = event.target.closest("[data-remove-option]");
      if (!remove) return;
      const rows = optionList.querySelectorAll("[data-option-row]");
      if (rows.length <= 2) return;
      remove.closest("[data-option-row]").remove();
      refreshLetters();
    });
  }

  // ---- Drag reorder --------------------------------------------------------
  const sortable = document.querySelector("[data-sortable]");
  if (sortable) {
    let dragged = null;
    const orderInput = document.getElementById("question-order");
    const syncOrder = () => {
      const items = Array.from(sortable.querySelectorAll("[data-sortable-item]"));
      if (orderInput) {
        orderInput.value = items.map((item) => item.dataset.id).join(",");
      }
      items.forEach((item, index) => {
        const number = item.querySelector(".question-index");
        if (number) number.textContent = String(index + 1);
        const up = item.querySelector("[data-move-up]");
        const down = item.querySelector("[data-move-down]");
        if (up) {
          up.disabled = index === 0;
          up.setAttribute("aria-label", `Naikkan soal ${index + 1}`);
        }
        if (down) {
          down.disabled = index === items.length - 1;
          down.setAttribute("aria-label", `Turunkan soal ${index + 1}`);
        }
      });
    };
    sortable.querySelectorAll("[data-sortable-item]").forEach((item) => {
      item.setAttribute("draggable", "true");
      item.addEventListener("dragstart", () => {
        dragged = item;
        item.classList.add("dragging");
      });
      item.addEventListener("dragend", () => {
        item.classList.remove("dragging");
        sortable.querySelectorAll("[data-sortable-item]").forEach((el) => el.classList.remove("drop-target"));
        dragged = null;
        syncOrder();
      });
      item.addEventListener("dragover", (event) => {
        event.preventDefault();
        if (!dragged || dragged === item) return;
        item.classList.add("drop-target");
        const box = item.getBoundingClientRect();
        const after = event.clientY > box.top + box.height / 2;
        sortable.insertBefore(dragged, after ? item.nextSibling : item);
      });
      item.addEventListener("dragleave", () => item.classList.remove("drop-target"));
    });
    sortable.querySelectorAll("[data-move-up], [data-move-down]").forEach((button) => {
      button.addEventListener("click", () => {
        const item = button.closest("[data-sortable-item]");
        if (!item) return;
        if (button.hasAttribute("data-move-up") && item.previousElementSibling) {
          sortable.insertBefore(item, item.previousElementSibling);
        }
        if (button.hasAttribute("data-move-down") && item.nextElementSibling) {
          sortable.insertBefore(item.nextElementSibling, item);
        }
        syncOrder();
        const next = item.querySelector("[data-move-up]:not(:disabled), [data-move-down]:not(:disabled)");
        (next || button).focus();
      });
    });
    syncOrder();
  }

  // ---- Copy buttons --------------------------------------------------------
  document.querySelectorAll("[data-copy]").forEach((button) => {
    button.addEventListener("click", async () => {
      const source = document.getElementById(button.dataset.copy);
      const text = button.dataset.copyText || (source ? source.value || source.textContent : "");
      if (!text) return;
      try {
        await navigator.clipboard.writeText(text.trim());
        const original = button.getAttribute("aria-label");
        button.setAttribute("aria-label", "Tersalin");
        window.setTimeout(() => {
          if (original) button.setAttribute("aria-label", original);
        }, 1500);
      } catch {
        if (source && source.select) source.select();
      }
    });
  });

  // ---- Confirmation --------------------------------------------------------
  document.querySelectorAll("form[data-confirm]").forEach((form) => {
    form.addEventListener("submit", (event) => {
      if (!window.confirm(form.dataset.confirm)) {
        event.preventDefault();
      }
    });
  });

  // ---- Character counter ---------------------------------------------------
  // With a ceiling (textarea maxlength), the counter reads "312 / 2000" so
  // the limit is visible before the browser starts truncating silently.
  document.querySelectorAll("[data-char-count]").forEach((input) => {
    const output = document.getElementById(input.dataset.charCount);
    if (!output) return;
    const limit = input.maxLength >= 0 ? input.maxLength : null;
    const update = () => {
      output.textContent = limit
        ? `${input.value.length} / ${limit} karakter`
        : `${input.value.length} karakter`;
    };
    input.addEventListener("input", update);
    update();
  });

  // ---- Attempt answers save themselves -------------------------------------
  // Commits ride the page's own POST through a hidden "quiet" submitter —
  // fetch/XHR/beacons are banned in this file — and the server answers with
  // the same question and no flash, so the reload reads as nothing happening.
  // Scroll position and keyboard focus survive that reload. The visible Simpan
  // button is the no-JS fallback and steps aside once autosave is armed.
  document
    .querySelectorAll("form[data-attempt-form]")
    .forEach((form) => {
      form.classList.add("is-autosave");

      const status = document.createElement("div");
      status.className = "visually-hidden";
      status.setAttribute("role", "status");
      status.setAttribute("aria-live", "polite");
      form.appendChild(status);

      const quiet = document.createElement("button");
      quiet.type = "submit";
      quiet.name = "nav";
      quiet.value = "autosave";
      quiet.hidden = true;
      quiet.setAttribute("data-quiet", "");
      form.appendChild(quiet);

      const remember = () => {
        const active = document.activeElement;
        const mark =
          active && form.contains(active) && active.matches("input, textarea")
            ? active.id
              ? "id:" + active.id
              : "value:" + active.value
            : null;
        try {
          window.sessionStorage.setItem(
            "attempt-retake",
            JSON.stringify({ scroll: Math.round(window.scrollY), mark })
          );
        } catch {
          // Storage may be unavailable; the reload just loses its position.
        }
      };

      let timer = 0;
      const save = () => {
        if (quiet.disabled) return;
        remember();
        quiet.click();
      };

      form.addEventListener("submit", (event) => {
        const submitter = event.submitter;
        if (!submitter || submitter.disabled) return;
        window.clearTimeout(timer);

        // Disabling the submitter (below) drops its name/value from the
        // submission, so mirror the routing pair into a hidden input first.
        if (submitter.name) {
          const mirror = document.createElement("input");
          mirror.type = "hidden";
          mirror.name = submitter.name;
          mirror.value = submitter.value;
          form.appendChild(mirror);
        }
        submitter.disabled = true;

        // Automatic commits stay silent; explicit ones show the DESIGN §8
        // spinner and announce themselves.
        if (submitter.hasAttribute("data-quiet")) return;
        status.textContent = "Menyimpan…";
        form.setAttribute("aria-busy", "true");
        const spinner = document.createElement("span");
        spinner.className = "spinner-border spinner-border-sm me-2";
        spinner.setAttribute("aria-hidden", "true");
        submitter.prepend(spinner);
      });

      form.querySelectorAll("input[type=radio], input[type=checkbox]").forEach((input) => {
        input.addEventListener("change", () => {
          window.clearTimeout(timer);
          timer = window.setTimeout(save, 450);
        });
      });

      const essay = form.querySelector(".essay-input");
      if (essay) {
        essay.addEventListener("input", () => {
          window.clearTimeout(timer);
          timer = window.setTimeout(save, 1400);
        });
      }

      // Put the reader back where they were after a quiet reload.
      let retake = null;
      try {
        retake = JSON.parse(window.sessionStorage.getItem("attempt-retake") || "null");
        window.sessionStorage.removeItem("attempt-retake");
      } catch {
        retake = null;
      }
      if (retake) {
        window.scrollTo(0, retake.scroll || 0);
        if (retake.mark) {
          const target = retake.mark.startsWith("id:")
            ? form.querySelector(`[id="${retake.mark.slice(3)}"]`)
            : form.querySelector(
                `input[type=radio][value="${retake.mark.slice(6)}"], input[type=checkbox][value="${retake.mark.slice(6)}"]`
              );
          if (target) target.focus({ preventScroll: true });
        }
      }
    });

  // ---- Leave guard while an attempt is open -------------------------------
  const attemptForm = document.querySelector("[data-attempt-form]");
  if (attemptForm) {
    let dirty = false;
    attemptForm.addEventListener("input", () => {
      dirty = true;
    });
    attemptForm.addEventListener("submit", () => {
      dirty = false;
    });
    window.addEventListener("beforeunload", (event) => {
      if (!dirty) return;
      event.preventDefault();
      event.returnValue = "";
    });
  }

  // ---- Bootstrap-style validation feedback --------------------------------
  document.querySelectorAll("form[data-validate]").forEach((form) => {
    form.addEventListener("submit", (event) => {
      if (!form.checkValidity()) {
        event.preventDefault();
        event.stopPropagation();
      }
      form.classList.add("was-validated");
    });
  });

  // ---- Nav drawer expanded state ------------------------------------------
  // Bootstrap's Drawer never mirrors open state back onto the toggler, so
  // watch the <dialog open> attribute instead and keep every matching
  // [data-bs-toggle="drawer"] in sync for screen readers.
  document.querySelectorAll("dialog.drawer").forEach((dialog) => {
    const toggles = document.querySelectorAll(
      `[data-bs-toggle="drawer"][data-bs-target="#${dialog.id}"]`
    );
    if (!toggles.length) return;
    const syncDrawer = () => {
      const open = dialog.hasAttribute("open");
      toggles.forEach((toggle) => {
        toggle.setAttribute("aria-expanded", open ? "true" : "false");
      });
    };
    new MutationObserver(syncDrawer).observe(dialog, { attributeFilter: ["open"] });
    syncDrawer();
  });
});
