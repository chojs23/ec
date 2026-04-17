#!/usr/bin/env node
// Downloads the prebuilt ec binary from GitHub Releases for the current
// platform/arch, verifies its SHA-256 against checksums.txt, and places
// it under `vendor/`. The release workflow ships plain binaries (not
// archives), so no extraction step is needed.

"use strict";

const fs = require("node:fs");
const path = require("node:path");
const https = require("node:https");
const crypto = require("node:crypto");
const os = require("node:os");

const pkg = require("./package.json");

const REPO = "chojs23/ec";
const VERSION = pkg.version;
const VENDOR_DIR = path.join(__dirname, "vendor");

function log(msg) {
  process.stderr.write(`[ec] ${msg}\n`);
}

function die(msg) {
  log(`error: ${msg}`);
  process.exit(1);
}

function detectTarget() {
  const platforms = { linux: "linux", darwin: "darwin", win32: "windows" };
  const arches = { x64: "amd64", arm64: "arm64" };
  const platform = platforms[process.platform];
  const arch = arches[process.arch];
  if (!platform) die(`unsupported OS: ${process.platform}`);
  if (!arch) die(`unsupported CPU arch: ${process.arch}`);
  const suffix = platform === "windows" ? ".exe" : "";
  return {
    platform,
    arch,
    suffix,
    assetName: `ec-${platform}-${arch}${suffix}`,
    localBinaryName: `ec${suffix}`,
  };
}

function assetUrl(target) {
  const base = `https://github.com/${REPO}/releases/download/v${VERSION}`;
  return {
    binary: `${base}/${target.assetName}`,
    checksums: `${base}/checksums.txt`,
  };
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const visit = (current, redirects) => {
      if (redirects > 10) return reject(new Error("too many redirects"));
      https
        .get(current, (res) => {
          if (
            res.statusCode &&
            res.statusCode >= 300 &&
            res.statusCode < 400 &&
            res.headers.location
          ) {
            res.resume();
            const next = new URL(res.headers.location, current).toString();
            return visit(next, redirects + 1);
          }
          if (res.statusCode !== 200) {
            res.resume();
            return reject(new Error(`GET ${current} -> ${res.statusCode}`));
          }
          const file = fs.createWriteStream(dest);
          res.pipe(file);
          file.on("finish", () => file.close(() => resolve()));
          file.on("error", reject);
        })
        .on("error", reject);
    };
    visit(url, 0);
  });
}

function sha256File(p) {
  return crypto.createHash("sha256").update(fs.readFileSync(p)).digest("hex");
}

function verifyChecksum(binaryPath, checksumsPath, assetName) {
  const lines = fs.readFileSync(checksumsPath, "utf8").split(/\r?\n/);
  const match = lines
    .map((l) => l.trim())
    .find((l) => l.endsWith(`  ${assetName}`) || l.endsWith(` ${assetName}`));
  if (!match) die(`checksum for ${assetName} not found in checksums.txt`);
  const expected = match.split(/\s+/)[0];
  const actual = sha256File(binaryPath);
  if (expected !== actual) {
    die(`checksum mismatch for ${assetName}: expected ${expected}, got ${actual}`);
  }
}

async function main() {
  if (process.env.EC_SKIP_DOWNLOAD === "1") {
    log("EC_SKIP_DOWNLOAD=1, skipping binary download");
    return;
  }

  const target = detectTarget();
  const { binary, checksums } = assetUrl(target);

  fs.mkdirSync(VENDOR_DIR, { recursive: true });
  const finalBinary = path.join(VENDOR_DIR, target.localBinaryName);
  if (fs.existsSync(finalBinary)) {
    log(`binary already present at ${finalBinary}, skipping download`);
    return;
  }

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "ec-"));
  const downloadedBinary = path.join(tmpDir, target.assetName);
  const checksumsPath = path.join(tmpDir, "checksums.txt");

  try {
    log(`downloading ${binary}`);
    await download(binary, downloadedBinary);
    log(`downloading ${checksums}`);
    await download(checksums, checksumsPath);
    verifyChecksum(downloadedBinary, checksumsPath, target.assetName);

    fs.copyFileSync(downloadedBinary, finalBinary);
    if (target.platform !== "windows") {
      fs.chmodSync(finalBinary, 0o755);
    }
    log(`installed ec to ${finalBinary}`);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

main().catch((err) => die(err.message || String(err)));
