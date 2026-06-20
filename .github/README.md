# GitHub Actions

## Release Please

Automated versioning and GitHub Releases from [Conventional Commits](../CONTRIBUTING.md).

### One-time setup (repository settings)

Open **Settings → Actions → General → Workflow permissions** and configure **both**:

1. Select **Read and write permissions** (not read-only)
2. Check **Allow GitHub Actions to create and approve pull requests**
3. Click **Save**

Direct link (replace owner/repo):

`https://github.com/doutorinfamous/filabridge-plus/settings/actions`

> New GitHub repositories default to read-only workflow permissions. Without the checkbox above, release-please fails with:
> `GitHub Actions is not permitted to create or approve pull requests`

If the repo belongs to an **organization**, an org admin may need to allow the same setting under **Organization → Settings → Actions → General**.

After saving, re-run the failed workflow: **Actions → Release Please → Re-run all jobs**.

### Maintainer workflow

1. Merge feature/fix PRs into `main` using conventional commit messages
2. Release Please opens or updates a **Release X.Y.Z** pull request
3. Review the generated changelog, then merge that PR
4. A git tag (`vX.Y.Z`) and GitHub Release are created automatically

### Bootstrap

The manifest starts at `0.1.0` (see [CHANGELOG.md](../CHANGELOG.md)). If no `v0.1.0` tag exists yet on GitHub, create it once so future releases only include commits after that point:

```bash
git tag v0.1.0
git push origin v0.1.0
```

### Optional: CI on Release PRs

PRs opened by `GITHUB_TOKEN` do not trigger other workflows by default. If you need tests to run on Release PRs, create a fine-grained PAT with **Contents** and **Pull requests** (read/write), add it as secret `RELEASE_PLEASE_TOKEN`, and set `token: ${{ secrets.RELEASE_PLEASE_TOKEN }}` in the workflow.
