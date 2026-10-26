// Twisty Quiz focus-loss client (ATT-156..ATT-160).
//
// Every attempt surface reports the moments it leaves the screen: a tab or
// window switch on desktop, the app switcher, a screen lock or another app on
// mobile. One absence is one POST of the kind that described it to
// POST /attempt/:id/beacon, which appends one focus_loss_events row — the
// source of the monitor's "Fokus hilang" tally and the result page's focus
// sentence.
//
// The transport is navigator.sendBeacon: that route is the one POST without a
// CSRF token precisely because a beacon cannot set one, and it checks
// same-origin instead (ATT-159). app.js stays network-free, so this lives in
// its own page-scoped module, loaded through partials/page-script exactly the
// way monitor.js is.
//
// The count is informational only (ATT-039): it never changes an answer, a
// score, an attempt state, or submission eligibility, and the server drops
// whatever exceeds the shared 10-per-minute focus budget (ATT-158).
export {};

const root = document.querySelector("[data-beacon-href]");
const beaconHref = root ? root.dataset.beaconHref : "";
if (beaconHref && typeof navigator.sendBeacon === "function") run(beaconHref);

function run(beaconHref) {
  // One absence reports several signals — desktop fires blur and then
  // visibilitychange, a mobile backgrounding fires visibilitychange and then
  // pagehide — and the first of them speaks for the whole event. Sending the
  // pair would count a single switch twice and spend two of the ten
  // per-minute slots on it (ATT-158, ATT-306).
  const COALESCE_MS = 1500;

  // Leaving through the page's own controls also hides the document on the way
  // out — the Next button, a navigator link, Kirim jawaban — but nothing left
  // the screen: the student moved inside the attempt. The window is wide
  // enough to swallow the unload that follows the click. The autosave POST is
  // excluded because it targets a hidden iframe and never leaves the page.
  const NAVIGATE_MS = 2000;

  let sentAt = 0;
  let navigatedAt = 0;

  const absence = (kind) => {
    const now = Date.now();
    if (now - navigatedAt < NAVIGATE_MS) return;
    if (now - sentAt < COALESCE_MS) return;
    sentAt = now;
    navigator.sendBeacon(beaconHref, kind);
  };

  // Navigation intent is a click on a link or a submit that is not the quiet
  // autosave one; both listeners sit in the capture phase so they see the
  // event before anything cancels it.
  document.addEventListener(
    "click",
    (event) => {
      const link = event.target instanceof Element ? event.target.closest("a[href]") : null;
      if (link) navigatedAt = Date.now();
    },
    true
  );
  document.addEventListener(
    "submit",
    (event) => {
      const submitter = event.submitter;
      if (submitter && submitter.name === "nav" && submitter.value === "autosave") return;
      navigatedAt = Date.now();
    },
    true
  );

  // hidden is the one signal every platform agrees on: it fires for a tab
  // switch, an OS window switch, and the mobile app switcher or screen lock —
  // the cases a desktop-only blur listener would miss. Coming back is not an
  // event.
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) absence("hidden");
  });

  // blur is the desktop case visibilitychange does not cover: the window
  // loses focus while the page stays on screen (another window in front, a
  // second monitor, devtools). Mobile browsers fire it unreliably, which is
  // why it is never the only signal.
  window.addEventListener("blur", () => absence("blur"));

  // pagehide catches a backgrounding on engines that report it without a
  // visible transition. It also fires while leaving, where the navigation
  // guard above already answers for it.
  window.addEventListener("pagehide", () => absence("hidden"));
}
