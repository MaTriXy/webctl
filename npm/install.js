// Downloads the webctl release binary matching this package's version and
// platform from GitHub Releases into bin/. Runs on npm install.
const fs = require("fs");
const os = require("os");
const path = require("path");
const https = require("https");
const { execFileSync } = require("child_process");

const { version } = require("./package.json");
const platform = { darwin: "darwin", linux: "linux" }[process.platform];
const arch = { x64: "amd64", arm64: "arm64" }[process.arch];
if (!platform || !arch) {
  console.error(`webctl: no prebuilt binary for ${process.platform}/${process.arch}; use go install github.com/dorkitude/webctl/cmd/webctl@latest`);
  process.exit(1);
}

const asset = `webctl_${version}_${platform}_${arch}.tar.gz`;
const url = `https://github.com/dorkitude/webctl/releases/download/v${version}/${asset}`;
const binDir = path.join(__dirname, "bin");
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "webctl-"));
const tarball = path.join(tmp, asset);

function download(u, dest, redirects = 0) {
  return new Promise((resolve, reject) => {
    https.get(u, { headers: { "User-Agent": "webctl-npm" } }, (res) => {
      if ([301, 302, 303, 307, 308].includes(res.statusCode) && res.headers.location && redirects < 5) {
        res.resume();
        return resolve(download(res.headers.location, dest, redirects + 1));
      }
      if (res.statusCode !== 200) {
        res.resume();
        return reject(new Error(`HTTP ${res.statusCode} for ${u}`));
      }
      const out = fs.createWriteStream(dest);
      res.pipe(out);
      out.on("finish", () => out.close(resolve));
      out.on("error", reject);
    }).on("error", reject);
  });
}

download(url, tarball)
  .then(() => {
    execFileSync("tar", ["-xzf", tarball, "-C", tmp, "webctl"]);
    fs.mkdirSync(binDir, { recursive: true });
    fs.copyFileSync(path.join(tmp, "webctl"), path.join(binDir, "webctl"));
    fs.chmodSync(path.join(binDir, "webctl"), 0o755);
    fs.rmSync(tmp, { recursive: true, force: true });
  })
  .catch((err) => {
    console.error(`webctl: failed to download ${url}: ${err.message}`);
    process.exit(1);
  });
