# Design: Bundling Categraf in the Release Package and One-Click Installation from the Host List

This addresses the very first thing that blocks a new user from onboarding: installing the first categraf requires manually downloading the package, hand-editing the reporting address, and afterwards the UI gives no live feedback that it worked.

The problems it addresses (from the walkthrough):

- **C1**: the categraf installation document hardcodes `127.0.0.1:17000`, so the first machine always requires a manual edit (`fe: public/n9e-docs/categraf/zh_CN.md:17,22`; DocumentDrawer cannot substitute variables).
- **C2**: there is no live "waiting for the first machine to report" confirmation — you can only refresh manually (the host list page does no polling at all).
- Upstream categraf has no one-click installation script, only the "download manually → edit conf/config.toml → `sudo ./categraf --install --start`" flow; in an air-gapped or intranet environment even the download step is impossible.

## 1. Goals

1. The n9e release tar.gz (and the Docker image) bundles the categraf installation packages (linux amd64 + arm64), so that air-gapped and intranet environments work.
2. The host list page offers a copyable one-click installation command; running it on the target machine performs download → extract → set the reporting address → register the systemd service → start.
3. Zero manual editing of the reporting address: the script is rendered dynamically by the server, and the default address is "the address the target machine actually used to download the script".
4. The frontend confirms the first machine's report in real time, closing C2.

Non-goals (phase two, see §8): one-click installation on Windows, automatically attaching to a business group during installation, versioning categraf independently of n9e releases.

## 2. Key facts about the current state

### 2.1 Backend (ccfos/nightingale)

| Fact | Location |
|------|------|
| The release tar.gz contains 3 binaries plus `docker/ etc/ integrations/ cli/ n9e.sql`, about 167MB | `.goreleaser.yaml:81-86` |
| The frontend pub directory is embedded into the `n9e` binary via statik and is not in the tar file list | `.goreleaser.yaml:22-24`, `fe.sh`, `center/router/router.go:252` |
| There is already a precedent for "serving files anonymously from a directory on disk": the integrations icon | `center/router/router.go:437`, `center/router/router_builtin.go:308-317` |
| There are already precedents for unauthenticated endpoints: `/pub`, `/api/n9e/versions`, `/api/n9e/site-info` | `center/router/router.go:258,625,751` |
| The categraf heartbeat entry point `POST /v1/n9e/heartbeat` is anonymous by default (BasicAuth is commented out) | `center/router/router.go:898-905`, `etc/config.toml:47-50` |
| The heartbeat INSERTs the ident into the `target` table, and the frontend can poll `/api/n9e/targets` to confirm | `pushgw/idents/idents.go:137-150`, `center/router/router.go:422` |
| `site_url` lives in the DB configs table, is initialized to `http://<heartbeat IP>:17000`, and `/api/n9e/site-info` can be read anonymously | `center/center.go:173-234`, `center/router/router_config.go:66-69` |
| No categraf installation endpoint, script, or pinned version exists | A repository-wide grep finds nothing |

### 2.2 Frontend (n9e/fe)

| Fact | Location |
|------|------|
| The host list empty-state guide already exists, but its primary button only opens a static document drawer | `src/pages/hosts/pages/List/List.tsx:397-413` |
| DocumentDrawer simply fetches and renders markdown with no variable substitution | `src/components/DocumentDrawer/index.tsx:42-64` |
| The host list does not poll automatically; the empty state has to be refreshed by hand | No interval anywhere under `src/pages/hosts/` |
| A ready-made "command + copy button" component `Code` and a `copy2ClipBoard` helper exist | `src/components/Code/index.tsx`, `src/utils/index.ts:77-103` |
| `siteInfo` is already fetched into the Context at application startup, but the frontend never consumes `site_url` | `src/App.tsx:247-254` |
| The content of `en_US.md` is wrong — it describes the enterprise edition (http_provider / categraf_ent) | `public/n9e-docs/categraf/en_US.md:6-18` |

