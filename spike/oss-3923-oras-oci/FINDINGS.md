# OSS-3923 spike findings: oras-go/v2 vs the OpenTofu OCI module contract

Date: 2026-07-10. Spike code: `/tmp/claude-1001/-projects-gruntwork-terragrunt-2/e6de581e-e844-4835-b38a-8f143fc7f6d5/scratchpad/oci-spike/` (throwaway, nothing merged under `internal/`).
Setup: local `registry:3` (distribution v3) with self-signed TLS (+ htpasswd variant), artifacts published with `oras` CLI v1.3.2, consumed by OpenTofu 1.10.6 and 1.12.2 (via mise) and by a clean-room Go program on `oras.land/oras-go/v2 v2.5.0`.

## Verdict

oras-go/v2 v2.5.0 reproduces OpenTofu 1.10's observable OCI module contract end-to-end. A tofu-published artifact pulled by the spike is byte-identical to what `tofu init` extracts, by tag and by digest, with blob digest verification. Pin v2.5.0. One implementation trap found in ambient credentials (below).

## Acceptance criteria status

- Spike pulls a tofu-published module artifact end-to-end and verifies the blob digest: DONE (tag + digest refs; `diff -r` identical to tofu 1.10.6 output; drift-checked identical on 1.12.2).
- Findings note with subpackage list, seam shape, pinned version: this document.
- Clean-room: no OpenTofu source consulted; 1.10.6/1.12.2 binaries used as behavioral oracles only.

## 1. Pinned version and dependency delta (feeds OSS-3924)

- Pin `oras.land/oras-go/v2 v2.5.0` (matches OpenTofu 1.10; validated here).
- Full transitive set of oras-go v2.5.0: `github.com/opencontainers/go-digest v1.0.0`, `github.com/opencontainers/image-spec v1.1.1`, `golang.org/x/sync` - ALL already present in terragrunt go.mod (x/sync v0.21.0, both opencontainers modules as indirect at exactly the needed versions).
- Net delta of the dependency PR: ONE new module (oras-go itself). image-spec and go-digest flip indirect -> direct when imports land (OSS-3927+).
- License: Apache-2.0.

## 2. Confirmed API surface / subpackage list (revises the ticket's guess)

| Subpackage | Needed | Used for |
|---|---|---|
| `registry/remote` | YES | `remote.NewRepository(domain + "/" + repo)`; `Resolve`, `Fetch` |
| `registry/remote/auth` | YES | `auth.Client{Client, Cache: auth.NewCache(), Credential}` |
| `registry/remote/credentials` | YES | `NewStore(path, ...)`, `NewStoreFromDocker`, `NewStoreWithFallbacks`, `Credential(store)` |
| `content` | YES | `content.FetchAll` (manifest, digest+size verified), `content.NewVerifyReader` + `Verify()` (streamed blob digest verification; go-digest under the hood) |
| `errdef` | YES | `errors.Is(err, errdef.ErrNotFound)` on missing tag/digest |
| `registry` | NO (drop from list) | not needed; `remote.NewRepository` parses references itself |
| `registry/remote/errcode` | ADD | registry HTTP errors surface as `errcode.ErrorResponse` (e.g. 401), needed for typed-error mapping |

## 3. Seam shape validated (feeds OSS-3927)

Both compile-time assertions hold against v2.5.0:

```go
var _ OCIRepositoryStore = (*remote.Repository)(nil)  // satisfies the seam AS-IS
var _ content.Fetcher = (OCIRepositoryStore)(nil)     // seam doubles as content.Fetcher
```

- `*remote.Repository` implements `Resolve(ctx, ref) (ocispec.Descriptor, error)` and `Fetch(ctx, desc) (io.ReadCloser, error)` directly; the planned `OCIRepositoryStore` interface needs no adapter.
- Because the seam is signature-identical to `content.Fetcher`, `content.FetchAll` and `content.NewVerifyReader` work through the seam, so the getter needs no oras-go types in its own signatures beyond `ocispec.Descriptor`.
- `NewStore func(ctx, registryDomain, repositoryName)` is the right injection point: repository ref is `registryDomain + "/" + repositoryName`; credentials and TLS live entirely inside the closure.

