# Contributing to FilaBridge+

Thank you for considering contributing to FilaBridge+! This document provides guidelines and information for contributors.

## How to Contribute

### Reporting Bugs

Before creating a bug report:
1. Check the existing issues to avoid duplicates
2. Gather relevant information (OS, versions, printer stack, error messages)
3. Create a detailed issue with steps to reproduce

Include in your bug report:
- **Description**: Clear description of the bug
- **Steps to reproduce**: Numbered list of steps
- **Expected behavior**: What should happen
- **Actual behavior**: What actually happens
- **Environment**: OS, Go version, Node.js version (for web changes), printer model and integration path:
  - **Snapmaker U1** — Moonraker URL, toolhead count
  - **Bambu Lab** — Home Assistant version, ha-bambulab version, LAN vs Cloud
- **Logs**: Relevant log output from FilaBridge+, Moonraker, or Home Assistant (sanitize API keys and tokens!)

### Suggesting Features

Feature requests are welcome! Please:
1. Check if the feature has already been requested
2. Explain the use case and benefit
3. Provide examples of how it would work
4. Consider whether it fits the project scope (Spoolman inventory bridge for supported printer stacks)

### Submitting Pull Requests

1. **Fork the repository** and create a branch from `main`
2. **Make your changes** with clear, descriptive commits
3. **Test thoroughly** — ensure existing functionality still works
4. **Update documentation** if needed (README, `docs/`, code comments)
5. **Submit a PR** with a clear description of changes

#### PR Guidelines

- **One feature per PR**: Keep changes focused
- **Follow Go conventions**: Run `go fmt` and `go vet` on backend changes
- **Follow web conventions**: Run `npm run lint` in `web/` for front-end changes
- **Write clear commits**: Use conventional commit format (see below)
- **Update tests**: Add tests for new features
- **Document changes**: Update README or docs if user-facing

#### Conventional Commits

