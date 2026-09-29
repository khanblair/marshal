import { daemonUrl, normalizeAddress, parseScan } from "./address.js";

const KEY = "marshal-address";
const REACH_MS = 6000;
const $ = (id) => document.getElementById(id);
const invoke = (command, args) => window.__TAURI_INTERNALS__.invoke(command, args);

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

async function scan() {
  try {
    if ((await invoke("plugin:barcode-scanner|check_permissions")) !== "granted") {
      if ((await invoke("plugin:barcode-scanner|request_permissions")) !== "granted") {
        say("Marshal needs the camera to scan the code. You can type the address instead.");
        return;
      }
    }
    const scanned = await invoke("plugin:barcode-scanner|scan", { formats: ["QR_CODE"] });
    const pairing = parseScan(scanned?.content);
    if (pairing) await open(pairing.address, pairing.code);
    else say("That is not a Marshal code. Scan the one shown in Settings, Profile, Pair a device.");
  } catch {
    say("The camera could not read a code. You can type the address instead.");
  }
}

$("form").addEventListener("submit", (event) => {
  event.preventDefault();
  const answer = normalizeAddress($("address").value);
  if (answer.ok) {
    $("error").hidden = true;
    void open(answer.address);
  } else {
    say(answer.reason, true);
  }
});
$("scan").addEventListener("click", () => void scan());
$("again").addEventListener("click", () => void open(localStorage.getItem(KEY) ?? ""));
$("change").addEventListener("click", () => {
  say("Control your agents from your phone. They keep running on your computer.");
  show("form");
});

const known = localStorage.getItem(KEY);
if (known) void open(known);
else show("form");
