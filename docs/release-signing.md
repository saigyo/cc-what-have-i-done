# macOS Release Signing & Notarization

Release builds sign and notarize the darwin binaries when **all five**
repository secrets below are configured. Without them — including a partial
set, which is treated as absent — releases build unsigned exactly as before:
the signing step is skipped, never a failure. Signing runs
inside goreleaser (cross-platform, keychain-free, via anchore/quill) on the
regular `ubuntu-latest` release runner.

Certificate: `Developer ID Application: Markus Ackermann (5JHYPBANQ4)`
(Team ID `5JHYPBANQ4`).

## Required secrets (environment `release`)

The secrets live in the protected **`release` environment**, not as
repository secrets: repository secrets are readable by any workflow on any
ref, so a pushed `v*` tag carrying a modified workflow or malicious
goreleaser hooks could exfiltrate them. Environment secrets are withheld
until the environment's protection rules pass — the `release` environment
requires a review approval and only deploys from `v*` tags, so each release
run pauses in the Actions UI until approved.

Create these under **Settings → Environments → release → Environment
secrets**:

| Secret | Content |
|---|---|
| `MACOS_SIGN_P12` | base64 of the Developer ID Application certificate incl. private key, exported as `.p12` |
| `MACOS_SIGN_PASSWORD` | password chosen for the `.p12` export |
| `MACOS_NOTARY_ISSUER_ID` | Issuer UUID (App Store Connect → Users and Access → Integrations → Team Keys) |
| `MACOS_NOTARY_KEY_ID` | Key ID of the Team Key |
| `MACOS_NOTARY_KEY` | base64 of the downloaded `AuthKey_<KEYID>.p8` file |

## Exporting the certificate (local, one-time)

1. Open **Keychain Access**, keychain *login*, category *My Certificates*.
2. Expand `Developer ID Application: Markus Ackermann (5JHYPBANQ4)` so the
   private key is included, right-click → *Export…*, format `.p12`, choose
   a password (this becomes `MACOS_SIGN_PASSWORD`).
3. Base64-encode the export and copy it to the clipboard for the
   `MACOS_SIGN_P12` secret:

   ```bash
   base64 -i developerID.p12 | pbcopy
   ```

## Creating the App Store Connect API key (one-time)

1. Sign in at <https://appstoreconnect.apple.com> → **Users and Access** →
   **Integrations** → **Team Keys**.
2. Generate a key with the **Developer** role. Note the **Issuer ID**
   (`MACOS_NOTARY_ISSUER_ID`) and the **Key ID** (`MACOS_NOTARY_KEY_ID`).
3. Download the `AuthKey_<KEYID>.p8` file — Apple offers the download only
   once. Base64-encode it for the `MACOS_NOTARY_KEY` secret — quill treats
   the value as a file path or base64, so raw PEM text fails:

   ```bash
   base64 -i AuthKey_<KEYID>.p8 | pbcopy
   ```

## Verifying a released binary

After downloading and unpacking a darwin release archive:

```bash
codesign --verify --verbose ccwhid
spctl -a -vvv -t install ccwhid
```

Expected: `accepted` with `source=Notarized Developer ID`.

Note: bare Mach-O binaries cannot be stapled (`stapler staple` requires an
app bundle, dmg, or pkg). Gatekeeper validates the notarization ticket
online on first run — this is expected behavior, not a defect.

## Supply-chain pinning

The release job is the only one that sees the signing credentials, so it is
exempt from the repo's tag-pinning convention: its actions are pinned to
immutable commit SHAs (with `# vX.Y.Z` comments), and goreleaser is
installed by a dedicated workflow step that verifies the downloaded tarball
fail-closed against a SHA-256 digest committed in the workflow.
`goreleaser-action` is deliberately not used: it fetches the binary at
runtime with fail-open verification (checksum download failures are skipped
with a warning), which would let a replaced release asset run with the
keys. Only the separate `Release` step receives the secrets, and only after
the `release` environment's protection rules (required reviewer, `v*` tag
rule) have passed. The environment itself is configured under
**Settings → Environments → release**; if it is ever recreated, restore the
required-reviewer rule and the `v*` deployment tag rule — without them the
environment binding is decorative. Updates:

- **Action SHAs**: dependabot's `github-actions` ecosystem proposes bumps
  weekly (it rewrites the SHA and its version comment); review and merge.
- **goreleaser version + digest** (`GORELEASER_VERSION` /
  `GORELEASER_SHA256` in the workflow): bump deliberately — read the
  release notes, take the new `goreleaser_Linux_x86_64.tar.gz` digest from
  the release's `checksums.txt` AND cross-check it by hashing an
  independently downloaded tarball, update both values, then re-run the
  snapshot verification locally
  (`go run github.com/goreleaser/goreleaser/v2@<version> release --snapshot
  --clean --skip=publish` must still skip signing without secrets).

## Troubleshooting

- Notarization failures surface in the goreleaser step of the release
  workflow log (submission ID included).
- Fetch Apple's detailed log for a submission:

  ```bash
  xcrun notarytool log <submission-id> \
    --issuer <MACOS_NOTARY_ISSUER_ID> \
    --key-id <MACOS_NOTARY_KEY_ID> \
    --key AuthKey_<KEYID>.p8
  ```

- Notarization waits synchronously (`wait: true`) with a 20-minute cap
  **per submission**; the two darwin binaries are processed sequentially, so
  the stage can take up to ~40 minutes in the worst case. Apple typically
  takes 1–5 minutes each.
- A submission Apple **rejects** (or marks invalid) fails the release. A
  submission still pending when the 20-minute cap expires does **not**:
  goreleaser logs `notarize timeout` and continues, so such a release can
  ship before notarization completed. If Apple accepts afterwards, the
  online Gatekeeper check succeeds anyway (nothing is stapled); if Apple
  rejects afterwards, the binary stays unnotarized. After any release whose
  notarize stage logged a timeout, run the verification commands above and
  re-cut the release if they fail.
