# joca

A terminal UI for monitoring and managing CI/CD pipelines.

**Stack:** Go · [Cobra](https://github.com/spf13/cobra) · [Bubble Tea](https://github.com/charmbracelet/bubbletea) · [Lip Gloss](https://github.com/charmbracelet/lipgloss)

**Providers:**
- **GitHub Actions** — authenticates via `GITHUB_TOKEN` env var or `gh auth token`
- **AWS CodePipeline** — uses the AWS SDK v2 default credential chain; supports named profiles

---

## Install / Build

```bash
# Build and run
go build ./...
go run .

# Versioned binary
go build \
  -ldflags "-X github.com/thiagomarinho/joca/version.Version=0.1.0 \
            -X github.com/thiagomarinho/joca/version.Commit=$(git rev-parse --short HEAD)" \
  -o joca .
```

---

## Configuration

joca stores its config at `~/.joca/config.yaml`.

```yaml
refresh_interval: 30s
pipelines:
  - name: my-app CI
    provider: github
    owner: acme
    repo: my-app
    workflow: ci.yml       # omit to track all workflows

  - name: deploy-prod
    provider: aws
    pipeline_name: deploy-prod
    aws_profile: production   # optional; uses default chain if omitted
    aws_region: us-east-1
```

### Adding pipelines via CLI

```bash
joca add github owner/repo           # adds a GitHub Actions pipeline
joca add aws pipeline-name           # adds an AWS CodePipeline pipeline
```

---

## TUI Keybindings

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `Enter` | Open pipeline detail / run logs |
| `o` | Open in browser |
| `r` | Refresh now |
| `a` | Add pipeline (inline form) |
| `q` / `Ctrl+C` | Quit |

---

## Roadmap

| Feature | Notes |
|---------|-------|
| Trigger / re-run pipeline (`R`) | `Trigger()` is already implemented in both providers; needs TUI wiring |
| Delete pipeline (`d`) | Needs config write + inline confirm prompt |
| Filter pipelines in list | Type-to-search, similar to the add wizard |
| AWS approval action | Approve / reject from TUI when a pipeline awaits manual approval |
| Real log content for GitHub | Download and extract the zip from the GitHub log URL |
| Dead code cleanup | Remove unused `addform/` and `watch/` packages |
