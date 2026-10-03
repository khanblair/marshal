import {
  daemonUrl,
  forget,
  hostLabel,
  normalizeAddress,
  pairingFrom,
  parseScan,
  permissionState,
  readRecent,
  remember,
} from "./address.js";

const RECENT_KEY = "marshal-recent";
/** The one address the first screen used to remember, read once and then dropped. */
const LEGACY_KEY = "marshal-address";
const REACH_MS = 6000;
const INTRO =
  "Marshal runs on your computer. This app is its remote control, so your agents keep working while you are away.";
const $ = (id) => document.getElementById(id);
const tauri = window.__TAURI_INTERNALS__;
const invoke = (command, args) => tauri.invoke(command, args);

/** The recent computers, newest first. Read once at start, then kept in step with what is saved. */
let recent = loadRecent();
/** The address the last attempt was for, so Try again tries that one. */
let attempt = "";

function loadRecent() {
  let list = readRecent(localStorage.getItem(RECENT_KEY));
  const legacy = normalizeAddress(localStorage.getItem(LEGACY_KEY));
  if (legacy.ok) list = remember(list, legacy.address, Date.now());
  localStorage.removeItem(LEGACY_KEY);
  localStorage.setItem(RECENT_KEY, JSON.stringify(list));
  return list;
}

function saveRecent(list) {
  recent = list;
  localStorage.setItem(RECENT_KEY, JSON.stringify(list));
  renderRecent();
}

/** Draws the recent computers as buttons, using text only, so an address is never read as markup. */
function renderRecent() {
  const items = recent.map((entry) => {
    const row = document.createElement("li");
    row.className = "recent-row";
    const open = document.createElement("button");
    open.type = "button";
    open.className = "recent-open";
    open.dataset.address = entry.address;
    const name = document.createElement("span");
    name.className = "recent-name";
    name.textContent = hostLabel(entry.address);
    const address = document.createElement("span");
    address.className = "recent-address";
    address.textContent = entry.address;
    open.append(name, address);
    const drop = document.createElement("button");
    drop.type = "button";
    drop.className = "recent-forget";
    drop.dataset.address = entry.address;
    drop.textContent = "Forget";
    drop.setAttribute("aria-label", `Forget ${hostLabel(entry.address)}`);
    row.append(open, drop);
    return row;
  });
  $("recent-list").replaceChildren(...items);
  $("recent").hidden = items.length === 0;
}

function show(part) {
  $("form").hidden = part !== "form";
  $("retry").hidden = part !== "retry";
}

function say(text, isError = false) {
  const target = isError ? $("error") : $("status");
  target.textContent = text;
  if (isError) target.hidden = false;
}

/** True when something answers at the address, whatever it says: the answer itself is not readable from here. */
async function reachable(address) {
  const stop = new AbortController();
  const timer = setTimeout(() => stop.abort(), REACH_MS);
  try {
    await fetch(`http://${address}/v1/health`, {
      mode: "no-cors",
      signal: stop.signal,
      cache: "no-store",
    });
    return true;
  } catch {
    return false;
  } finally {
    clearTimeout(timer);
  }
}

async function open(address, code) {
  attempt = address;
  show("none");
  say(`Connecting to ${address}...`);
  if (await reachable(address)) {
    saveRecent(remember(recent, address, Date.now()));
    window.location.href = daemonUrl(address, code);
    return;
  }
  say(
    `Marshal could not reach ${address}. Check that Tailscale is on for this phone. Then, on your computer, open Settings, then Remote control: it shows the address that works and what is missing.`,
  );
  show("retry");
}

/** Asks for the camera if it has not been allowed. False means it will not be. */
async function cameraAllowed() {
  if (permissionState(await invoke("plugin:barcode-scanner|check_permissions")) === "granted")
    return true;
  return permissionState(await invoke("plugin:barcode-scanner|request_permissions")) === "granted";
}

async function scan() {
  try {
    if (!(await cameraAllowed())) {
      say(
        "Marshal needs the camera to scan the code. Allow it in the phone's settings, or type the address instead.",
      );
      return;
    }
    const scanned = await invoke("plugin:barcode-scanner|scan", { formats: ["QR_CODE"] });
    const pairing = parseScan(scanned?.content);
    if (pairing) await open(pairing.address, pairing.code);
    else
      say(
        "That is not a Marshal code. Scan the one shown in Settings, Remote control, Pair a device.",
      );
  } catch {
    say("The camera could not read a code. You can type the address instead.");
  }
}

/** The pairing link the app was opened with, when the phone's own camera or a message opened it. */
async function openedWithLink() {
  try {
    return pairingFrom(await invoke("plugin:deep-link|get_current"));
  } catch {
    return null;
  }
}

/** A pairing link opened while the app is already showing this screen. */
function listenForLinks() {
  try {
    void invoke("plugin:event|listen", {
      event: "deep-link://new-url",
      target: { kind: "Any" },
      handler: tauri.transformCallback((event) => {
        const pairing = pairingFrom(event?.payload);
        if (pairing) void open(pairing.address, pairing.code);
      }),
    });
  } catch {
    // Without the event bridge the link is still read when the app starts.
  }
}

$("form").addEventListener("submit", (event) => {
  event.preventDefault();
  const answer = normalizeAddress($("address").value);
  if (answer.ok) {
    $("error").hidden = true;
    $("address").removeAttribute("aria-invalid");
    void open(answer.address, $("code").value.trim() || undefined);
  } else {
    $("address").setAttribute("aria-invalid", "true");
    say(answer.reason, true);
  }
});
$("scan").addEventListener("click", () => void scan());
$("again").addEventListener("click", () => void open(attempt));
$("recent-list").addEventListener("click", (event) => {
  const button = event.target.closest("button");
  const address = button?.dataset.address;
  if (!address) return;
  if (button.classList.contains("recent-forget")) saveRecent(forget(recent, address));
  else void open(address);
});
$("change").addEventListener("click", () => {
  say(INTRO);
  show("form");
});

// A page opened in a browser has no camera, so it offers only the typed address.
if (!tauri) $("alt").hidden = true;
else listenForLinks();

renderRecent();

// Coming back to this screen with the phone's Back, from a computer's page, is a way to choose another
// computer, so it shows the list and does not connect again by itself.
const cameBack = performance.getEntriesByType("navigation")[0]?.type === "back_forward";
window.addEventListener("pageshow", (event) => {
  if (!event.persisted) return;
  say(INTRO);
  show("form");
});

const link = tauri ? await openedWithLink() : null;
if (link) void open(link.address, link.code);
else if (recent[0] && !cameBack) void open(recent[0].address);
else show("form");
