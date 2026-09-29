import assert from "node:assert/strict";
import { test } from "node:test";
import { daemonUrl, normalizeAddress, parseScan } from "./address.js";

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
