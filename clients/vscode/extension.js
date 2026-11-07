"use strict";

const vscode = require("vscode");
const {
  DEFAULT_ADDRESS,
  normalizeAddress,
  dashboardURL,
  health,
  listRuns
} = require("./lib/controlplane");

let statusItem;

function configuration() {
  const config = vscode.workspace.getConfiguration("aiEngineeringOS");
  return {
    binaryPath: String(config.get("binaryPath", "ai")).trim() || "ai",
    address: String(config.get("controlPlaneAddress", DEFAULT_ADDRESS)).trim() || DEFAULT_ADDRESS,
    timeoutMs: Number(config.get("requestTimeoutMs", 3000)) || 3000
  };
}

async function refreshStatus(showErrors = false) {
  if (!statusItem) return;
  const config = configuration();
  try {
    normalizeAddress(config.address);
    await health(config.address, config.timeoutMs);
    statusItem.text = "$(pulse) AI OS: online";
    statusItem.tooltip = "AI Engineering OS Control Plane is reachable";
    statusItem.command = "aiEngineeringOS.showRuns";
  } catch (error) {
    statusItem.text = "$(circle-slash) AI OS: offline";
    statusItem.tooltip = String(error && error.message || error);
    statusItem.command = "aiEngineeringOS.startControlPlane";
    if (showErrors) vscode.window.showWarningMessage("AI Engineering OS: " + statusItem.tooltip);
  }
}

async function openControlPlane(runId = "") {
  const config = configuration();
  let url;
  try {
    url = dashboardURL(config.address, runId);
  } catch (error) {
    vscode.window.showErrorMessage("AI Engineering OS: " + error.message);
    return;
  }
  await vscode.env.openExternal(vscode.Uri.parse(url));
}

async function startControlPlane() {
  const config = configuration();
  let normalized;
  try {
    normalized = normalizeAddress(config.address);
  } catch (error) {
    vscode.window.showErrorMessage("AI Engineering OS: " + error.message);
    return;
  }

  const workspace = vscode.workspace.workspaceFolders && vscode.workspace.workspaceFolders[0];
  const terminal = vscode.window.createTerminal({
    name: "AI Engineering OS",
    shellPath: config.binaryPath,
    shellArgs: ["run", "serve", "--addr", normalized.host],
    cwd: workspace ? workspace.uri.fsPath : undefined,
    isTransient: false
  });
  terminal.show(true);
  vscode.window.showInformationMessage("AI Engineering OS Control Plane starting on " + normalized.host);
  setTimeout(() => refreshStatus(false), 1200);
}

async function showRuns() {
  const config = configuration();
  let runs;
  try {
    runs = await listRuns(config.address, config.timeoutMs);
  } catch (error) {
    const action = await vscode.window.showWarningMessage(
      "AI Engineering OS Control Plane is unavailable: " + error.message,
      "Start Control Plane"
    );
    if (action === "Start Control Plane") await startControlPlane();
    return;
  }

  if (!runs.length) {
    const action = await vscode.window.showInformationMessage(
      "AI Engineering OS has no durable runs yet.",
      "Open Dashboard"
    );
    if (action === "Open Dashboard") await openControlPlane();
    return;
  }

  const items = runs.map((run) => ({
    label: "$(run) " + (run.run_id || "run"),
    description: String(run.status || "unknown"),
    detail: progressDetail(run),
    runId: run.run_id
  }));

  const selected = await vscode.window.showQuickPick(items, {
    placeHolder: "Select a durable AI Engineering OS run",
    matchOnDescription: true,
    matchOnDetail: true
  });
  if (selected) await openControlPlane(selected.runId);
}

function progressDetail(run) {
  const progress = run && run.progress || {};
  const completed = Number(progress.completed || 0);
  const total = Number(progress.total || 0);
  const sequence = Number(run && run.last_sequence || 0);
  return completed + "/" + total + " tasks · event #" + sequence;
}

function activate(context) {
  statusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 50);
  statusItem.text = "$(sync~spin) AI OS: checking";
  statusItem.command = "aiEngineeringOS.showRuns";
  statusItem.show();

  context.subscriptions.push(
    statusItem,
    vscode.commands.registerCommand("aiEngineeringOS.startControlPlane", startControlPlane),
    vscode.commands.registerCommand("aiEngineeringOS.openControlPlane", () => openControlPlane()),
    vscode.commands.registerCommand("aiEngineeringOS.showRuns", showRuns),
    vscode.commands.registerCommand("aiEngineeringOS.refreshStatus", () => refreshStatus(true)),
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration("aiEngineeringOS")) refreshStatus(false);
    })
  );

  refreshStatus(false);
}

function deactivate() {
  statusItem = undefined;
}

module.exports = { activate, deactivate, progressDetail };
