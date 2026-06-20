# GitHub Actions

## Release Please

Automated versioning and GitHub Releases from [Conventional Commits](../CONTRIBUTING.md).

### One-time setup (repository settings)

1. Open **Settings → Actions → General**
2. Under **Workflow permissions**, select **Read and write permissions**
3. Save

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
