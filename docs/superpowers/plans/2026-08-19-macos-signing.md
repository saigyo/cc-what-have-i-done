# macOS Release Signing & Notarization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Release builds sign and notarize the darwin binaries via goreleaser's built-in cross-platform notarization, skipping cleanly when signing secrets are absent.

**Architecture:** A `notarize.macos` block in `.goreleaser.yaml` (backed by anchore/quill, pure Go) signs and notarizes both darwin binaries after build and before archiving, gated on all five `MACOS_*` secrets being set (`isEnvSet` AND-chain). The release workflow stays on `ubuntu-latest` and only passes five repository secrets into the goreleaser step's env. A new operator doc explains secret creation and post-release verification.

**Tech Stack:** goreleaser v2 (OSS), GitHub Actions, App Store Connect API key.

**Spec:** `docs/superpowers/specs/2026-08-19-macos-signing-design.md`

## Global Constraints

- No secrets committed, echoed, or logged; no `set -x` in any signing-related step (this design has no shell signing steps at all).
- No changes to Linux/Windows build or archive configuration in `.goreleaser.yaml`.
- The release workflow must succeed when the secrets are absent or only partially configured: signing is skipped, never a failure (`enabled` AND-chains `isEnvSet` over all five secrets; goreleaser's `isEnvSet` returns true only for set AND non-empty, and GitHub Actions expands unset secrets to empty strings).
- Secret names exactly: `MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD`, `MACOS_NOTARY_ISSUER_ID`, `MACOS_NOTARY_KEY_ID`, `MACOS_NOTARY_KEY`.
- Runner stays `ubuntu-latest`; no macOS runner, no keychain scripts.
- goreleaser is not installed locally: invoke it as `go run github.com/goreleaser/goreleaser/v2@v2.17.1 <args>` — the exact version the release workflow pins, so local validation exercises the same schema and notary implementation (first run downloads modules; that is expected and may take a few minutes).
- Go gates for the repo still apply before any commit that touches Go-adjacent files (none here, but run them once at the end anyway): `gofmt -l . && go vet ./... && go test -race ./...`.

---

### Task 1: goreleaser notarize block + workflow secret passthrough

**Files:**
- Modify: `.goreleaser.yaml` (append a top-level `notarize` section after the `checksum` section)
- Modify: `.github/workflows/release.yml:28-29` (extend the goreleaser step's `env`)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: the five secret/env names listed in Global Constraints — Task 2's documentation must match them exactly.

- [ ] **Step 1: Verify the config is currently valid (baseline)**

Run from the repo root:

```bash
go run github.com/goreleaser/goreleaser/v2@v2.17.1 check
```

Expected: `1 configuration file(s) validated` and exit code 0. (First invocation downloads goreleaser modules; allow a few minutes.)

- [ ] **Step 2: Add the notarize block to `.goreleaser.yaml`**

Insert between the `checksum:` section and the `changelog:` section (top-level key, exact content):

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

Do not touch `builds`, `archives`, `checksum`, `changelog`, or `release` sections.

- [ ] **Step 3: Validate the changed config**

```bash
go run github.com/goreleaser/goreleaser/v2@v2.17.1 check
```

Expected: still valid, exit code 0. If it reports an unknown field, the block is misplaced or mis-indented — `notarize` must be a top-level key.

- [ ] **Step 4: Prove the skip path with a local snapshot build**

Run WITHOUT any of the MACOS_* env vars set:

```bash
go run github.com/goreleaser/goreleaser/v2@v2.17.1 release --snapshot --clean --skip=publish
```

Expected: exit code 0; `dist/` contains all six platform builds and the archives `cc-what-have-i-done_*_darwin_amd64.tar.gz` and `..._darwin_arm64.tar.gz`; the log shows the notarize step skipped (no signing attempted, no error). Afterwards remove the build output:

```bash
rm -rf dist/
```

- [ ] **Step 5: Extend the release workflow env**

In `.github/workflows/release.yml`, the goreleaser step currently ends with:

```yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Replace that `env:` block with:

```yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          MACOS_SIGN_P12: ${{ secrets.MACOS_SIGN_P12 }}
          MACOS_SIGN_PASSWORD: ${{ secrets.MACOS_SIGN_PASSWORD }}
          MACOS_NOTARY_ISSUER_ID: ${{ secrets.MACOS_NOTARY_ISSUER_ID }}
          MACOS_NOTARY_KEY_ID: ${{ secrets.MACOS_NOTARY_KEY_ID }}
          MACOS_NOTARY_KEY: ${{ secrets.MACOS_NOTARY_KEY }}
```

Additionally (security hardening, decided with Markus during review): the job handles the signing credentials, so nothing mutable may decide what code runs in it. Pin `actions/checkout` and `actions/setup-go` to immutable commit SHAs with `# vX.Y.Z` comments, and do NOT use `goreleaser/goreleaser-action` — it downloads the goreleaser binary at runtime with fail-open verification (checksum download failures are skipped with a warning). Instead, install goreleaser in a dedicated step that downloads the `v2.17.1` Linux x86_64 tarball and verifies it fail-closed against a SHA-256 digest committed in the workflow (`sha256sum -c`), then run the extracted binary in a separate `Release` step that alone receives the secrets. Trigger, permissions, and runner (`ubuntu-latest`) stay unchanged; `ci.yml` keeps the repo's tag-pinning convention.

- [ ] **Step 6: Sanity-check the workflow YAML parses**

```bash
python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml')); print('yaml ok')"
```

Expected: `yaml ok`. (If PyYAML is unavailable, `ruby -ryaml -e "YAML.load_file('.github/workflows/release.yml'); puts 'yaml ok'"` is an acceptable substitute.)

- [ ] **Step 7: Commit**

```bash
git add .goreleaser.yaml .github/workflows/release.yml
git commit -m "feat(release): sign and notarize darwin binaries via goreleaser"
```

### Task 2: Operator documentation `docs/release-signing.md`

**Files:**
- Create: `docs/release-signing.md`

**Interfaces:**
- Consumes: the five secret names from Task 1 (`MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD`, `MACOS_NOTARY_ISSUER_ID`, `MACOS_NOTARY_KEY_ID`, `MACOS_NOTARY_KEY`) — names in the doc must match the config verbatim.
- Produces: nothing consumed by other tasks.

- [ ] **Step 1: Create `docs/release-signing.md` with exactly this content**

````markdown
# macOS Release Signing & Notarization

Release builds sign and notarize the darwin binaries when **all five**
repository secrets below are configured. Without them — including a partial
set, which is treated as absent — releases build unsigned exactly as before:
the signing step is skipped, never a failure. Signing runs
inside goreleaser (cross-platform, keychain-free, via anchore/quill) on the
regular `ubuntu-latest` release runner.

Certificate: `Developer ID Application: Markus Ackermann (5JHYPBANQ4)`
(Team ID `5JHYPBANQ4`).

## Required repository secrets

Create these under **Settings → Secrets and variables → Actions**:

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
````

- [ ] **Step 2: Verify secret-name consistency between doc, config, and workflow**

```bash
for s in MACOS_SIGN_P12 MACOS_SIGN_PASSWORD MACOS_NOTARY_ISSUER_ID MACOS_NOTARY_KEY_ID MACOS_NOTARY_KEY; do
  if grep -q "$s" .goreleaser.yaml && grep -q "$s" .github/workflows/release.yml && grep -q "$s" docs/release-signing.md; then
    echo "$s ok"
  else
    echo "$s MISSING"
  fi
done
```

Expected: five lines ending in `ok`, no `MISSING`.

- [ ] **Step 3: Run the repo gates**

```bash
gofmt -l . && go vet ./... && go test -race ./...
```

Expected: no gofmt output, vet clean, all tests pass (no Go code changed; this is the pre-commit gate).

- [ ] **Step 4: Commit**

```bash
git add docs/release-signing.md
git commit -m "docs: operator guide for macOS release signing secrets"
```
