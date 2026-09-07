# Feidex LaunchAgent Access To External Volumes

This guide keeps the Feidex binary and configuration in the repository while
allowing a per-user macOS `launchd` service to use workspaces on a removable
volume.

The important parts are:

- Keep the LaunchAgent working directory local, such as `/Users/yuhong`.
- Keep the binary and config at their normal repository paths.
- Sign each macOS build with the same stable code-signing identity.
- Approve the first removable-volume request from the logged-in macOS desktop.

## Paths Used By This Installation

```text
Repository:       /Volumes/Second HD/proj/feidex
Binary:           /Volumes/Second HD/proj/feidex/bin/feidex
Config:           /Volumes/Second HD/proj/feidex/config.toml
Launcher:         /Users/yuhong/bin/feidex-mac-launch.sh
LaunchAgent:      /Users/yuhong/Library/LaunchAgents/com.yuhong.feidex.plist
WorkingDirectory: /Users/yuhong
Data directory:   /Users/yuhong/feidex/.feidex-data
```

The LaunchAgent must not use the external repository as its
`WorkingDirectory`. The launcher can still execute the binary and read the
config from `/Volumes/Second HD`.

## One-Time Certificate Setup

Use the logged-in macOS desktop, either locally or through Remote Desktop.

1. Open **Keychain Access**.
2. Select the `login` keychain.
3. Choose **Keychain Access -> Certificate Assistant -> Create a Certificate**.
4. Use this name:

   ```text
   Feidex Local Code Signing
   ```

5. Select `Self Signed Root` for the identity type and `Code Signing` for the
   certificate type.
6. Complete the assistant. If offered, enable **Let me override defaults**.
7. Open the new certificate, expand **Trust**, set **When using this
   certificate** to **Always Trust**, close the window, and authenticate.

Confirm that macOS recognizes the identity:

```bash
security find-identity -v -p codesigning
```

The output must list `Feidex Local Code Signing` as a valid identity. An Apple
Development or Developer ID certificate can be used instead.

## Build And Sign

Run these commands in Terminal:

```bash
cd "/Volumes/Second HD/proj/feidex"

FEIDEX_CODESIGN_IDENTITY="Feidex Local Code Signing" \
  ./scripts/build_macos_signed.sh
```

Verify that the designated requirement is certificate-based:

```bash
codesign -dr - "/Volumes/Second HD/proj/feidex/bin/feidex"
```

The result must not be only a requirement of the form `cdhash H"..."`.

## Reload The LaunchAgent

Stop any old copy first. This is especially important if an earlier ad-hoc
build was running from a temporary local copy:

```bash
launchctl bootout gui/$(id -u)/com.yuhong.feidex 2>/dev/null || true
```

Load and start the current LaunchAgent:

```bash
launchctl bootstrap gui/$(id -u) \
  "$HOME/Library/LaunchAgents/com.yuhong.feidex.plist"

launchctl kickstart -kp gui/$(id -u)/com.yuhong.feidex
```

Watch the macOS desktop for a prompt that Feidex wants to access files on a
removable volume. Click **Allow**. This is a one-time TCC authorization for the
stable signed identity. Remote Desktop or Screen Sharing is sufficient; an SSH
session cannot answer the prompt.

## Verify The Service

```bash
launchctl print gui/$(id -u)/com.yuhong.feidex \
  | rg 'state =|pid =|runs ='

ps aux | rg '[f]eidex'

tail -80 /tmp/feidex.run.log
tail -80 /tmp/feidex.stderr.log
```

The running command must reference:

```text
/Volumes/Second HD/proj/feidex/bin/feidex
```

Then send a simple request through Feishu in a workspace whose `cwd` is under
`/Volumes/Second HD`, for example a request to list the files. This confirms
that Feidex, Codex, and Git can use the external workspace under LaunchAgent.

## Future Rebuilds

Always sign rebuilt binaries with the same identity:

```bash
cd "/Volumes/Second HD/proj/feidex"
FEIDEX_CODESIGN_IDENTITY="Feidex Local Code Signing" \
  ./scripts/build_macos_signed.sh
launchctl kickstart -kp gui/$(id -u)/com.yuhong.feidex
```

Do not copy the binary or config to a second deployment directory. Changing the
signing identity, identifier, or binary path may require a new TCC approval.

## Troubleshooting

If `security find-identity` reports zero valid identities, fix the certificate
trust settings in Keychain Access before rebuilding.

If the service runs but external workspaces hang, check these items:

1. `codesign -dr - bin/feidex` still shows the stable certificate requirement.
2. The LaunchAgent `WorkingDirectory` is local, not under `/Volumes/Second HD`.
3. The external volume is mounted before starting the job.
4. The one-time TCC prompt was approved in the logged-in desktop session.
5. `/tmp/feidex.run.log` and `/tmp/feidex.stderr.log` do not show a new
   authorization or `Operation not permitted` failure.

There is no supported `tccutil` command to grant this permission. `tccutil` can
only reset decisions. Fully unattended authorization requires an MDM-delivered
PPPC policy for `SystemPolicyRemovableVolumes` using the same stable designated
code requirement.
