"use strict";

const http = require("node:http");
const https = require("node:https");
const net = require("node:net");

const DEFAULT_ADDRESS = "127.0.0.1:8765";
const MAX_RESPONSE_BYTES = 2 * 1024 * 1024;

function normalizeAddress(address) {
  const value = String(address || DEFAULT_ADDRESS).trim();
  if (!value) throw new Error("Control Plane address is required.");

  let url;
  try {
    url = new URL(value.includes("://") ? value : "http://" + value);
  } catch {
    throw new Error("Control Plane address must be host:port or an http(s) URL.");
  }

  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("Control Plane URL must use http or https.");
  }
  if (!isLoopbackHost(url.hostname)) {
    throw new Error("Control Plane address must use localhost or a loopback IP.");
  }
  if (!url.port) {
    throw new Error("Control Plane address must include an explicit port.");
  }
  url.pathname = "/";
  url.search = "";
  url.hash = "";
  return url;
}

function isLoopbackHost(hostname) {
  const host = String(hostname || "").replace(/^\[|\]$/g, "").toLowerCase();
  if (host === "localhost" || host === "::1") return true;
  const ipType = net.isIP(host);
  if (ipType === 4) {
    const first = Number(host.split(".")[0]);
    return first === 127;
  }
  return false;
}

function dashboardURL(address, runId = "") {
  const base = normalizeAddress(address);
  const url = new URL("/ui/", base);
  if (runId) url.searchParams.set("run", runId);
  return url.toString();
}

function requestJSON(address, path, timeoutMs = 3000) {
  const base = normalizeAddress(address);
  const target = new URL(path, base);
  const transport = target.protocol === "https:" ? https : http;

  return new Promise((resolve, reject) => {
    const request = transport.get(target, {
      headers: {
        Accept: "application/json",
        "User-Agent": "ai-engineering-os-vscode/0.1"
      }
    }, (response) => {
      const chunks = [];
      let size = 0;

      response.on("data", (chunk) => {
        size += chunk.length;
        if (size > MAX_RESPONSE_BYTES) {
          request.destroy(new Error("Control Plane response exceeded 2 MiB."));
          return;
        }
        chunks.push(chunk);
      });

      response.on("end", () => {
        const body = Buffer.concat(chunks).toString("utf8");
        if (response.statusCode < 200 || response.statusCode >= 300) {
          reject(new Error("Control Plane returned HTTP " + response.statusCode));
          return;
        }
        try {
          resolve(body ? JSON.parse(body) : null);
        } catch {
          reject(new Error("Control Plane returned invalid JSON."));
        }
      });
    });

    request.setTimeout(Math.max(250, Number(timeoutMs) || 3000), () => {
      request.destroy(new Error("Control Plane request timed out."));
    });
    request.on("error", reject);
  });
}

async function health(address, timeoutMs) {
  return requestJSON(address, "/health", timeoutMs);
}

async function listRuns(address, timeoutMs) {
  const value = await requestJSON(address, "/runs", timeoutMs);
  if (!Array.isArray(value)) throw new Error("Control Plane /runs response is not an array.");
  return value;
}

module.exports = {
  DEFAULT_ADDRESS,
  normalizeAddress,
  isLoopbackHost,
  dashboardURL,
  requestJSON,
  health,
  listRuns
};
