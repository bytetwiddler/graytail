# Installing graytail and grayquery

Pre-built binaries are published on the [releases page](https://github.com/bytetwiddler/graytail/releases/latest). Pick the install method for your platform below. (To build from source instead, see the [README](README.md#installation).)

## macOS and Linux (zip release)

1. Download the release archive, `graytail-vX.Y.Z.zip`.
2. Unzip it:

   ```bash
   unzip graytail-vX.Y.Z.zip -d graytail-release
   ```

   This produces `<os>/<arch>/graytail` and `<os>/<arch>/grayquery` for every supported platform. Pick the pair matching your machine — e.g. `darwin/arm64` for Apple Silicon Macs, `darwin/amd64` for Intel Macs, `linux/amd64` or `linux/arm64` for Linux.

3. Move both binaries somewhere on your `PATH` and make them executable:

   ```bash
   sudo mv graytail-release/darwin/arm64/graytail /usr/local/bin/
   sudo mv graytail-release/darwin/arm64/grayquery /usr/local/bin/
   chmod +x /usr/local/bin/graytail /usr/local/bin/grayquery
   ```

   (use the matching `linux/<arch>` path on Linux)

4. macOS only: these binaries aren't notarized, so Gatekeeper may block the first run. Clear the quarantine flag:

   ```bash
   xattr -d com.apple.quarantine /usr/local/bin/graytail /usr/local/bin/grayquery
   ```

5. Set up environment variables — see [Environment variables (macOS/Linux shells)](#environment-variables-macoslinux-shells) below — then run `graytail` or `grayquery` from anywhere.

## Debian/Ubuntu (.deb release)

1. Download the package, `graytail_X.Y.Z_amd64.deb`.
2. Install it:

   ```bash
   sudo apt install ./graytail_X.Y.Z_amd64.deb
   ```

   (`apt install ./...` is preferred over `dpkg -i` since it resolves any missing dependencies automatically)

   This installs `graytail` and `grayquery` to `/usr/bin/`, already on `PATH` — no manual move needed.

3. Set up environment variables — see [Environment variables (macOS/Linux shells)](#environment-variables-macoslinux-shells) below — then run `graytail` or `grayquery` from anywhere.

To uninstall: `sudo apt remove graytail`.

## Windows (zip release)

1. Download the release archive, `graytail-vX.Y.Z.zip`.
2. Extract it, then locate `windows\amd64\graytail.exe` and `windows\amd64\grayquery.exe` (or `windows\arm64\` on ARM64 Windows).
3. Move both `.exe` files to a permanent folder, e.g. `C:\Program Files\graytail\`, and add that folder to your `PATH`:
   Settings → System → About → Advanced system settings → Environment Variables → select `Path` under **User variables** → New → paste the folder path.
4. Set up the required environment variables using [`windows_env_setup.ps1`](windows_env_setup.ps1). From PowerShell:

   ```powershell
   .\windows_env_setup.ps1
   ```

   It prompts for `GRAYLOG_URL`, `GRAYLOG_API_TOKEN`, and `GRAYLOG_STREAM_ID`, and persists them at User scope, so `graytail.exe`/`grayquery.exe` work from any directory in any new shell — no `.env` file required.
5. Open a **new** PowerShell or cmd window (existing windows won't see the new variables) and run `graytail` or `grayquery`.

## Environment variables (macOS/Linux shells)

There's no setup script for macOS/Linux — export the variables directly in your shell's startup file so they're available in every new shell, from any directory:

```bash
# ~/.bashrc, ~/.zshrc, or ~/.profile
export GRAYLOG_URL="https://graylog.example.com:9000"
export GRAYLOG_API_TOKEN="your_token_here"
export GRAYLOG_STREAM_ID="000000000000000000000001"   # optional, defaults to "All Messages"
```

Then reload the shell (`source ~/.bashrc`) or open a new terminal.

Only `GRAYLOG_URL` and `GRAYLOG_API_TOKEN` are required; everything else (`TIMEOUT`, `INSECURE`, `LIMIT`, and the graytail/grayquery-specific overrides) has a working default — see the [README](README.md#configuration) for the full list.

**Alternative — a `.env` file.** If you'd rather not export real environment variables, both binaries also load a `.env` file (see `.env.example`) on startup. Note that it's only read from the **current working directory** at launch time, not from the binary's install location — so it only works if you always run the binary from that same directory. Exported environment variables (above) work from anywhere and are recommended for a system-wide install.

## Verifying the install

```bash
graytail -h
grayquery -h
```

If you see the usage/flags output — rather than a "command not found" error, or a missing URL/token error — the binary is on your `PATH` and configuration is resolving correctly.
