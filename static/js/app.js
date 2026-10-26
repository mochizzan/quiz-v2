// Twisty Quiz application behaviour (ES module). Bootstrap's bundle is loaded
// by the script tag in foot.html and re-imported here only for the components
// this file drives directly (window.bootstrap is banned); everything else is
// data-API wiring.
//
// NO network calls of any kind — fetch / XMLHttpRequest / EventSource /
// sendBeacon are banned here on purpose (the backend wiring arrives later).
// Everything is guarded so the file is inert on pages that do not carry the
// relevant elements.
import { Dialog } from "../vendor/bootstrap/bootstrap.bundle.min.js";

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

  // ---- Quiz settings: attempts follow the retake toggle --------------------
  // The server already ignores max_attempts while allow_retake is off
  // (AUT-005), so the number field is hidden and disabled until the toggle
  // asks for it. Without JavaScript both stay visible; the saved value is
  // unchanged either way.
  const retakeToggle = document.querySelector("[data-retake-toggle]");
  const retakeField = document.querySelector("[data-retake-field]");
  if (retakeToggle && retakeField) {
    const syncRetake = () => {
      const allowed = retakeToggle.checked;
      retakeField.hidden = !allowed;
      retakeField.querySelectorAll("input, select").forEach((field) => {
        field.disabled = !allowed;
      });
    };
    retakeToggle.addEventListener("change", syncRetake);
    syncRetake();
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

  // ---- Question image attachment -------------------------------------------
  // The server already renders a saved image; this block only adds the live
  // preview for a file chosen in this session and keeps removal reversible
  // until the form is saved.
  const imageField = document.querySelector("[data-image-field]");
  if (imageField) {
    const input = imageField.querySelector("[data-image-input]");
    const empty = imageField.querySelector("[data-image-empty]");
    const attached = imageField.querySelector("[data-image-attached]");
    const thumb = imageField.querySelector("[data-image-thumb]");
    const title = imageField.querySelector("[data-image-title]");
    const meta = imageField.querySelector("[data-image-meta]");
    const pickLabel = imageField.querySelector("[data-image-pick-label]");
    const remove = imageField.querySelector("[data-image-remove]");
    const removeInput = imageField.querySelector("[data-image-remove-input]");
    const cancel = imageField.querySelector("[data-image-cancel]");
    const error = imageField.querySelector("[data-image-error]");

    const saved = {
      has: !attached.hidden,
      src: thumb.getAttribute("src") || "",
      title: title.textContent,
      meta: meta.textContent,
    };
    const allowedTypes = ["image/jpeg", "image/png", "image/webp"];
    const maxBytes = 10 * 1024 * 1024;
    let previewURL = "";
    let staged = false;
    let removeWasChecked = false;

    const formatBytes = (bytes) => {
      if (bytes < 1024) return `${bytes} B`;
      const units = ["KB", "MB", "GB"];
      let value = bytes / 1024;
      let unit = 0;
      while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit += 1;
      }
      return `${value.toFixed(1).replace(".", ",")} ${units[unit]}`;
    };

    const setThumb = (src) => {
      if (src) thumb.setAttribute("src", src);
      else thumb.removeAttribute("src");
    };

    const clearPreview = () => {
      if (previewURL) URL.revokeObjectURL(previewURL);
      previewURL = "";
    };

    const setError = (message) => {
      error.textContent = message;
      error.hidden = message === "";
      if (message) input.setAttribute("aria-invalid", "true");
      else input.removeAttribute("aria-invalid");
    };

    const showSaved = () => {
      clearPreview();
      attached.hidden = !saved.has;
      empty.hidden = saved.has;
      setThumb(saved.src);
      title.textContent = saved.title;
      meta.textContent = saved.meta;
      pickLabel.textContent = saved.has ? "Ganti gambar" : "Pilih gambar";
      cancel.hidden = true;
      remove.hidden = !saved.has;
      if (staged) removeInput.checked = removeWasChecked;
      staged = false;
    };

    const showStaged = (file) => {
      if (!staged) {
        removeWasChecked = removeInput.checked;
        staged = true;
      }
      clearPreview();
      previewURL = URL.createObjectURL(file);
      setThumb(previewURL);
      title.textContent = file.name;
      meta.textContent = formatBytes(file.size);
      attached.hidden = false;
      empty.hidden = true;
      pickLabel.textContent = "Ganti gambar";
      remove.hidden = true;
      removeInput.checked = false;
      cancel.hidden = false;
      setError("");
    };

    input.addEventListener("change", () => {
      const file = input.files && input.files[0];
      if (!file) {
        showSaved();
        setError("");
        return;
      }
      if (file.size > maxBytes) {
        input.value = "";
        showSaved();
        setError("Gambar maksimal 10 MB.");
        input.focus();
        return;
      }
      if (file.type && !allowedTypes.includes(file.type)) {
        input.value = "";
        showSaved();
        setError("Gambar harus berupa JPEG, PNG, atau WebP.");
        input.focus();
        return;
      }
      showStaged(file);
    });

    // A file dragged anywhere over the editor gets the whole page as its
    // target. The overlay is pointer-events:none, so dragenter/dragleave stay
    // on the page underneath and the depth count remains stable.
    const dropOverlay = document.querySelector("[data-image-drop]");
    if (dropOverlay) {
      const imageName = /\.(jpe?g|png|webp)$/i;
      let dragDepth = 0;

      const hasFiles = (event) => {
        const types = event.dataTransfer && event.dataTransfer.types;
        return !!types && Array.from(types).includes("Files");
      };

      const hasImage = (event) => {
        if (!hasFiles(event)) return false;
        const items = Array.from(event.dataTransfer.items || []).filter(
          (item) => item.kind === "file",
        );
        // Some browsers withhold the MIME type until drop; show the target
        // for those instead of making the drag feel ignored.
        return items.length === 0 || items.some(
          (item) => !item.type || item.type.startsWith("image/"),
        );
      };

      const showDropOverlay = () => {
        if (!dropOverlay.hidden) return;
        dropOverlay.hidden = false;
        dropOverlay.setAttribute("aria-hidden", "false");
      };

      const hideDropOverlay = () => {
        dragDepth = 0;
        if (dropOverlay.hidden) return;
        dropOverlay.hidden = true;
        dropOverlay.setAttribute("aria-hidden", "true");
      };

      window.addEventListener("dragenter", (event) => {
        if (!hasImage(event)) return;
        event.preventDefault();
        dragDepth += 1;
        showDropOverlay();
      });

      window.addEventListener("dragover", (event) => {
        if (!hasImage(event)) return;
        event.preventDefault();
        if (event.dataTransfer) event.dataTransfer.dropEffect = "copy";
        showDropOverlay();
      });

      window.addEventListener("dragleave", (event) => {
        if (dropOverlay.hidden) return;
        if (!event.relatedTarget) {
          hideDropOverlay();
          return;
        }
        dragDepth = Math.max(0, dragDepth - 1);
        if (dragDepth === 0) hideDropOverlay();
      });

      window.addEventListener("drop", (event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        hideDropOverlay();

        const file = event.dataTransfer.files && event.dataTransfer.files[0];
        if (!file) return;

        const looksLikeImage = file.type
          ? allowedTypes.includes(file.type)
          : imageName.test(file.name);
        if (!looksLikeImage) {
          setError("Gambar harus berupa JPEG, PNG, atau WebP.");
          input.focus();
          return;
        }

        try {
          const transfer = new DataTransfer();
          transfer.items.add(file);
          input.files = transfer.files;
          input.dispatchEvent(new Event("change", { bubbles: true }));
          if (!error.textContent) {
            imageField.scrollIntoView({ block: "nearest" });
          }
        } catch {
          setError("Gambar tidak dapat dipasang. Pilih lewat tombol Pilih gambar.");
          input.focus();
        }
      });

      window.addEventListener("dragend", hideDropOverlay);
      window.addEventListener("blur", hideDropOverlay);
      document.addEventListener("keydown", (event) => {
        if (event.key === "Escape") hideDropOverlay();
      });
    }

    cancel.addEventListener("click", () => {
      input.value = "";
      showSaved();
      setError("");
      input.focus();
    });

    window.addEventListener("pagehide", clearPreview);
  }

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
    let dirty = false;
    const reorderForm = sortable.closest("form");
    const saveOrder = reorderForm ? reorderForm.querySelector("[data-save-order]") : null;
    const orderInput = document.getElementById("question-order");
    const canSaveOrder = Boolean(reorderForm && saveOrder && orderInput);
    const baselineOrder = Array.from(sortable.querySelectorAll("[data-sortable-item]"))
      .map((item) => item.dataset.id)
      .join(",");
    const syncOrder = () => {
      const items = Array.from(sortable.querySelectorAll("[data-sortable-item]"));
      const order = items.map((item) => item.dataset.id).join(",");
      if (orderInput) {
        orderInput.value = order;
      }
      if (canSaveOrder) {
        dirty = order !== baselineOrder;
        saveOrder.hidden = !dirty;
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
    if (canSaveOrder) {
      reorderForm.addEventListener("submit", () => {
        dirty = false;
      });
      window.addEventListener("beforeunload", (event) => {
        if (!dirty) return;
        event.preventDefault();
        event.returnValue = "";
      });
    }
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

  // ---- Confirmation dialog -------------------------------------------------
  // The browser's own confirm() chrome is banned, so every form[data-confirm]
  // asks through the shared Bootstrap 6 dialog (partials/dialogs.html): the
  // attribute supplies the question, the pressed button supplies the label.
  const confirmDialog = document.getElementById("dialog-confirm");
  const confirmMessage = document.getElementById("dialog-confirm-message");
  const confirmAccept = document.getElementById("dialog-confirm-accept");
  if (confirmDialog && confirmMessage && confirmAccept) {
    const confirmInstance = Dialog.getOrCreateInstance(confirmDialog);
    let pending = null;

    const acceptLabel = (submitter) => {
      if (!submitter) return "Konfirmasi";
      if (submitter.dataset.confirmLabel) return submitter.dataset.confirmLabel;
      const text = submitter.textContent.replace(/\s+/g, " ").trim();
      return text || "Konfirmasi";
    };

    document.querySelectorAll("form[data-confirm]").forEach((form) => {
      form.addEventListener("submit", (event) => {
        // The post-dialog submit is the same form asking a second time.
        if (form.dataset.confirmed === "true") {
          delete form.dataset.confirmed;
          return;
        }
        event.preventDefault();
        pending = { form, submitter: event.submitter };
        confirmMessage.textContent = form.dataset.confirm;
        confirmAccept.textContent = acceptLabel(event.submitter);
        confirmInstance.show(event.submitter);
      });
    });

    // Accept submits at once instead of on close: Bootstrap drops a hide()
    // issued while the opening transition still runs, and a sticky "approved"
    // flag would then fire on a later Escape or backdrop click.
    confirmAccept.addEventListener("click", () => {
      const target = pending;
      pending = null;
      if (!target) return;
      target.form.dataset.confirmed = "true";
      target.form.requestSubmit(target.submitter || undefined);
      window.setTimeout(() => {
        delete target.form.dataset.confirmed;
      }, 0);
    });

    // Every close that is not an accept (Batal, Escape, the backdrop) forgets
    // the form it was asking about.
    confirmDialog.addEventListener("hidden.bs.dialog", () => {
      pending = null;
    });
  }

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

  // ---- Dirty-form save gates ------------------------------------------------
  // A form opts in with [data-dirty-form]; its [data-dirty-actions] wrapper
  // stays hidden while every field equals the value the server rendered, so the
  // page does not offer a save action before there is anything to save. The
  // whole wrapper goes, not just the button, so no empty action bar remains.
  // A 422 re-render marks the form [data-dirty-open], and a browser without
  // JavaScript never runs this block, so the submit stays available in both
  // cases. Comparing against defaultValue/defaultChecked rather than a snapshot
  // keeps autofill from being mistaken for a pristine form.
  const isDirty = (form) => {
    let dirty = false;
    form.querySelectorAll("input, select, textarea").forEach((field) => {
      if (!field.name || field.disabled) return;
      if (["submit", "button", "reset", "file", "image", "hidden"].includes(field.type)) return;
      if (field.type === "checkbox" || field.type === "radio") {
        if (field.checked !== field.defaultChecked) dirty = true;
        return;
      }
      if (field.tagName === "SELECT") {
        const options = Array.from(field.options);
        const initial = options.find((option) => option.defaultSelected);
        const baseline = initial ? initial.value : options.length ? options[0].value : "";
        if (field.value !== baseline) dirty = true;
        return;
      }
      if (field.value !== field.defaultValue) dirty = true;
    });
    return dirty;
  };

  document.querySelectorAll("form[data-dirty-form]").forEach((form) => {
    const actions = form.querySelector("[data-dirty-actions]");
    const submit = form.querySelector("[data-dirty-submit]");
    const surface = actions || submit;
    if (!surface || form.hasAttribute("data-dirty-open")) return;

    const sync = () => {
      const dirty = isDirty(form);
      surface.hidden = !dirty;
      // An implicit submit (Enter in a text field) uses the form's default
      // button even while its action row is hidden. Disable it with the row so
      // a pristine form cannot post through the keyboard either.
      if (submit) submit.disabled = !dirty;
    };
    form.addEventListener("input", sync);
    form.addEventListener("change", sync);
    sync();
  });

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
