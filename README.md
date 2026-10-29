<h1>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/kubedrive-logo-dark.svg">
    <img src="assets/kubedrive-logo.svg" alt="KubeDrive" height="64">
  </picture>
</h1>

A fast, native-feeling desktop client for Kubernetes, for macOS, Windows and Linux.

This repository hosts KubeDrive releases and the issue tracker. The source code is not public.

## Install

**Requirements:** macOS 13 Ventura or later (Apple silicon or Intel); Windows 10 1809 or later
or Windows 11 (x64 or Arm); Linux with GTK 4 and WebKitGTK 6.0: Debian 13, Ubuntu 24.04,
Fedora 40 and newer (x64). The [install guide](https://kubedrive.com/docs/install) has the details.

### macOS

With [Homebrew](https://brew.sh):

```sh
brew install --cask aleksskoz/tap/kubedrive
```

Or download `KubeDrive-<version>.dmg` from the [latest release](https://github.com/aleksskoz/kubedrive/releases/latest),
open it and drag KubeDrive to Applications. Each release lists the DMG's SHA-256:

```sh
shasum -a 256 -c KubeDrive-<version>.dmg.sha256
```

The app is signed with a Developer ID and notarized by Apple.

### Windows

Download `KubeDrive-<version>-x64-setup.exe` from the [latest release](https://github.com/aleksskoz/kubedrive/releases/latest)
(`KubeDrive-<version>-arm64-setup.exe` for Arm PCs) and run it. The installer is not code-signed
yet, so SmartScreen asks once: **More info → Run anyway**. The release notes carry each installer's
SHA-256. KubeDrive installs to `C:\Program Files\AKStudio\KubeDrive` and adds the WebView2 runtime if
the PC has none.

### Linux

From KubeDrive's package repository, so that system updates bring new versions too.
Debian 13, Ubuntu 24.04 and newer:

```sh
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://kubedrive.com/kubedrive.gpg | sudo tee /etc/apt/keyrings/kubedrive.gpg >/dev/null
echo "deb [signed-by=/etc/apt/keyrings/kubedrive.gpg] https://kubedrive.com/apt stable main" |
  sudo tee /etc/apt/sources.list.d/kubedrive.list
sudo apt update && sudo apt install kubedrive
```

Fedora 40 and newer:

```sh
sudo tee /etc/yum.repos.d/kubedrive.repo <<'EOF'
[kubedrive]
name=KubeDrive
baseurl=https://kubedrive.com/rpm
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://kubedrive.com/kubedrive.asc
EOF
sudo dnf install kubedrive
```

Or download `kubedrive_<version>_amd64.deb` or `kubedrive-<version>.x86_64.rpm` from the
[latest release](https://github.com/aleksskoz/kubedrive/releases/latest) and install it with
`sudo apt install ./…` or `sudo dnf install ./…`. The repository and the RPMs are signed with
KubeDrive's key ([kubedrive.asc](https://kubedrive.com/kubedrive.asc), fingerprint
`87AE 5682 0F7C 659C 28CF  91F8 9D49 8CCC F6E3 1831`).

## Getting started

KubeDrive reads your kubeconfig the same way kubectl does (`$KUBECONFIG` or `~/.kube/config`)
and talks only to your clusters. The one exception is the update check below.

## Updating

On its second launch KubeDrive asks whether to check for updates automatically; if you agree, it
looks at this repository's latest release once a day, without sending any data about you or your
clusters. You can also check by hand at any time: **KubeDrive → Check for Updates…** on macOS,
**File → Check for Updates…** on Windows and Linux.

- **macOS** updates itself. Every update is signed, and KubeDrive refuses one whose signature
  doesn't match. Version 0.1.0 can't update itself: download the new DMG once, or run
  `brew upgrade --cask kubedrive`.
- **Windows**: the update notice links the installer for your PC; run it over the old version,
  your settings stay.
- **Linux**: with the package repository, `sudo apt upgrade` or `sudo dnf upgrade` updates KubeDrive
  like any other package; otherwise the update notice links the new package.

## Uninstalling

- **macOS**: `brew uninstall --cask kubedrive` (add `--zap` to remove settings and caches too).
  Without Homebrew, move KubeDrive to the Trash; settings live in
  `~/Library/Application Support/KubeDrive`.
- **Windows**: **Settings → Apps → KubeDrive → Uninstall**. Settings stay in `%AppData%\KubeDrive`
  until you delete the folder.
- **Linux**: `sudo apt remove kubedrive` or `sudo dnf remove kubedrive`. Settings stay in
  `~/.config/KubeDrive`.

KubeDrive never changes your kubeconfig, so there is nothing to undo there.

## Benchmarks

[`benchmarks/`](benchmarks) holds the scripts and raw results behind
[kubedrive.com/performance](https://kubedrive.com/performance): how fast KubeDrive and other
Kubernetes clients open 50,000 pods. They are MIT-licensed, so you can rerun them yourself.

## Feedback

Found a bug or missing something? [Open an issue](https://github.com/aleksskoz/kubedrive/issues).

## License

KubeDrive is free to use under the [license](LICENSE).
