import { daemonUrl, normalizeAddress, pairingFrom, parseScan, permissionState } from "./address.js";

const KEY = "marshal-address";
const REACH_MS = 6000;
const INTRO =
  "Marshal runs on your computer. This app is its remote control, so your agents keep working while you are away.";
const $ = (id) => document.getElementById(id);
const tauri = window.__TAURI_INTERNALS__;
const invoke = (command, args) => tauri.invoke(command, args);

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
  show("none");
  say(`Connecting to ${address}...`);
  if (await reachable(address)) {
    localStorage.setItem(KEY, address);
    window.location.href = daemonUrl(address, code);
    return;
  }
  say(
    `Marshal could not reach ${address}. Check that Tailscale is on for this phone, and that Marshal is running on your computer.`,
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
    else say("That is not a Marshal code. Scan the one shown in Settings, Profile, Pair a device.");
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
    void open(answer.address);
  } else {
    $("address").setAttribute("aria-invalid", "true");
    say(answer.reason, true);
  }
});
$("scan").addEventListener("click", () => void scan());
$("again").addEventListener("click", () => void open(localStorage.getItem(KEY) ?? ""));
$("change").addEventListener("click", () => {
  say(INTRO);
  show("form");
});

// A page opened in a browser has no camera, so it offers only the typed address.
if (!tauri) $("alt").hidden = true;
else listenForLinks();

const link = tauri ? await openedWithLink() : null;
const known = localStorage.getItem(KEY);
if (link) void open(link.address, link.code);
else if (known) void open(known);
else show("form");