## 4. Contract matrix (all observed against tofu 1.10.6; drift-checked on 1.12.2: identical)

| Case | tofu behavior (exact where quoted) |
|---|---|
| `?tag=1.0.0` | downloads, extracts zip to module dir |
| `?digest=sha256:...` | downloads by manifest digest |
| no query | defaults to tag `latest` (`resolving tag: ...:latest: not found` when absent) |
| `//subdir` | whole zip extracted, module `Dir` points into subdir (go-getter semantics); subdir stripped before download |
| `?tag=&digest=` together | error: `cannot set both "tag" and "digest" arguments` |
| unknown query param | error: `unsupported argument "foo"` |
| manifest artifactType != `application/vnd.opentofu.modulepkg` | error: `unexpected artifact type ...` |
| two `archive/zip` layers | error: `multiple layers with media type ...` |
| zero `archive/zip` layers | error: `image manifest contains no layers of types supported as module packages by OpenTofu ...` |
| plain-HTTP registry (incl. localhost) | REFUSED: `http: server gave HTTP response to HTTPS client` (no docker-style localhost exception) |
| error wrapper format | `error downloading '<canonicalized-src>': <cause>` (go-getter style; query params re-serialized alphabetically, digest colon percent-encoded) |

Manifest shape produced by `oras push --artifact-type application/vnd.opentofu.modulepkg module.zip:archive/zip`: OCI image manifest (`application/vnd.oci.image.manifest.v1+json`), `artifactType` set, empty config (`application/vnd.oci.empty.v1+json`), one `archive/zip` layer.

## 5. Ambient credential order (feeds OSS-3929) - THE trap

Empirical, with basic-auth registry and hermetic HOME/XDG:

- creds only in `$HOME/.docker/config.json`: tofu OK.
- WRONG creds in `$HOME/.docker/config.json` + CORRECT in `$XDG_RUNTIME_DIR/containers/auth.json`: tofu OK -> containers/auth.json takes precedence over Docker config, confirming the OpenTofu search order.
- control (wrong creds only): tofu 401.
- oras-go DEFAULT (`credentials.NewStoreFromDocker`): reads ONLY Docker config -> got 401 in the precedence setup. It does NOT reproduce OpenTofu's ambient order.
- FIX validated: per-path `credentials.NewStore(path, ...)` over the OpenTofu candidate list (`$XDG_RUNTIME_DIR/containers/auth.json`, `$HOME/.config/containers/auth.json`, `$XDG_CONFIG_HOME/containers/auth.json`, `$HOME/.docker/config.json`), chained with `credentials.NewStoreWithFallbacks(first, rest...)` -> reproduces tofu behavior in both setups.

Implication for OSS-3929: implement explicit multi-path discovery; do NOT rely on `NewStoreFromDocker` alone. (`$HOME/.dockercfg` legacy format remains unverified; oras-go file store parses the modern schema.)

## 6. Typed-error mapping targets (feeds OSS-3927/3928)

- missing tag/digest: `errors.Is(err, errdef.ErrNotFound)` = true (message `<repo>:<tag>: not found`).
- auth failure: `*errcode.ErrorResponse` with StatusCode 401 (wrap into a typed credential error).
- digest mismatch on blob: `content.NewVerifyReader(...).Verify()` returns error (fail closed).
- validation errors (artifact type, layer count, query params) are OUR checks; mirror tofu's behaviors from the matrix above.

## 7. Notes for later tickets

- TLS: tofu offers no plain-HTTP or insecure-registry escape hatch observed in 1.10.6; integration tests (OSS-3935) must run the local registry with TLS and trust the CA via `SSL_CERT_FILE` (works for tofu; spike passes the CA into the http.Client).
- OSS-3930 gating test should assert Terragrunt's go-getter v2 fallthrough error (`error downloading 'oci://...'`), matching tofu's wrapper format.
- Registry for tests: `registry:3` handles OCI 1.1 artifactType fine; `oras` CLI v1.3.2 publishing flow matches the OpenTofu docs example.
