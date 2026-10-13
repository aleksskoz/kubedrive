<h1>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/kubedrive-logo-dark.svg">
    <img src="assets/kubedrive-logo.svg" alt="KubeDrive" height="64">
  </picture>
</h1>

A fast, native-feeling desktop client for Kubernetes. macOS first; Windows and Linux later.

This repository hosts KubeDrive releases and the issue tracker. The source code is not public.

## Install

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

**Requirements:** macOS 13 Ventura or later, Apple silicon or Intel.

## Getting started

KubeDrive reads your kubeconfig the same way kubectl does (`$KUBECONFIG` or `~/.kube/config`)
and talks only to your clusters. The one exception is the update check below.

## Updating

From version 0.2.0 KubeDrive updates itself. On its second launch it asks whether to check for
updates automatically; if you agree, it looks at this repository's latest release once a day,
without sending any data about you or your clusters. You can also choose
**KubeDrive → Check for Updates…** at any time. Every update is signed, and KubeDrive refuses
one whose signature doesn't match.

Version 0.1.0 can't update itself: download the new DMG from
[Releases](https://github.com/aleksskoz/kubedrive/releases) once, or run `brew upgrade --cask kubedrive`.

## Uninstalling

```sh
brew uninstall --cask kubedrive          # add --zap to remove settings and caches too
```

Without Homebrew, move KubeDrive to the Trash; settings live in `~/Library/Application Support/KubeDrive`.

## Feedback

Found a bug or missing something? [Open an issue](https://github.com/aleksskoz/kubedrive/issues).

## License

KubeDrive is free to use under the [license](LICENSE).
