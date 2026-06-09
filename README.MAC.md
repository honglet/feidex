# Feidex On macOS

This repository's built-in `daemon` management is Linux-only. On macOS, `launchd` is the natural first thing to try, but on this machine it turned out to be finicky enough that a `tmux`-managed process is the more pragmatic day-to-day choice.

This guide assumes you run Feidex from this checkout at:

- `/Volumes/Second HD/proj/feidex`

and start it with:

```bash
bin/feidex serve --config config.toml
```

## Why `launchd`

Using `launchd` gives you:

- start on login
- clean stop/start/restart commands
- automatic restart if the process exits
- stable log files instead of tying Feidex to an open terminal window

## Best-Effort `launchd` Setup

This section documents the best `launchd` setup we reached during debugging. It is useful as a reference and may work on another Mac, but for this specific machine/install we still preferred `tmux` in the end.

## Paths Used In This Setup

- Binary: `/Volumes/Second HD/proj/feidex/bin/feidex`
- Config: `/Volumes/Second HD/proj/feidex/config.toml`
- Local wrapper: `/Users/yuhong/bin/feidex-mac-launch.sh`
- Working directory: `/Users/yuhong`
- Data directory: `/Users/yuhong/feidex/.feidex-data`
- LaunchAgent file: `~/Library/LaunchAgents/com.yuhong.feidex.plist`
- Stdout log: `/tmp/feidex.stdout.log`
- Stderr log: `/tmp/feidex.stderr.log`
- Wrapper log: `/tmp/feidex.run.log`

The binary and config can live on the external volume, but the LaunchAgent working directory should stay on a normal local path under `/Users/...`. In testing, using `/Volumes/Second HD/...` as `WorkingDirectory` caused `getcwd: Operation not permitted` failures under `launchd`.

## 1. Create The LaunchAgent

First create a local wrapper script. Keeping the wrapper under `/Users/...` avoided `Operation not permitted` failures we saw when `launchd` tried to execute a script directly from the external volume.

Create `/Users/yuhong/bin/feidex-mac-launch.sh`:

```sh
#!/bin/sh
set -eu

LOG_FILE="/tmp/feidex.run.log"
exec >>"$LOG_FILE" 2>&1

echo "=== $(date) ==="
echo "pwd=$(pwd)"
env | sort
echo "--- starting feidex ---"

exec /usr/bin/env -i \
  HOME=/Users/yuhong \
  PATH=/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin \
  SHELL=/bin/zsh \
  LANG=en_US.UTF-8 \
  LC_ALL=en_US.UTF-8 \
  SSH_AUTH_SOCK="${SSH_AUTH_SOCK:-}" \
  "/Volumes/Second HD/proj/feidex/bin/feidex" \
  serve \
  --config \
  "/Volumes/Second HD/proj/feidex/config.toml"
```

Then make it executable:

```bash
chmod 755 /Users/yuhong/bin/feidex-mac-launch.sh
```

Then create `~/Library/LaunchAgents/com.yuhong.feidex.plist` with this content:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>com.yuhong.feidex</string>

    <key>ProgramArguments</key>
    <array>
      <string>/bin/sh</string>
      <string>/Users/yuhong/bin/feidex-mac-launch.sh</string>
    </array>

    <key>WorkingDirectory</key>
    <string>/Users/yuhong</string>

    <key>EnvironmentVariables</key>
    <dict>
      <key>HOME</key>
      <string>/Users/yuhong</string>

      <key>PATH</key>
      <string>/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>

      <key>SHELL</key>
      <string>/bin/zsh</string>

      <key>LANG</key>
      <string>en_US.UTF-8</string>

      <key>LC_ALL</key>
      <string>en_US.UTF-8</string>
    </dict>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>/tmp/feidex.stdout.log</string>

    <key>StandardErrorPath</key>
    <string>/tmp/feidex.stderr.log</string>
  </dict>
