# Brew Release Runbook

This runbook documents how to publish macOS Homebrew assets and keep the in-repo tap formula synchronized.

## Scope

- Supported architecture for initial release: `darwin-arm64` only.
- Homebrew tap location: `https://github.com/EndersonPro/homebrew-flutree` (`Formula/flutree.rb`).

## Packaging scripts

`scripts/package_macos.sh`:
- builds standalone binary from `./cmd/flutree` with `go build`;
- creates `flutree-${VERSION}-macos-${ARCH}.tar.gz`;
- creates `flutree-${VERSION}-macos-${ARCH}.sha256`;
- validates architecture contract (`ARCH=arm64` only).

Required env vars:
- `VERSION` (no leading `v`, example `0.7.0`)
- `ARCH` (`arm64`)

Optional env var:
- `OUTPUT_DIR` (defaults to `dist`)

Examples:

```bash
VERSION=0.7.0 ARCH=arm64 ./scripts/package_macos.sh build
VERSION=0.7.0 ARCH=arm64 ./scripts/package_macos.sh contract
./scripts/verify_macos_binary.sh dist/flutree-0.7.0-macos-arm64.tar.gz
```

## Formula contract

`Formula/flutree.rb` in `EndersonPro/homebrew-flutree` MUST stay synchronized on every release:
- `version` matches released CLI version (without `v`)
- `url` points to matching GitHub Release asset
- `sha256` matches published `.sha256` file

Formula URL/checksum naming contract:
- `flutree-${VERSION}-macos-arm64.tar.gz`
- `flutree-${VERSION}-macos-arm64.sha256`

## Maintainer checklist

1. Create and push tag `vX.Y.Z`.
2. Ensure repository secret `HOMEBREW_TAP_TOKEN` is set in `EndersonPro/flutree` with push access to `EndersonPro/homebrew-flutree`.
3. Wait for `.github/workflows/release-brew.yml` to publish tarball + sha256 assets and auto-update the tap formula.
4. Confirm workflow steps passed:
   - release asset publish
   - tap formula update commit/push
5. Validate install on clean macOS ARM machine:
   - `brew tap EndersonPro/flutree`
   - `brew install EndersonPro/flutree/flutree`
   - `flutree --help`
6. If automation fails, manually patch `Formula/flutree.rb` in `homebrew-flutree` and rerun install validation.