### 2.3 categraf (external constraints)

- A single-architecture tar.gz is about 49–55MB (the slim and full builds are almost the same size, so the full build is used).
- The target machine needs two edits in `conf/config.toml`: `[[writers]] url` (remote write) and `[heartbeat] url` (the heartbeat, which is what makes the machine appear on the list page; `enable = true` by default).
- systemd installation: `sudo ./categraf --install`, started with `--start`; user mode is `--user --install` (v0.4.5+).

## 3. Choosing an approach

### 3.1 Where to put the categraf package

| Option | Approach | Verdict |
|------|------|------|
| **A. A directory inside the release package (chosen)** | Add `agents/categraf/*.tar.gz` to the tar.gz and have the backend serve the files from disk | Works offline; one line in the goreleaser `files` list plus one download endpoint; does not inflate the binary or memory |
| B. go:embed into the n9e binary | Mirror the sandbox asset chain (`.github/workflows/n9e.yml:17-53`) | No. It adds 100MB to the binary and keeps it resident in memory, with no benefit |
| C. Do not bundle; have the script download from the public internet | wget from GitHub releases inside install.sh | No (kept as a fallback for A). It does not work in air-gapped or intranet environments, which defeats the purpose |

**Chosen: A with C as a fallback**: install.sh first downloads from the local n9e, and if that fails (an older package, or a deleted agents directory) it falls back to GitHub releases.

The public fallback source is GitHub only, not the flashcat CDN: the pinned version is exactly what `scripts/download_categraf.sh` fetched from GitHub, so GitHub is guaranteed to have it, whereas the CDN lags behind (its download page was still on v0.5.9 when v0.5.15 had already shipped), so falling back to it would 404.

### 3.2 Architecture matrix (confirmed)

**Both architectures bundled in full**: every release package bundles both linux-amd64 and linux-arm64 categraf packages (the monitored machines' architecture is not necessarily the n9e server's). The cost: the release package grows from ~167MB to **~270MB**, which has been accepted. Windows is out of scope for phase one; a documentation link covers it.

### 3.3 How the reporting address avoids manual editing (the core design)

The shape of the one-click command:

```bash
curl -fsSL http://<n9e-address>:17000/api/n9e/agents/categraf/install.sh | sudo bash
```

install.sh is **rendered dynamically** by the server, and the default reporting address is taken from the Host of this very HTTP request (honoring `X-Forwarded-Proto` / `X-Forwarded-Host`), falling back to `site_url`.