</plist>
```

## 2. Load And Start It

```bash
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.yuhong.feidex.plist
launchctl kickstart -kp gui/$(id -u)/com.yuhong.feidex
```

## 2.5. Use An Absolute `data_dir`

If your repo lives on an external volume, set `data_dir` in `config.toml` to a local absolute path:

```toml
data_dir = "/Users/yuhong/feidex/.feidex-data"
```

Avoid:

```toml
data_dir = ".feidex-data"
```

because relative state paths tied to an external-volume working directory can be fragile under `launchd`.

If it was already loaded and you changed the plist, reload it with:

```bash
launchctl bootout gui/$(id -u)/com.yuhong.feidex
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.yuhong.feidex.plist
launchctl kickstart -kp gui/$(id -u)/com.yuhong.feidex
```

## 3. Day-To-Day Management

Start:

```bash
launchctl kickstart -kp gui/$(id -u)/com.yuhong.feidex
```

Stop:

```bash
launchctl bootout gui/$(id -u)/com.yuhong.feidex
```

Restart:

```bash
launchctl kickstart -kp gui/$(id -u)/com.yuhong.feidex
```

Status:

```bash
launchctl print gui/$(id -u)/com.yuhong.feidex
```

## 4. Logs

Check logs with:

```bash
tail -f /tmp/feidex.stdout.log
tail -f /tmp/feidex.stderr.log
tail -f /tmp/feidex.run.log
```

In practice, `/tmp/feidex.run.log` was the most useful file, because the wrapper script controlled it directly. `launchd`'s own `stdout`/`stderr` capture was less reliable for debugging than expected.

## 5. Common Notes

- `launchd` requires absolute paths. Do not use relative paths in the plist.
- On this machine, use the real mounted path under `/Volumes/Second HD/...` for the binary and config, not a convenience alias path such as `/Users/yuhong/proj/feidex`.
- Do not use the external-volume repo path as `WorkingDirectory`. Using `/Volumes/Second HD/...` as the process cwd caused `getcwd` failures under `launchd`; a local directory such as `/Users/yuhong` worked.
- `launchd` starts jobs with a much smaller environment than an interactive shell. In particular, `codex` may fail unless `PATH`, `HOME`, and locale variables are set explicitly.
- Avoid embedding long shell one-liners directly in the plist when your paths contain spaces such as `/Volumes/Second HD/...`. A dedicated script file is more reliable and easier to debug.
- A direct `env -i ... feidex serve --config ...` launch from Terminal worked reliably on this machine. Reproducing that exact environment in a wrapper script was the most promising `launchd` approach.
- After `bootstrap`, it is normal to use `launchctl kickstart -kp ...` to force the first real spawn and print the spawned pid.
- Build the binary before loading the service:

```bash
mkdir -p bin
go build -o bin/feidex ./cmd/feidex
```

- If you replace the binary with a new build, a restart is enough:

```bash
launchctl kickstart -kp gui/$(id -u)/com.yuhong.feidex
```

- This LaunchAgent runs in your logged-in user session. That is usually the right choice for a personal Mac mini setup.

## 6. What We Tried

These points summarize the debugging path and the practical lessons from it:

- Using `/Users/yuhong/proj/feidex` as a convenience path was a mistake for `launchd`; the real `/Volumes/Second HD/...` path was safer for binary/config references.
- Using `/Volumes/Second HD/...` as `WorkingDirectory` failed with `getcwd: Operation not permitted`.
- Putting the wrapper script itself on the external volume also failed under `launchd` with `Operation not permitted`.
- A local wrapper under `/Users/yuhong/bin/` was significantly more reliable.
- `codex` depended on `node`, so `PATH` had to include `/opt/homebrew/bin`.
- A minimal environment created with `env -i` still allowed `feidex` to start correctly when launched manually from Terminal.
- Even after the `launchd` job successfully spawned the real `feidex` process, behavior was still inconsistent enough that `tmux` was chosen as the operational fallback on this machine.

## 7. Practical Recommendation

If you specifically want to keep experimenting with `launchd`, the setup above is the best one we found.

For a personal Mac mini installation, though, the lower-risk operational choice is:

- run Feidex in `tmux`
- keep the explicit `env -i` startup recipe
- treat `launchd` support here as best-effort rather than fully solved

## 8. Optional Aliases

If you want a shorter command surface, add shell aliases such as:

```bash
alias feidex-start='launchctl kickstart -kp gui/$(id -u)/com.yuhong.feidex'
alias feidex-restart='launchctl kickstart -kp gui/$(id -u)/com.yuhong.feidex'
alias feidex-stop='launchctl bootout gui/$(id -u)/com.yuhong.feidex'
alias feidex-status='launchctl print gui/$(id -u)/com.yuhong.feidex'
```

## 9. Uninstall

```bash
launchctl bootout gui/$(id -u)/com.yuhong.feidex
rm ~/Library/LaunchAgents/com.yuhong.feidex.plist
```
