// Twisty Quiz profile-photo editor. The server owns the authoritative decode,
// EXIF fix, square crop and WebP encode (internal/avatar). This module only
// gives the user a truthful preview of that square and exports the chosen
// crop into the same `photo` field, so the upload stays a plain multipart POST
// and keeps working unchanged with JavaScript off.
//
// The module is page-scoped: the profile templates emit it through
// partials/page-script, and every lookup is guarded so it is inert elsewhere.
// No network calls of any kind — the file input is the only transport.
export {};

// Mirrors of the server limits (internal/avatar and httpserver/pipeline.go).
// They are used for early feedback only; the server stays authoritative.
const MAX_FILE_BYTES = 200_000_000;
const MAX_CONTAINER_BYTES = 64 * 1024 * 1024;
const MAX_PIXELS = 50_000_000;
const MAX_SIDE = 16_000;
const PREVIEW_MAX_SIDE = 3072;
const EXPORT_SIZE = 1024;
const ZOOM_MIN = 1;
const ZOOM_MAX = 4;

document.querySelectorAll("form[data-photo-form]").forEach(init);

function init(form) {
  const input = form.querySelector("[data-photo-input]");
  const drop = form.querySelector("[data-photo-drop]");
  const dropTitle = form.querySelector("[data-photo-drop-title]");
  const dropSub = form.querySelector("[data-photo-drop-sub]");
  const editor = form.querySelector("[data-photo-editor]");
  const stage = form.querySelector("[data-photo-stage]");
  const crop = form.querySelector("[data-photo-crop]");
  const canvas = form.querySelector("[data-photo-view]");
  const thumb = form.querySelector("[data-photo-thumb]");
  const zoom = form.querySelector("[data-photo-zoom]");
  const zoomValue = form.querySelector("[data-photo-zoom-value]");
  const resetButton = form.querySelector("[data-photo-reset]");
  const cancelButton = form.querySelector("[data-photo-cancel]");
  const submitButton = form.querySelector("[data-photo-upload]");
  const actions = form.querySelector("[data-photo-actions]");
  const error = form.querySelector("[data-photo-error]");
  const note = form.querySelector("[data-photo-note]");
  const loading = form.querySelector("[data-photo-loading]");

  if (!input || !drop || !editor || !stage || !crop || !canvas || !thumb || !zoom || !zoomValue || !error || !loading) {
    return;
  }

  const ctx = canvas.getContext("2d");
  const thumbCtx = thumb.getContext("2d");
  if (!ctx || !thumbCtx) return;

  const state = {
    image: null,
    file: null,
    exportable: false,
    prepared: false,
    zoom: 1,
    centerX: 0.5,
    centerY: 0.5,
  };

  let loadToken = 0;
  let dragging = false;
  let lastX = 0;
  let lastY = 0;
  let exporting = false;

  if (actions) actions.hidden = true;

  const clamp = (value, min, max) => Math.min(max, Math.max(min, value));

  const extension = (name) => {
    const dot = (name || "").lastIndexOf(".");
    return dot < 0 ? "" : name.slice(dot + 1).toLowerCase();
  };
  const mime = (file) => (file && file.type ? file.type.toLowerCase() : "");
  const isHeic = (file) =>
    mime(file) === "image/heic" ||
    mime(file) === "image/heif" ||
    ["heic", "heif"].includes(extension(file.name));
  const isContainer = (file) =>
    isHeic(file) || mime(file) === "image/webp" || extension(file.name) === "webp";

  const validateFile = (file) => {
    if (!file) return "Pilih foto terlebih dahulu.";
    const type = mime(file);
    const supportedType = [
      "image/jpeg",
      "image/png",
      "image/webp",
      "image/heic",
      "image/heif",
    ].includes(type);
    const supportedName = ["jpg", "jpeg", "png", "webp", "heic", "heif"].includes(
      extension(file.name)
    );
    if (!supportedType && !supportedName) {
      return "Format foto harus JPG, PNG, WebP, atau HEIC.";
    }
    if (file.size > MAX_FILE_BYTES) return "Foto terlalu besar untuk diproses.";
    if (isContainer(file) && file.size > MAX_CONTAINER_BYTES) {
      return "Foto terlalu besar untuk diproses.";
    }
    return "";
  };

  const assignFile = (file) => {
    try {
      const transfer = new DataTransfer();
      transfer.items.add(file);
      input.files = transfer.files;
      return true;
    } catch {
      return false;
    }
  };

  const setDropText = (title, sub) => {
    if (dropTitle) dropTitle.textContent = title;
    if (dropSub) dropSub.textContent = sub;
  };
  const showPristineDrop = () => {
    setDropText("Tarik foto ke sini", "atau pilih file dari perangkat");
  };
  const showPickedDrop = (file, title = "Foto siap diatur") => {
    setDropText(title, file && file.name ? file.name : "Klik atau tarik untuk mengganti");
  };

  const showError = (message) => {
    error.textContent = message;
    error.hidden = message === "";
    if (message) input.setAttribute("aria-invalid", "true");
    else input.removeAttribute("aria-invalid");
  };
  const hideError = () => showError("");

  const showNote = (message) => {
    if (!note) return;
    note.textContent = message;
    note.hidden = message === "";
  };
  const hideNote = () => showNote("");

  const showLoading = (on) => {
    loading.hidden = !on;
  };

  const syncZoom = () => {
    const percent = Math.round(state.zoom * 100);
    zoom.value = state.zoom.toFixed(2);
    zoomValue.textContent = `${percent}%`;
    zoom.setAttribute("aria-valuetext", `${percent} persen`);
  };

  const metrics = () => {
    const side = crop.clientWidth || 1;
    const cover = Math.max(side / state.image.width, side / state.image.height);
    return {
      side,
      width: state.image.width * cover * state.zoom,
      height: state.image.height * cover * state.zoom,
    };
  };

  const offsets = (size) => ({
    x: size.side / 2 - state.centerX * size.width,
    y: size.side / 2 - state.centerY * size.height,
  });

  const clampCenter = (size) => {
    const minX = size.side / (2 * size.width);
    const minY = size.side / (2 * size.height);
    state.centerX = clamp(state.centerX, minX, 1 - minX);
    state.centerY = clamp(state.centerY, minY, 1 - minY);
  };

  const renderThumb = () => {
    const side = 128;
    if (thumb.width !== side || thumb.height !== side) {
      thumb.width = side;
      thumb.height = side;
    }
    thumbCtx.clearRect(0, 0, side, side);
    thumbCtx.imageSmoothingEnabled = true;
    thumbCtx.imageSmoothingQuality = "high";
    thumbCtx.drawImage(canvas, 0, 0, canvas.width, canvas.height, 0, 0, side, side);
  };

  const render = () => {
    if (!state.image) return;
    const size = metrics();
    if (size.side <= 1) return;
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const px = Math.max(1, Math.round(size.side * dpr));
    if (canvas.width !== px || canvas.height !== px) {
      canvas.width = px;
      canvas.height = px;
    }
    const offset = offsets(size);
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.imageSmoothingEnabled = true;
    ctx.imageSmoothingQuality = "high";
    ctx.drawImage(state.image, offset.x, offset.y, size.width, size.height);
    renderThumb();
  };

  const panBy = (dx, dy) => {
    if (!state.image) return;
    const size = metrics();
    state.centerX -= dx / size.width;
    state.centerY -= dy / size.height;
    clampCenter(size);
    render();
  };

  const setZoom = (next) => {
    if (!state.image) return;
    state.zoom = clamp(next, ZOOM_MIN, ZOOM_MAX);
    clampCenter(metrics());
    syncZoom();
    render();
  };

  // Zoom around a point in the crop square, so a wheel gesture keeps the
  // subject under the pointer instead of drifting. The slider uses
  // setZoom(), which anchors on the current centre.
  const zoomAt = (next, anchorX, anchorY) => {
    if (!state.image) return;
    const before = metrics();
    const offset = offsets(before);
    const focusX = (anchorX - offset.x) / before.width;
    const focusY = (anchorY - offset.y) / before.height;
    state.zoom = clamp(next, ZOOM_MIN, ZOOM_MAX);
    const after = metrics();
    state.centerX = (after.side / 2 - (anchorX - focusX * after.width)) / after.width;
    state.centerY = (after.side / 2 - (anchorY - focusY * after.height)) / after.height;
    clampCenter(after);
    syncZoom();
    render();
  };

  const resetView = () => {
    if (!state.image) return;
    state.zoom = 1;
    state.centerX = 0.5;
    state.centerY = 0.5;
    syncZoom();
    render();
  };

  const releaseImage = () => {
    if (state.image && typeof state.image.close === "function") state.image.close();
    state.image = null;
  };

  const resetPreview = () => {
    loadToken += 1;
    releaseImage();
    state.file = null;
    state.exportable = false;
    state.prepared = false;
    state.zoom = 1;
    state.centerX = 0.5;
    state.centerY = 0.5;
    editor.hidden = true;
    if (actions) actions.hidden = true;
    showLoading(false);
    hideError();
    hideNote();
    syncZoom();
  };

  const discard = (focusInput) => {
    resetPreview();
    input.value = "";
    showPristineDrop();
    if (focusInput) input.focus({ preventScroll: true });
  };

  const decodeFile = async (file) => {
    if (typeof createImageBitmap === "function") {
      let bitmap = null;
      try {
        bitmap = await createImageBitmap(file, { imageOrientation: "from-image" });
      } catch {
        bitmap = null;
      }
      if (bitmap) {
        const width = bitmap.width;
        const height = bitmap.height;
        let source = bitmap;
        if (Math.max(width, height) > PREVIEW_MAX_SIDE) {
          const ratio = PREVIEW_MAX_SIDE / Math.max(width, height);
          try {
            source = await createImageBitmap(bitmap, {
              resizeWidth: Math.max(1, Math.round(width * ratio)),
              resizeHeight: Math.max(1, Math.round(height * ratio)),
              resizeQuality: "high",
            });
          } catch {
            source = bitmap;
          }
        }
        if (source !== bitmap && typeof bitmap.close === "function") bitmap.close();
        return { source, width, height };
      }
    }

    const url = URL.createObjectURL(file);
    try {
      const image = new Image();
      image.decoding = "async";
      image.src = url;
      await image.decode();
      const width = image.naturalWidth || image.width;
      const height = image.naturalHeight || image.height;
      if (!width || !height) throw new Error("empty image");
      let source = image;
      if (Math.max(width, height) > PREVIEW_MAX_SIDE) {
        const ratio = PREVIEW_MAX_SIDE / Math.max(width, height);
        const surface = document.createElement("canvas");
        surface.width = Math.max(1, Math.round(width * ratio));
        surface.height = Math.max(1, Math.round(height * ratio));
        const surfaceCtx = surface.getContext("2d");
        if (surfaceCtx) {
          surfaceCtx.imageSmoothingEnabled = true;
          surfaceCtx.imageSmoothingQuality = "high";
          surfaceCtx.drawImage(image, 0, 0, surface.width, surface.height);
          source = surface;
        }
      }
      return { source, width, height };
    } finally {
      URL.revokeObjectURL(url);
    }
  };

  const loadFile = async (file, fromDrop) => {
    resetPreview();
    const problem = validateFile(file);
    if (problem) {
      input.value = "";
      showPristineDrop();
      showError(problem);
      input.focus({ preventScroll: true });
      return;
    }
    if (fromDrop && !assignFile(file)) {
      input.value = "";
      showPristineDrop();
      showError("Peramban ini tidak mendukung unggah lewat tarik-lepas. Pilih file dari perangkat.");
      input.focus({ preventScroll: true });
      return;
    }

    state.file = file;
    showLoading(true);
    const token = ++loadToken;
    let decoded = null;
    try {
      decoded = await decodeFile(file);
    } catch {
      decoded = null;
    }
    if (token !== loadToken) {
      if (decoded && typeof decoded.source.close === "function") decoded.source.close();
      return;
    }
    showLoading(false);

    if (!decoded) {
      if (isHeic(file)) {
        // HEIC decodes on the server (libheif) even where the browser cannot
        // draw it. Keep the original file and submit without a preview.
        showPickedDrop(file, "Foto siap diunggah");
        showNote("Pratinjau tidak tersedia di peramban ini. Foto akan diproses otomatis oleh server.");
        if (actions) actions.hidden = false;
      } else {
        input.value = "";
        showPristineDrop();
        showError("Foto tidak dapat dibaca. Coba file lain.");
        input.focus({ preventScroll: true });
      }
      return;
    }

    const { source, width, height } = decoded;
    if (width * height > MAX_PIXELS || width > MAX_SIDE || height > MAX_SIDE) {
      if (typeof source.close === "function") source.close();
      input.value = "";
      showPristineDrop();
      showError("Foto terlalu besar untuk diproses.");
      input.focus({ preventScroll: true });
      return;
    }

    state.image = source;
    state.exportable = true;
    state.zoom = 1;
    state.centerX = 0.5;
    state.centerY = 0.5;
    showPickedDrop(file);
    syncZoom();
    editor.hidden = false;
    if (actions) actions.hidden = false;
    render();
  };

  const exportCrop = () =>
    new Promise((resolve, reject) => {
      if (!state.image) {
        reject(new Error("no image"));
        return;
      }
      const size = metrics();
      const offset = offsets(size);
      const sourceX = (-offset.x / size.width) * state.image.width;
      const sourceY = (-offset.y / size.height) * state.image.height;
      const sourceW = (size.side / size.width) * state.image.width;
      const sourceH = (size.side / size.height) * state.image.height;
      const surface = document.createElement("canvas");
      surface.width = EXPORT_SIZE;
      surface.height = EXPORT_SIZE;
      const surfaceCtx = surface.getContext("2d");
      if (!surfaceCtx) {
        reject(new Error("no canvas"));
        return;
      }
      surfaceCtx.fillStyle = "#FFFFFF";
      surfaceCtx.fillRect(0, 0, EXPORT_SIZE, EXPORT_SIZE);
      surfaceCtx.imageSmoothingEnabled = true;
      surfaceCtx.imageSmoothingQuality = "high";
      surfaceCtx.drawImage(
        state.image,
        sourceX,
        sourceY,
        sourceW,
        sourceH,
        0,
        0,
        EXPORT_SIZE,
        EXPORT_SIZE
      );
      surface.toBlob(
        (blob) => {
          if (!blob) {
            reject(new Error("encode failed"));
            return;
          }
          const base = (state.file && state.file.name ? state.file.name : "foto").replace(
            /\.[^.]+$/,
            ""
          );
          resolve(
            new File([blob], `${base || "foto"}.jpg`, {
              type: "image/jpeg",
              lastModified: Date.now(),
            })
          );
        },
        "image/jpeg",
        0.92
      );
    });

  // ---- File selection ------------------------------------------------------
  input.addEventListener("change", () => {
    const file = input.files && input.files[0];
    if (!file) {
      discard(false);
      return;
    }
    loadFile(file, false);
  });

  const isFileDrag = (event) =>
    Boolean(event.dataTransfer) &&
    Array.from(event.dataTransfer.types || []).includes("Files");

  drop.addEventListener("dragenter", (event) => {
    if (!isFileDrag(event)) return;
    event.preventDefault();
    drop.dataset.dragover = "true";
  });
  drop.addEventListener("dragover", (event) => {
    if (!isFileDrag(event)) return;
    event.preventDefault();
    drop.dataset.dragover = "true";
    if (event.dataTransfer) event.dataTransfer.dropEffect = "copy";
  });
  drop.addEventListener("dragleave", (event) => {
    if (event.relatedTarget instanceof Node && drop.contains(event.relatedTarget)) return;
    delete drop.dataset.dragover;
  });
  drop.addEventListener("drop", (event) => {
    if (!isFileDrag(event)) return;
    event.preventDefault();
    delete drop.dataset.dragover;
    const file = event.dataTransfer.files && event.dataTransfer.files[0];
    if (file) loadFile(file, true);
  });

  // A file dropped outside the zone must never navigate the page away and
  // lose the edit. The zone's own handlers run first; this only swallows the
  // browser default everywhere else.
  const swallowFileDrop = (event) => {
    if (isFileDrag(event)) event.preventDefault();
  };
  document.addEventListener("dragover", swallowFileDrop);
  document.addEventListener("drop", swallowFileDrop);

  // ---- Pan, zoom and keyboard controls ------------------------------------
  stage.addEventListener("pointerdown", (event) => {
    if (!state.image || event.button !== 0) return;
    dragging = true;
    lastX = event.clientX;
    lastY = event.clientY;
    if (typeof stage.setPointerCapture === "function") {
      try {
        stage.setPointerCapture(event.pointerId);
      } catch {
        // Pointer capture is an enhancement; dragging still works without it.
      }
    }
    // A pointerdown on a tabindex element would normally focus it, but the
    // preventDefault below suppresses that. Focus it explicitly so the arrow
    // keys and +/- keep working after a drag; :focus-visible keeps the ring
    // off for pointer users.
    if (typeof stage.focus === "function") stage.focus({ preventScroll: true });
    event.preventDefault();
  });
  stage.addEventListener("pointermove", (event) => {
    if (!dragging) return;
    panBy(event.clientX - lastX, event.clientY - lastY);
    lastX = event.clientX;
    lastY = event.clientY;
  });
  const endDrag = (event) => {
    if (!dragging) return;
    dragging = false;
    if (typeof stage.hasPointerCapture === "function" && stage.hasPointerCapture(event.pointerId)) {
      stage.releasePointerCapture(event.pointerId);
    }
  };
  stage.addEventListener("pointerup", endDrag);
  stage.addEventListener("pointercancel", endDrag);
  stage.addEventListener("lostpointercapture", () => {
    dragging = false;
  });

  stage.addEventListener(
    "wheel",
    (event) => {
      if (!state.image) return;
      event.preventDefault();
      const bounds = crop.getBoundingClientRect();
      const anchorX = clamp(event.clientX - bounds.left, 0, bounds.width);
      const anchorY = clamp(event.clientY - bounds.top, 0, bounds.height);
      const factor = Math.exp(-event.deltaY * 0.0015);
      zoomAt(state.zoom * factor, anchorX, anchorY);
    },
    { passive: false }
  );

  stage.addEventListener("keydown", (event) => {
    if (!state.image) return;
    const step = event.shiftKey ? 36 : 12;
    const middle = (crop.clientWidth || 1) / 2;
    let handled = true;
    switch (event.key) {
      case "ArrowLeft":
        panBy(-step, 0);
        break;
      case "ArrowRight":
        panBy(step, 0);
        break;
      case "ArrowUp":
        panBy(0, -step);
        break;
      case "ArrowDown":
        panBy(0, step);
        break;
      case "+":
      case "=":
        zoomAt(state.zoom + 0.1, middle, middle);
        break;
      case "-":
      case "_":
        zoomAt(state.zoom - 0.1, middle, middle);
        break;
      case "0":
        resetView();
        break;
      default:
        handled = false;
    }
    if (handled) event.preventDefault();
  });

  zoom.addEventListener("input", () => {
    const next = Number.parseFloat(zoom.value);
    setZoom(Number.isFinite(next) ? next : ZOOM_MIN);
  });

  if (resetButton) resetButton.addEventListener("click", resetView);
  if (cancelButton) cancelButton.addEventListener("click", () => discard(true));

  if (typeof ResizeObserver === "function") {
    new ResizeObserver(() => {
      if (state.image && !editor.hidden) render();
    }).observe(crop);
  } else {
    window.addEventListener("resize", () => {
      if (state.image && !editor.hidden) render();
    });
  }

  // ---- Submit: replace the original with the chosen square -----------------
  form.addEventListener("submit", (event) => {
    if (!state.image || !state.exportable || state.prepared || exporting) return;
    event.preventDefault();
    exporting = true;
    if (submitButton) submitButton.disabled = true;
    form.setAttribute("aria-busy", "true");
    exportCrop()
      .then((file) => {
        if (!assignFile(file)) throw new Error("cannot stage the crop");
        state.prepared = true;
        exporting = false;
        if (submitButton) submitButton.disabled = false;
        form.setAttribute("aria-busy", "false");
        if (typeof form.requestSubmit === "function") {
          if (submitButton) form.requestSubmit(submitButton);
          else form.requestSubmit();
        } else {
          form.submit();
        }
      })
      .catch(() => {
        exporting = false;
        if (submitButton) submitButton.disabled = false;
        form.setAttribute("aria-busy", "false");
        showError("Foto gagal disiapkan. Coba lagi.");
      });
  });
}
