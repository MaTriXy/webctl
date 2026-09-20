#!/usr/bin/env node
// Runs the platform binary that install.js placed next to this file.
const path = require("path");
const { spawnSync } = require("child_process");

const bin = path.join(__dirname, "webctl");
const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`webctl: ${result.error.message}; reinstall with \`npm install -g webctl\``);
  process.exit(1);
}
process.exit(result.status ?? 1);
