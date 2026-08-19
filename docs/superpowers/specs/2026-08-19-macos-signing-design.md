# macOS Code Signing & Notarization — Design

**Date:** 2026-08-19
**Status:** Approved

## Goal

Release builds of `ccwhid` for macOS (amd64 + arm64) are signed with the
Developer ID Application certificate and notarized by Apple, so downloaded
binaries run without a Gatekeeper warning. Linux/Windows builds are
unchanged. Builds without the signing secrets (forks, PRs, local snapshots)
keep working — signing is skipped, never a failure.

## Approach

Use goreleaser's built-in cross-platform notarization (`notarize.macos`,
backed by anchore/quill). It is pure Go and keychain-free, so the release
workflow **stays on `ubuntu-latest`** — no macOS runner, no temp-keychain
scripts, no codesign/notarytool hook steps.

Authentication is via an **App Store Connect API key** (issuer ID, key ID,
`.p8` key). The alternative route from the original task notes (Apple ID +
app-specific password with notarytool on a macOS runner) was considered and
rejected: goreleaser OSS does not support it, and the fallback would add a
macOS runner, keychain import/cleanup scripts, and per-binary hook ordering
for no functional gain.

No macOS universal binary: users get the correct per-arch archive, and
`go install` covers the rest (YAGNI, decided with Markus).

## Changes

### 1. `.goreleaser.yaml` — add `notarize` block

Only addition to the file; everything else stays as is:

```yaml
notarize:
  macos:
    - enabled: '{{ and (isEnvSet "MACOS_SIGN_P12") (isEnvSet "MACOS_SIGN_PASSWORD") (isEnvSet "MACOS_NOTARY_ISSUER_ID") (isEnvSet "MACOS_NOTARY_KEY_ID") (isEnvSet "MACOS_NOTARY_KEY") }}'
      ids: [ccwhid]
      sign:
        certificate: "{{ .Env.MACOS_SIGN_P12 }}"
        password: "{{ .Env.MACOS_SIGN_PASSWORD }}"
      notarize:
        issuer_id: "{{ .Env.MACOS_NOTARY_ISSUER_ID }}"
        key_id: "{{ .Env.MACOS_NOTARY_KEY_ID }}"
        key: "{{ .Env.MACOS_NOTARY_KEY }}"
        wait: true
        timeout: 20m
```

goreleaser runs this after build and before archiving: both darwin binaries
are signed (hardened runtime + timestamp, quill defaults) and notarized, and
the existing tar.gz archives contain the signed binaries. `enabled` requires
all five `MACOS_*` secrets to be set: an environment without them — or with
only a partial set, which could otherwise ship signed-but-unnotarized
binaries — builds exactly as today.

### 2. `.github/workflows/release.yml` — pass secrets through

Runner stays `ubuntu-latest`. The goreleaser step's `env` gains the five
secrets:

```yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          MACOS_SIGN_P12: ${{ secrets.MACOS_SIGN_P12 }}
          MACOS_SIGN_PASSWORD: ${{ secrets.MACOS_SIGN_PASSWORD }}
          MACOS_NOTARY_ISSUER_ID: ${{ secrets.MACOS_NOTARY_ISSUER_ID }}
          MACOS_NOTARY_KEY_ID: ${{ secrets.MACOS_NOTARY_KEY_ID }}
          MACOS_NOTARY_KEY: ${{ secrets.MACOS_NOTARY_KEY }}
```

Unset repository secrets expand to empty strings, which `isEnvSet` treats as
unset — the same workflow file works with and without secrets. No other
workflow changes.

### 3. `docs/release-signing.md` — operator documentation

New file documenting (secrets are created manually by Markus, never by
tooling):

- **Secrets table** (repository Settings → Secrets and variables → Actions):

  | Secret | Content |
  |---|---|
  | `MACOS_SIGN_P12` | base64 of the Developer ID Application cert incl. private key, exported as `.p12` |
  | `MACOS_SIGN_PASSWORD` | password chosen for the `.p12` export |
  | `MACOS_NOTARY_ISSUER_ID` | Issuer UUID (App Store Connect → Users and Access → Integrations → Team Keys) |
  | `MACOS_NOTARY_KEY_ID` | Key ID of the Team Key |
  | `MACOS_NOTARY_KEY` | base64 of the downloaded `AuthKey_<KEYID>.p8` file |

- **Local export steps**: export `Developer ID Application: Markus Ackermann
  (5JHYPBANQ4)` (Team ID `5JHYPBANQ4`) plus private key from the login
  keychain as `.p12` via Keychain Access, then
  `base64 -i developerID.p12 | pbcopy`; App Store Connect Team Key creation
  and one-time `.p8` download.
- **Verification** after downloading a released binary:

  ```bash
  codesign --verify --verbose ccwhid
  spctl -a -vvv -t install ccwhid
  # expected: "accepted", "source=Notarized Developer ID"
  ```

- **Stapling note**: bare Mach-O binaries cannot be stapled
  (`stapler staple` needs a bundle/dmg/pkg); Gatekeeper validates the
  notarization ticket online. Expected behavior, not a defect.
- **Troubleshooting**: notarization failures surface in the goreleaser step
  log; fetch Apple's detail log with
  `xcrun notarytool log <submission-id> --issuer <issuer> --key-id <key-id> --key <AuthKey.p8>`.

## Constraints

- No secrets committed, echoed, or logged; no `set -x` anywhere near
  signing (there are no shell signing steps at all in this design).
- No changes to Linux/Windows build or archive configuration.
- Workflow must succeed when secrets are absent (skip signing, not fail).
- Secret names follow goreleaser convention (`MACOS_SIGN_*`,
  `MACOS_NOTARY_*`), superseding the `APPLE_ID`/`APPLE_APP_PASSWORD` names
  from the original task notes, which belong to the rejected fallback.

## Testing

- `goreleaser check` must pass on the updated config.
- `goreleaser release --snapshot --clean --skip=publish` (local, no
  secrets) must still produce all archives — proves the skip path.
- CI has no new test surface; the existing release workflow is the
  integration test. First real verification happens on the next `v*` tag
  after Markus adds the secrets, using the verification commands above.
- Notarization adds ~1–5 min per darwin binary to the release job
  (`wait: true`, capped at 20m per submission; the two binaries are
  processed sequentially, so worst case ~40m total). If Apple rejects a
  submission the release fails visibly instead of shipping unsigned
  binaries. A submission still pending at the 20m cap does NOT fail the
  release — goreleaser logs `notarize timeout` and continues — so a
  timed-out release must be verified (and re-cut if Apple later rejects);
  the operator doc describes this.

## Manual steps for Markus (after merge)

1. Export the certificate as `.p12`, base64 it.
2. Create an App Store Connect Team Key (role: Developer is sufficient for
   notarization), download the `.p8`.
3. Create the five repository secrets.
4. Cut the next release; run the verification commands on a downloaded
   darwin archive.