We use [Conventional Commits](https://www.conventionalcommits.org/) to automatically generate changelogs. Please format your commit messages as follows:

```
type(optional-scope): brief description

optional body

optional footer
```

**Commit Types:**
- `feat:` - New feature (appears in "Added" section)
- `fix:` - Bug fix (appears in "Fixed" section)
- `docs:` - Documentation changes (appears in "Documentation" section)
- `chore:` - Maintenance tasks (appears in "Changed" section)
- `refactor:` - Code refactoring (appears in "Changed" section)
- `perf:` - Performance improvements (appears in "Changed" section)
- `test:` - Test additions/changes
- `ci:` - CI/CD changes

**Examples:**
```bash
feat: add AMS slot validation for Bambu printers
fix(web): resolve dashboard refresh issue
fix(bambu): handle tray_change with empty RFID uuid
docs: update Home Assistant setup guide
chore: update dependencies
refactor(api): simplify printer status endpoint
perf: optimize database queries
test: add unit tests for spoolman client
ci: add automated changelog generation
```

**Breaking Changes:**
Add `!` after the type for breaking changes:
```bash
feat!: change API response format
```

**Scope (optional):**
Use scope to indicate the area of codebase affected:
```bash
feat(web): add print history filters
fix(moonraker): handle G-code download timeouts
fix(ha): validate utility_meter entities
docs(home-assistant): update Bambu package steps
```

This format helps us automatically generate changelogs and determine semantic version bumps.

## Development Setup

### Prerequisites

- **Go 1.23** or higher
- **Node.js 20+** and npm (for the Next.js web UI)
- **Docker** (recommended for Spoolman and full-stack testing)
- For manual printer testing, at least one of:
  - **Snapmaker U1** (or other Klipper/Moonraker printer) on your network
  - **Bambu Lab** printer integrated in **Home Assistant** via [ha-bambulab](https://github.com/greghesp/ha-bambulab)
- **Spoolman** for inventory integration

### Local Development

1. **Clone your fork**:
   ```bash
   git clone https://github.com/doutorinfamous/filabridge-plus.git
   cd filabridge-plus
   ```

2. **Run Spoolman** (for testing):
   ```bash
   docker run -d --name spoolman -p 8000:8000 ghcr.io/donkie/spoolman:latest
   ```

3. **Backend** (Go API on port 5001):
   ```bash
   cd backend
   go mod download
   go run . --port 5001
   ```

4. **Front-end** (Next.js UI on port 5000, proxies `/api` and `/ws` to the backend):
   ```bash
   cd web
   npm install
   npm run dev -- -p 5000
   ```

   Open http://localhost:5000. Override the backend URL with `BACKEND_URL` if needed (default: `http://127.0.0.1:5001`).

5. **Run tests**:
   ```bash
   cd backend && go test ./...
   cd web && npm run lint
   ```

### Docker (full stack)

```bash
docker-compose up -d
```

UI at http://localhost:5000. If Spoolman runs on the host, use `http://host.docker.internal:8000` as the Spoolman URL inside the container.

### Code Style

- Follow standard Go conventions (run `go fmt`)
- Use meaningful variable and function names
- Add comments for complex logic
- Keep functions focused and reasonably sized
- Handle errors appropriately
- Match existing patterns in `web/` (React, Tailwind, shadcn/ui)

### Project Structure

```
filabridge/
├── backend/               # Go API (internal port 5001)
│   ├── main.go            # Entry point and CLI flags
│   ├── core/              # Bridge, SQLite, config, jobs, history, NFC toolheads
│   ├── snapmaker/         # Snapmaker U1: Moonraker client, G-code parsing, monitor
│   ├── bambu/             # Bambu Lab: HA discovery, AMS trays, webhooks, YAML packages
│   ├── spoolman/          # Spoolman API client
│   ├── homeassistant/     # Home Assistant REST client
│   ├── nfc/               # NFC pairing sessions (spool + location)
│   └── server/            # HTTP API (Gin), WebSocket /ws/status, handlers
├── web/                   # Next.js + shadcn/ui (port 5000)
│   ├── app/               # Dashboard, History, NFC, Settings
│   ├── components/        # Printer cards, spool picker, settings forms
│   └── lib/               # API client, types, WebSocket hook
├── docs/                  # Setup guides (e.g. home-assistant-setup.md)
├── docker/entrypoint.sh   # Runs Go + Next.js in one container
└── docker-compose.yml
```

### Supported printer integrations

| Printer | Integration | Filament tracking |
|---------|-------------|-------------------|
| Snapmaker U1 (and other Moonraker/Klipper) | Direct Moonraker API | G-code parsing on print complete |
| Bambu Lab | Home Assistant + ha-bambulab | HA webhooks (`spool_usage`, `tray_change`, print events) |

Spoolman is required for all paths — FilaBridge+ debits inventory and mirrors tray assignments there.

## Testing

### Manual Testing

**Snapmaker / Moonraker**
1. Add printer in Settings → Printers with correct Moonraker host
2. Map spools to toolheads on the dashboard
3. Run a print and confirm usage is debited in Spoolman when the job completes
4. Verify G-code parsing for multi-toolhead jobs if applicable

**Bambu Lab / Home Assistant**
1. Configure HA URL and token in Settings → General
2. Register the printer and download the HA package YAML
3. Restart Home Assistant and validate entities (Validate HA in Settings)
4. Map AMS slots to Spoolman spools
5. Run a short print or use HA `rest_command` tests from `docs/home-assistant-setup.md`

**NFC**
1. Generate URLs on the NFC tab
2. Complete spool + location scan flow on `/nfc/scan`

**General**
- Test error handling (Spoolman down, printer offline, unmapped toolhead)
- Test print error resolution flow on the dashboard

### Automated Testing

- Write unit tests for new backend functions (`backend/**/*_test.go`)
- Test edge cases and error conditions
- Ensure tests are repeatable (use `httptest` for HTTP clients where possible)

## Areas for Contribution

Looking for ideas? Here are some areas that need help:

### High Priority
- Unit tests and integration tests (Moonraker monitor, Bambu webhooks, NFC sessions)
- Improved error handling and logging
- Documentation improvements and setup guides
- Bambu / Home Assistant package and webhook reliability

### Medium Priority
- Mobile-responsive UI improvements
- Print statistics and analytics in History
- Configuration import/export
- Additional Moonraker/Klipper printer models beyond Snapmaker U1

### Low Priority
- Support for additional inventory systems beyond Spoolman
- Internationalization (i18n) — UI is currently English-only
- REST API documentation (OpenAPI/Swagger)
- Additional database backends

## Communication

- **Issues**: For bugs and feature requests
- **Discussions**: For questions and general discussion
- **Pull Requests**: For code contributions

## Code of Conduct

### Our Standards

- Be respectful and inclusive
- Welcome newcomers and help them learn
- Focus on constructive feedback
- Assume good intentions

### Unacceptable Behavior

- Harassment or discrimination
- Trolling or inflammatory comments
- Publishing others' private information
- Any unprofessional conduct

## License

By contributing to FilaBridge+, you agree that your contributions will be licensed under the GNU General Public License v3.0.

## Questions?

If you have questions about contributing:
1. Check existing issues and discussions
2. Open a new discussion
3. Reach out to the maintainers

Thank you for contributing to FilaBridge+!
