import assert from "node:assert/strict";
import { test } from "node:test";
import {
  daemonUrl,
  forget,
  hostLabel,
  normalizeAddress,
  pairingFrom,
  parseScan,
  permissionState,
  RECENT_LIMIT,
  readRecent,
  remember,
} from "./address.js";

test("a typed address gets the default port, lower case, and no scheme or path", () => {
  assert.deepEqual(normalizeAddress(" HTTP://Marshal-Laptop.tail1.ts.net/some/path?x=1 "), {
    ok: true,
    address: "marshal-laptop.tail1.ts.net:47800",
  });
  assert.deepEqual(normalizeAddress("100.64.0.7:5000"), { ok: true, address: "100.64.0.7:5000" });
});

test("an address that cannot be one is refused with a sentence", () => {
  for (const bad of ["", "   ", "a b", "host:99999", "host:abc", "a:1:2", "-bad-"]) {
    const answer = normalizeAddress(bad);
    assert.equal(answer.ok, false, bad);
    assert.ok(answer.reason.length > 0);
  }
});

test("the QR code carries the address and the code, and nothing else is accepted", () => {
  assert.deepEqual(parseScan("marshal://pair?host=laptop.tail1.ts.net&code=7QX-2LD"), {
    address: "laptop.tail1.ts.net:47800",
    code: "7QX-2LD",
  });
  for (const other of [
    "https://example.com",
    "marshal://card/abc",
    "marshal://pair?host=x",
    "marshal://pair?code=1",
    "not a url",
    null,
  ]) {
    assert.equal(parseScan(other), null, String(other));
  }
});

test("the daemon's page is opened with the code only when there is one", () => {
  assert.equal(daemonUrl("a:47800"), "http://a:47800/");
  assert.equal(daemonUrl("a:47800", "7QX-2LD"), "http://a:47800/?pair=7QX-2LD");
});

test("a camera permission answer is read from its camera field, as the plugin sends it", () => {
  assert.equal(permissionState({ camera: "granted" }), "granted");
  assert.equal(permissionState({ camera: "prompt" }), "prompt");
  assert.equal(permissionState({ camera: "denied" }), "denied");
  assert.equal(permissionState("granted"), "granted");
  for (const nothing of [null, undefined, {}, []])
    assert.equal(permissionState(nothing), "", String(nothing));
  // The bug this guards: the whole answer is an object, and an object is never the word "granted".
  assert.notEqual({ camera: "granted" }, "granted");
});

test("the first Marshal pairing link in what the phone handed over is the one used", () => {
  assert.deepEqual(
    pairingFrom([
      "https://example.com",
      "marshal://pair?host=a.tail1.ts.net&code=7QX-2LD",
      "marshal://pair?host=b&code=2",
    ]),
    { address: "a.tail1.ts.net:47800", code: "7QX-2LD" },
  );
  for (const none of [[], null, undefined, "marshal://pair?host=a&code=1", ["marshal://card/1"]]) {
    assert.equal(pairingFrom(none), null, String(none));
  }
});

test("a saved list of recent computers is read newest first, without bad or repeated entries", () => {
  const saved = JSON.stringify([
    { address: "old.tail1.ts.net", lastUsed: 1 },
    { address: "NEW.tail1.ts.net:47800", lastUsed: 30 },
    { address: "new.tail1.ts.net", lastUsed: 5 },
    { address: "not an address", lastUsed: 99 },
    { address: "mid.tail1.ts.net:5000", lastUsed: "20" },
    { address: "x.tail1.ts.net", lastUsed: "later" },
    null,
  ]);
  assert.deepEqual(readRecent(saved), [
    { address: "new.tail1.ts.net:47800", lastUsed: 30 },
    { address: "mid.tail1.ts.net:5000", lastUsed: 20 },
    { address: "old.tail1.ts.net:47800", lastUsed: 1 },
  ]);
  for (const damaged of [null, undefined, "", "not json", "{}", "42", "[1, 2]"]) {
    assert.deepEqual(readRecent(damaged), [], String(damaged));
  }
});

test("a computer just used goes first, once, and only the last few are kept", () => {
  const LATER = 1000;
  let list = [];
  for (let n = 1; n <= RECENT_LIMIT + 2; n += 1) list = remember(list, `c${n}.ts.net:47800`, n);
  assert.equal(list.length, RECENT_LIMIT);
  assert.equal(list[0].address, `c${RECENT_LIMIT + 2}.ts.net:47800`);
  list = remember(list, "c4.ts.net:47800", LATER);
  assert.equal(list[0].address, "c4.ts.net:47800");
  assert.equal(list.filter((entry) => entry.address === "c4.ts.net:47800").length, 1);
  assert.equal(list.length, RECENT_LIMIT);
});

test("forgetting a computer removes only that one", () => {
  const list = remember(remember([], "a.ts.net:47800", 1), "b.ts.net:47800", 2);
  assert.deepEqual(forget(list, "a.ts.net:47800"), [{ address: "b.ts.net:47800", lastUsed: 2 }]);
  assert.deepEqual(forget(list, "z.ts.net:47800"), list);
});

test("a computer is named by the first part of its name, or by its address when it has none", () => {
  assert.equal(
    hostLabel("kolaborates-macbook-air.tail44e33d.ts.net:47800"),
    "kolaborates-macbook-air",
  );
  assert.equal(hostLabel("100.78.221.84:47800"), "100.78.221.84");
  assert.equal(hostLabel("laptop:5000"), "laptop");
});
