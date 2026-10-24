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
  // Every answer commits through the page's own POST: objective answers on
  // change, essays once typing pauses. There is no Save button and no timer.
  // The POST targets a hidden iframe and the server answers 204, so the
  // question, its scroll offset, and the caret never move. fetch/XHR/beacons
  // are banned in this file; this is a plain form submission.
  document
    .querySelectorAll("form[data-attempt-form]")
    .forEach((form) => {
      const sink = document.createElement("iframe");
      sink.name = "attempt-sink";
      sink.hidden = true;
      sink.tabIndex = -1;
      sink.setAttribute("aria-hidden", "true");
      document.body.appendChild(sink);

      const quiet = document.createElement("button");
      quiet.type = "submit";
      quiet.name = "nav";
      quiet.value = "autosave";
      quiet.hidden = true;
      form.appendChild(quiet);

      const status = document.createElement("div");
      status.className = "visually-hidden";
      status.setAttribute("role", "status");
      status.setAttribute("aria-live", "polite");
      form.appendChild(status);

      let timer = 0;
      let inFlight = false;
      let queued = false;

      // A 204 leaves the current document in place, so the iframe may never
      // fire a load event. Release on a short window instead; while a commit
      // is in flight the next one waits, because two overlapping posts could
      // otherwise land out of order and the older answer would win.
      const commit = () => {
        if (inFlight) {
          queued = true;
          return;
        }
        inFlight = true;
        form.target = sink.name;
        quiet.click();
        form.target = "";
        window.setTimeout(() => {
          inFlight = false;
          if (queued) {
            queued = false;
            commit();
          }
        }, 400);
      };

      const stop = () => {
        window.clearTimeout(timer);
        timer = 0;
      };

      const schedule = (delay) => {
        stop();
        timer = window.setTimeout(() => {
          timer = 0;
          commit();
        }, delay);
      };

      form.querySelectorAll("input[type=radio], input[type=checkbox]").forEach((input) => {
        input.addEventListener("change", () => {
          stop();
          commit();
        });
      });

      // Essays commit after a pause in typing; leaving the field commits the
      // pending change at once instead of dropping it.
      const essay = form.querySelector(".essay-input");
      if (essay) {
        essay.addEventListener("input", () => schedule(1200));
        essay.addEventListener("blur", () => {
          if (!timer) return;
          stop();
          commit();
        });
      }

      form.addEventListener("submit", (event) => {
        const submitter = event.submitter;
        if (!submitter || submitter.disabled) return;

        // Automatic commits keep their submitter usable and stay silent.
        if (submitter === quiet) return;

        stop();
        // The explicit post carries the whole answer, so a queued quiet one
        // would only repeat it.
        queued = false;

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

        // Explicit navigation shows the DESIGN §8 spinner and announces it.
        status.textContent = "Menyimpan…";
        form.setAttribute("aria-busy", "true");
        const spinner = document.createElement("span");
        spinner.className = "spinner-border spinner-border-sm me-2";
        spinner.setAttribute("aria-hidden", "true");
        submitter.prepend(spinner);
      });
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