The logic closes on itself: **if the target machine could curl the script from this address, then this address must also work as the reporting address** — which is more reliable than reading site_url (site_url reflects the browser's viewpoint and does not guarantee the target machine can reach it). The script exposes it as the `N9E_HOST` environment variable so it can be overridden:

```bash
curl -fsSL http://10.1.2.3:17000/api/n9e/agents/categraf/install.sh | sudo N9E_HOST=http://other:17000 bash
```

## 4. Backend design

### 4.1 Packaging pipeline

1. Add `scripts/download_categraf.sh`:
   - The version is pinned in the script variable `CATEGRAF_VERSION` (e.g. `v0.5.15`); upgrading means changing one line, following the n9e release cadence.
   - It downloads `categraf-${VER}-linux-amd64.tar.gz` and `categraf-${VER}-linux-arm64.tar.gz` from the GitHub release into `agents/categraf/` and verifies them against `checksums.txt`.
   - If they already exist and pass verification, it skips them (friendly to repeated local builds).
2. `.goreleaser.yaml`:
   - add running that script to `before.hooks`;
   - append `agents/*` to `archives.files`.
3. `docker/Dockerfile.goreleaser`: add `ADD agents`. **This must not be missed** — a large share of users deploy via docker/compose, and missing it would deprive image users of the feature.
4. Add `agents/` to `.gitignore` (a build-time artifact, handled the same way as `pub/`).

### 4.2 HTTP API (new file `center/router/router_agent.go`)

All three endpoints are unauthenticated and mounted on the `pages` group (following the `site-info` and `integrations/icon` precedents):

| Endpoint | Behavior |
|------|------|
| `GET /api/n9e/agents/categraf/install.sh` | Renders the installation script with `text/template` (the template is go:embed'ed and only a few KB). Injected values: the default server address (§3.3), the categraf version, and the list of available architectures. `Content-Type: text/x-shellscript` |
| `GET /api/n9e/agents/categraf/download?arch=amd64\|arm64` | Serves the matching tar.gz from `AgentsDir`. **The arch parameter is validated against an allowlist** (only `amd64`/`arm64` are accepted, ruling out path traversal — the existing `builtinIcon` bare path concatenation is deliberately not copied). A missing file returns 404 (which triggers the script's fallback to GitHub) |
| `GET /api/n9e/agents/categraf/meta` | Returns `{"bundled": bool, "version": "v0.5.15", "arches": ["amd64","arm64"], "basic_auth": bool}`. The frontend uses it to decide between showing one-click installation and falling back to the document mode; `basic_auth` reflects whether `APIForAgent.BasicAuth` is enabled (a boolean only, never the credentials) |

New configuration option: `[Center] AgentsDir`, default `./agents/categraf` (following the default-value pattern of `BuiltinIntegrationsDir`, `center/cconf/conf.go:14`).

### 4.3 install.sh behavior

```
1. Preflight: root or sudo; either curl or wget present
2. Detect the architecture with uname -m (x86_64→amd64, aarch64→arm64; anything else errors out with a
   documentation link)
3. Download ${N9E_HOST}/api/n9e/agents/categraf/download?arch=${ARCH}
   └─ on failure → fall back to GitHub releases (error out if there is no internet access)
4. Extract into /opt/categraf (if it already exists, abort and suggest --force, so a user's edited
   configuration is never silently overwritten; with --force it runs ./categraf --stop first, then
   overwrites, keeping the customized files under conf)
5. Rewrite conf/config.toml with sed:
   ├─ [[writers]] url = "${N9E_HOST}/prometheus/v1/write"
   └─ [heartbeat] url = "${N9E_HOST}/v1/n9e/heartbeat"
6. With systemd → ./categraf --install && ./categraf --start
   Without systemd → start with nohup and print a note
7. Output: installation complete; the machine will appear on the n9e host list within about 10 seconds;
   a reminder to attach it to a business group
```

Supported overrides and flags: `N9E_HOST` (the reporting address), `--dir` (the installation directory, default `/opt/categraf`), `--force`, and `--auth user:pass` (for the `APIForAgent.BasicAuth` case, writing the basic auth fields of writers/heartbeat).

## 5. Frontend design (implemented in the separate n9e/fe repository)

1. New `InstallCategrafModal`:
   - Calls the meta endpoint when opened; `bundled=false` or a 404 (an older backend) → falls back to the existing document drawer.
   - An n9e address input, defaulting to `siteInfo.site_url || window.location.origin`, editable, with the command regenerated as it changes.
   - Uses the existing `Code` component to show the one-line command with a copy button, plus an alternative "download first, then review" command.
   - When `basic_auth=true`, it suggests appending `--auth user:pass`.
2. Entry points in `List.tsx`:
   - the primary button of the empty state `EmptyGuide` changes from "open the document" to opening this modal, with the documentation link demoted to secondary;
   - a permanent "Install collector" entry is added next to the refresh button in the toolbar (so new machines can be added even when the list is not empty).
3. **Closing C2 (polling confirmation)**:
   - While the modal is open, poll `GET /api/n9e/targets` every 5 s and compare against the ident set captured when it opened; on detecting a new machine, show "✓ machine xxx has reported", refresh the list automatically, and suggest attaching it to a business group;
   - In the empty state with no filters, the list itself polls every 10 s (so a user who runs the command without opening the modal still sees the machine appear).
4. **Wrapping up C1 in the documentation**:
   - `DocumentDrawer` gains an optional `variables` prop that substitutes `{{N9E_HOST}}` placeholders;
   - `zh_CN.md` is updated to reference the one-click installation;
   - and while we are there, fix the `en_US.md` content that wrongly describes the enterprise edition.

## 6. Compatibility and fallback matrix

| Combination | Behavior |
|------|------|
| New backend + new frontend | Full one-click installation plus polling confirmation |
| Old backend + new frontend | meta returns 404 → the frontend falls back to the document drawer, behaving as today |
| New backend + old frontend | The endpoints sit unused with no side effects; the command can still be obtained manually from the documentation |
| The agents directory is missing (deleted or trimmed by the user) | download returns 404 → the script falls back to GitHub; meta reports `bundled=false` → the frontend says so |
| The server sits behind a proxy or load balancer | install.sh rendering reads `X-Forwarded-*`; if that is still wrong, the user edits the address input in the modal |

## 7. Risks and security

- **Anonymous endpoint surface**: neither install.sh nor download contains sensitive information (the address comes from the caller's own Host, and categraf is public software), which is consistent with the existing anonymous policy for `/pub` and `site-info`.
- **Path safety**: the download endpoint uses an arch allowlist and never accepts an arbitrary filename.
- **The optics of curl | bash**: it is common practice in the industry (Datadog and others do the same), and the script is served by the user's own n9e rather than a third party; a "download and review first" alternative is provided.
- **When APIForAgent.BasicAuth is enabled**: credentials are never delivered through an anonymous endpoint; the user passes them explicitly with `--auth`, and meta only returns a boolean hint.
- **Release package size**: ~167MB → ~270MB (accepted).
- **categraf version lag**: the pinned version is updated with each n9e release, and the script's GitHub fallback path can always fetch exactly that pinned version.

## 8. Implementation checklist

### Backend (this repository)

- [ ] `scripts/download_categraf.sh`: download both architecture packages at the pinned version plus checksum verification
- [ ] `.goreleaser.yaml`: before hook plus `agents/*` appended to `archives.files`
- [ ] `docker/Dockerfile.goreleaser`: `ADD agents`
- [ ] `.gitignore`: append `agents/`
- [ ] `center/cconf/conf.go`: the `Center.AgentsDir` option (default `./agents/categraf`)
- [ ] `center/router/router_agent.go`: the three endpoints (install.sh rendering / download / meta)
- [ ] `center/router/templates/install-categraf.sh.tmpl` (go:embed): the script template from §4.3
- [ ] `center/router/router.go`: register the routes (pages group, unauthenticated)
- [ ] `etc/config.toml`: an AgentsDir example comment

### Frontend (the n9e/fe repository)

- [ ] `src/pages/hosts/pages/List/InstallCategrafModal.tsx`: the one-click installation modal (including polling confirmation)
- [ ] `src/pages/hosts/pages/List/List.tsx`: the empty-state and toolbar entry points plus empty-state auto-polling
- [ ] `src/pages/hosts/services.ts`: a wrapper for the meta endpoint
- [ ] `src/components/DocumentDrawer/index.tsx`: the optional `variables` placeholder substitution
- [ ] `public/n9e-docs/categraf/zh_CN.md`: update it to guide users to the one-click installation; fix the wrong content in `en_US.md`
- [ ] `src/pages/hosts/locale/*`: new strings

The two repositories can be developed in parallel; the meta endpoint decouples their release order.

### Phase two

- Automatically attaching to a business group during installation (extending the heartbeat protocol with host_tags, or server-side auto-attach rules)
- One-click installation on Windows (a PowerShell script)
- Lazy server-side download of categraf (shrinking the package again, with the agents directory filled in automatically when the internet is reachable)
