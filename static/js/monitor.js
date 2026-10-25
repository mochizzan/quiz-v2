// Twisty Quiz live monitor client (ATT-191…ATT-198). The SSE stream is a change
// signal, not the source of truth: the payload carries only the ATT-194 subset,
// so the server-rendered Skor and Mulai (WIB) columns stay authoritative and the
// page reloads when a row gains an attempt, changes state, or a late grade
// changes its score. Everything is guarded, so the file is inert on pages that
// do not carry [data-monitor].
//
// This is the one page-scoped module allowed to open a network connection;
// app.js stays network-free on purpose.
export {};

const root = document.querySelector("[data-monitor]");
const streamHref = root ? root.dataset.monitorStream : "";
if (root && streamHref) run(root, streamHref);

function run(root, streamHref) {
  const rows = root.querySelector("[data-monitor-rows]");
  const stale = root.querySelector("[data-monitor-stale]");
  const reloadButton = root.querySelector("[data-monitor-reload]");
  const clockFormat = new Intl.DateTimeFormat("en-GB", {
    timeZone: "Asia/Jakarta",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  });
  const stateBadges = {
    registered: ["Terdaftar", "badge theme-info"],
    in_progress: ["Sedang berlangsung", "badge theme-primary"],
    completed: ["Selesai", "badge theme-success"],
  };
  const flashTimers = new WeakMap();

  let source = null;
  let connections = 0;
  let sawSnapshot = false;
  let stopped = false;
  let reloadTimer = 0;

  const showStale = (on) => {
    if (stale) stale.hidden = !on;
  };

  // A reload is the only truthful way to refresh the server-rendered columns
  // and the ATT-183 row order. Closing first frees the subscriber slot, and the
  // small delay coalesces a burst of events into one navigation.
  const scheduleReload = () => {
    if (reloadTimer) return;
    stopped = true;
    if (source) {
      source.close();
      source = null;
    }
    reloadTimer = window.setTimeout(() => window.location.reload(), 250);
  };

  const findRow = (registrationID) => {
    if (!rows) return null;
    return rows.querySelector(`[data-registration-id="${registrationID}"]`);
  };

  const setMetric = (name, value) => {
    const el = root.querySelector(`[data-monitor-metric="${name}"]`);
    if (el) el.textContent = String(value);
  };

  const percentAnswered = (progress) => {
    const answered = Number(progress.answered) || 0;
    const total = Number(progress.total) || 0;
    if (total <= 0) return 0;
    const percent = Math.floor((answered * 100) / total);
    return Math.max(0, Math.min(100, percent));
  };

  const setAnswered = (answered, total) => {
    setMetric("answered", `${answered} / ${total}`);
    const percent = percentAnswered({ answered, total });
    setMetric("percent", `${percent}% dari seluruh jawaban`);
    const ring = root.querySelector("[data-monitor-progress] .progress-ring");
    const fill = root.querySelector("[data-monitor-progress] .progress-fill");
    if (ring) ring.setAttribute("aria-valuenow", String(percent));
    if (fill) fill.style.width = `${percent}%`;
  };

  const updateProgress = (progress) => {
    if (!progress) return;
    setMetric("registered", progress.registered);
    setMetric("in-progress", progress.in_progress);
    setMetric("completed", progress.completed);
    setAnswered(progress.answered, progress.total);
  };

  // Update events carry one participant, not the aggregate. The delta keeps
  // "Jawaban terkumpul" truthful between reloads, without patching the page
  // for every answer.
  const readAnswered = () => {
    const el = root.querySelector('[data-monitor-metric="answered"]');
    if (!el) return null;
    const [answered, total] = el.textContent
      .split("/")
      .map((part) => Number(part.trim()));
    if (!Number.isFinite(answered) || !Number.isFinite(total) || total <= 0) {
      return null;
    }
    return { answered, total };
  };

  const adjustAnswered = (row, answered) => {
    const prev = Number(row.dataset.answered);
    if (!Number.isFinite(prev) || prev === answered) return;
    const progress = readAnswered();
    if (!progress) return;
    const next = Math.max(
      0,
      Math.min(progress.total, progress.answered + answered - prev)
    );
    setAnswered(next, progress.total);
  };

  // Clock() on the server renders "07.00" (WIB, dot separator).
  const clock = (iso) => {
    if (!iso) return "—";
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return "—";
    return clockFormat.format(date).replace(":", ".");
  };

  const renderFocus = (cell, count) => {
    if (!cell) return;
    cell.replaceChildren();
    if (count > 0) {
      const badge = document.createElement("span");
      badge.className = "badge theme-danger";
      const num = document.createElement("span");
      num.className = "num";
      num.textContent = String(count);
      badge.append(num, " fokus hilang");
      cell.append(badge);
      return;
    }
    const zero = document.createElement("span");
    zero.className = "num";
    zero.textContent = "0";
    cell.append(zero);
  };

  const renderAction = (cell, attemptID) => {
    if (!cell) return;
    cell.replaceChildren();
    if (attemptID === null || attemptID === undefined) {
      const label = document.createElement("span");
      label.className = "cell-sub";
      label.textContent = "Belum mulai";
      cell.append(label);
      return;
    }
    const link = document.createElement("a");
    link.className = "link-quiet";
    link.setAttribute("href", `/teacher/recap/${encodeURIComponent(String(attemptID))}`);
    link.textContent = "Rekap";
    cell.append(link);
  };

  const flash = (row) => {
    window.clearTimeout(flashTimers.get(row));
    row.classList.remove("monitor-row-live");
    // Restart the animation when two updates arrive back to back.
    void row.offsetWidth;
    row.classList.add("monitor-row-live");
    flashTimers.set(
      row,
      window.setTimeout(() => row.classList.remove("monitor-row-live"), 1200)
    );
  };

  const patchRow = (row, participant, fromSnapshot) => {
    // The snapshot's aggregate is already exact and has been rendered by
    // updateProgress; only an incremental update needs the delta.
    if (!fromSnapshot) adjustAnswered(row, participant.answered);
    const stateCell = row.querySelector('[data-cell="state"]');
    const badge = stateCell ? stateCell.querySelector(".badge") : null;
    if (badge) {
      const [label, className] =
        stateBadges[participant.state] || ["—", "badge bg-2 fg-secondary"];
      badge.className = className;
      badge.textContent = label;
    }
    const progressCell = row.querySelector('[data-cell="progress"]');
    if (progressCell) {
      progressCell.textContent = `${participant.answered} / ${participant.total}`;
    }
    renderFocus(row.querySelector('[data-cell="focus"]'), participant.focus_loss);
    const updatedCell = row.querySelector('[data-cell="updated"]');
    if (updatedCell) updatedCell.textContent = clock(participant.last_seen);
    renderAction(row.querySelector('[data-cell="action"]'), participant.attempt_id);
    row.dataset.state = participant.state;
    row.dataset.attemptId =
      participant.attempt_id === null || participant.attempt_id === undefined
        ? ""
        : String(participant.attempt_id);
    row.dataset.answered = String(participant.answered);
    row.dataset.total = String(participant.total);
    flash(row);
  };

  // The live payload cannot describe Skor or Mulai, and a state change also
  // moves the row in ATT-183 order. Those cases need the server's render.
  // A snapshot may legitimately repeat a completed row, so only an update event
  // on a completed row forces the reload (it is the late-grade signal).
  const needsServer = (row, participant, fromSnapshot) => {
    const hadAttempt = (row.dataset.attemptId || "") !== "";
    const hasAttempt =
      participant.attempt_id !== null && participant.attempt_id !== undefined;
    if (hadAttempt !== hasAttempt) return true;
    const state = row.dataset.state || "";
    if (state !== participant.state) return true;
    return !fromSnapshot && state === "completed";
  };

  const applyParticipant = (participant, fromSnapshot) => {
    const row = findRow(participant.registration_id);
    if (!row) return false;
    if (needsServer(row, participant, fromSnapshot)) return false;
    patchRow(row, participant, fromSnapshot);
    return true;
  };

  const applySnapshot = (snapshot) => {
    updateProgress(snapshot.progress);
    const participants = Array.isArray(snapshot.participants)
      ? snapshot.participants
      : [];
    for (const participant of participants) {
      if (!applyParticipant(participant, true)) {
        scheduleReload();
        return;
      }
    }
    if (rows && participants.length !== rows.children.length) scheduleReload();
  };

  const parse = (event) => {
    try {
      return JSON.parse(event.data);
    } catch {
      return null;
    }
  };

  source = new EventSource(streamHref);

  source.addEventListener("open", () => {
    connections += 1;
    showStale(false);
  });

  source.addEventListener("snapshot", (event) => {
    const snapshot = parse(event);
    if (!snapshot) return;
    showStale(false);
    // The first connection's snapshot only closes the gap between the server
    // render and the stream start. A reconnect (ATT-196) or a mid-stream
    // snapshot is the recovery path, so reload for the server's render.
    if (connections > 1 || sawSnapshot) {
      scheduleReload();
      return;
    }
    sawSnapshot = true;
    applySnapshot(snapshot);
  });

  source.addEventListener("participant.added", () => {
    showStale(false);
    scheduleReload();
  });

  source.addEventListener("participant.updated", (event) => {
    const participant = parse(event);
    if (!participant) return;
    showStale(false);
    if (!applyParticipant(participant, false)) scheduleReload();
  });

  source.addEventListener("heartbeat", () => showStale(false));

  source.addEventListener("quiz.closed", () => scheduleReload());

  source.addEventListener("error", () => {
    if (!stopped) showStale(true);
  });

  if (reloadButton) {
    reloadButton.addEventListener("click", () => window.location.reload());
  }
}
