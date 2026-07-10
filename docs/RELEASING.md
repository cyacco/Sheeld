# Releasing Sheeld

Releases are cut by pushing a `vX.Y.Z` git tag. The
[`Release` workflow](../.github/workflows/release.yml) then builds and pushes the
three container images to GHCR and publishes a GitHub Release with
auto-generated notes. No manual image building or uploading is needed.

## Images

| Image | Contents |
|-------|----------|
| `ghcr.io/cyacco/sheeld-api` | Control plane (`cmd/control-plane`) |
| `ghcr.io/cyacco/sheeld-server` | Data plane / proxy (`cmd/sheeld-server`) |
| `ghcr.io/cyacco/sheeld-web` | Next.js dashboard (`web/`) |

Each is tagged `X.Y.Z`, `X.Y`, and `latest` — except that a **prerelease** tag
(one containing a hyphen, e.g. `v0.2.0-rc.1`) does **not** move `latest` and is
marked as a prerelease on GitHub.

## Cutting a release

1. **Land everything on `main`** and confirm CI is green there.
2. **Update the CHANGELOG.** Move items from `[Unreleased]` into a dated
   `## [X.Y.Z] - YYYY-MM-DD` section and update the comparison links at the
   bottom of [`CHANGELOG.md`](../CHANGELOG.md). Merge that as its own PR.
3. **Tag and push:**
   ```bash
   git checkout main && git pull
   git tag v0.1.0
   git push origin v0.1.0
   ```
4. **Watch the run:**
   ```bash
   gh run watch "$(gh run list --workflow Release --limit 1 --json databaseId --jq '.[0].databaseId')"
   ```
   All four jobs (three image builds + the GitHub Release) should be green.

## First release only: make the packages public

GHCR publishes new packages as **private** by default, so on the *first* tag the
images push successfully but nobody can `docker pull` them and `helm install`
fails with an auth error. This is a one-time fix per package — subsequent tags
reuse the same (now public) packages.

For each of `sheeld-api`, `sheeld-server`, `sheeld-web`:

1. Go to **`https://github.com/users/cyacco/packages/container/<name>/settings`**.
2. Under **Danger Zone → Change visibility**, set it to **Public**.
3. Use **Connect repository** to link the package to `cyacco/Sheeld`, so it
   appears on the repo page and inherits its access.

## Verifying a release

```bash
# The GitHub Release exists and isn't a draft:
gh release view v0.1.0

# Each image is publicly pullable at the new tag:
docker pull ghcr.io/cyacco/sheeld-api:0.1.0
docker pull ghcr.io/cyacco/sheeld-server:0.1.0
docker pull ghcr.io/cyacco/sheeld-web:0.1.0
```

If a pull fails with `denied` / `unauthorized`, the package is still private —
revisit the visibility step above.

## If something goes wrong

- **Workflow failed partway.** Fix the cause on `main`, delete the tag locally
  and remotely (`git tag -d v0.1.0 && git push origin :v0.1.0`), and re-tag. Only
  do this if the release was not yet announced or consumed.
- **Bad release already public.** Don't rewrite it — cut a `vX.Y.(Z+1)` patch
  instead.
