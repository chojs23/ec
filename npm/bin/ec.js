#!/usr/bin/env node
// Thin shim: spawn the native ec binary that postinstall placed in
// `vendor/`. Kept in JS (not a symlink to the binary) so npm can
// generate proper shims on Windows as well as Unix.

"use strict";

const path = require("node:path");
const fs = require("node:fs");
const { spawn } = require("node:child_process");

const binaryName = process.platform === "win32" ? "ec.exe" : "ec";
const binaryPath = path.join(__dirname, "..", "vendor", binaryName);

if (!fs.existsSync(binaryPath)) {
  process.stderr.write(
    `ec binary not found at ${binaryPath}.\n` +
      "The postinstall step may have been skipped (e.g. --ignore-scripts).\n" +
      "Reinstall with scripts enabled, or run `node install.js` inside the package.\n"
  );
  process.exit(1);
}

const child = spawn(binaryPath, process.argv.slice(2), { stdio: "inherit" });
child.on("exit", (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exit(code ?? 1);
});
child.on("error", (err) => {
  process.stderr.write(`failed to launch ec: ${err.message}\n`);
  process.exit(1);
});
