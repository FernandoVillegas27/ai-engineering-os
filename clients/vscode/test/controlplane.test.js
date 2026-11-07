"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const http = require("node:http");

const {
  normalizeAddress,
  isLoopbackHost,
  dashboardURL,
  requestJSON,
  listRuns
} = require("../lib/controlplane");

test("loopback validation accepts localhost and 127/8 only", () => {
  assert.equal(isLoopbackHost("localhost"), true);
  assert.equal(isLoopbackHost("127.0.0.1"), true);
  assert.equal(isLoopbackHost("127.10.20.30"), true);
  assert.equal(isLoopbackHost("::1"), true);
  assert.equal(isLoopbackHost("0.0.0.0"), false);
  assert.equal(isLoopbackHost("192.168.1.20"), false);
  assert.equal(isLoopbackHost("example.com"), false);
});

test("normalizeAddress requires explicit loopback port", () => {
  assert.equal(normalizeAddress("127.0.0.1:8765").host, "127.0.0.1:8765");
  assert.throws(() => normalizeAddress("http://example.com:8765"), /loopback/);
  assert.throws(() => normalizeAddress("http://127.0.0.1"), /explicit port/);
  assert.throws(() => normalizeAddress("file:///tmp/x"), /http/);
});

test("dashboardURL scopes selected run in query", () => {
  assert.equal(
    dashboardURL("127.0.0.1:8765", "run / 1"),
    "http://127.0.0.1:8765/ui/?run=run+%2F+1"
  );
});

test("requestJSON and listRuns use bounded read-only HTTP", async (t) => {
  const server = http.createServer((request, response) => {
    response.setHeader("Content-Type", "application/json");
    if (request.url === "/runs") {
      response.end(JSON.stringify([{run_id: "run-1", status: "completed"}]));
      return;
    }
    response.statusCode = 404;
    response.end(JSON.stringify({error: "not found"}));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(() => new Promise((resolve) => server.close(resolve)));

  const address = "127.0.0.1:" + server.address().port;
  const runs = await listRuns(address, 1000);
  assert.equal(runs.length, 1);
  assert.equal(runs[0].run_id, "run-1");
  await assert.rejects(() => requestJSON(address, "/missing", 1000), /HTTP 404/);
});

test("requestJSON enforces timeout", async (t) => {
  const server = http.createServer(() => {});
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(() => new Promise((resolve) => server.close(resolve)));

  const address = "127.0.0.1:" + server.address().port;
  await assert.rejects(() => requestJSON(address, "/health", 50), /timed out/);
});
