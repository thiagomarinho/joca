# joca

A terminal UI for monitoring and managing CI/CD pipelines.

**Stack:** Go · [Cobra](https://github.com/spf13/cobra) · [Bubble Tea](https://github.com/charmbracelet/bubbletea) · [Lip Gloss](https://github.com/charmbracelet/lipgloss)

**Providers:**

- **GitHub Actions** — authenticates via `GITHUB_TOKEN` env var or `gh auth token`
- **AWS CodePipeline** — uses the AWS SDK v2 default credential chain; supports named profiles

---

## Requirements

- Go 1.26.6 or newer for building from source
- For GitHub pipelines: `GITHUB_TOKEN` or an authenticated `gh` CLI
- For AWS pipelines: credentials through the SDK default chain or a named profile
- For AWS SSO recovery: the AWS CLI
- Optional: `bat`, `less`, or `$PAGER` for logs; `$CODE` or a configured editor for opening log files

## Build and run

```bash
go build -o joca .
./joca

# Or run directly from source
go run .

# Run the test suite
go test ./...

# Build a versioned binary
go build \
  -ldflags "-X github.com/thiagomarinho/joca/version.Version=0.1.0 \
            -X github.com/thiagomarinho/joca/version.Commit=$(git rev-parse --short HEAD)" \
  -o joca .
```

---

## Configuration

joca stores its config at `~/.joca/config.yaml`.

```yaml
refresh_interval: 30s # minimum: 10s
default_aws_profile: production  # optional; prefills the AWS add wizard
default_aws_region: ca-central-1 # optional; prefills the AWS add wizard
log_editor: code                 # optional; otherwise uses $CODE, then code
automation_allow_chains: true    # optional; defaults to true
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

The configuration screen (`s`) edits the refresh interval, AWS defaults, and
log editor. Pipeline and automation changes made in the TUI are persisted to
the same file. Refresh intervals shorter than 10 seconds are clamped to 10
seconds.

## CLI

```bash
# Launch the TUI (`joca tui` is equivalent)
joca

# Add pipelines
joca add github owner/repo --workflow ci.yml --name "my-app CI"
joca add aws pipeline-name --profile production --region us-east-1

# Manage automation rules
joca automation list
joca automation add --watch build --on success --trigger deploy --once --name deploy-after-build
joca automation add --watch tests --on failed --trigger notify --times 3
joca automation disable deploy-after-build
joca automation enable deploy-after-build
joca automation reset deploy-after-build
joca automation delete deploy-after-build

joca version
```

When adding an AWS pipeline in the TUI, joca suggests profiles and regions
from the standard AWS shared config and credentials files. Use `↑`/`↓` to
choose a suggestion and `→` to complete it.

Automation rules are evaluated on status transitions observed while joca is
running. Supported transition values are `success`, `failed`, and `cancelled`.
Rules can fire indefinitely, once, or a fixed number of times. Multiple
`--trigger` flags create one rule per target. Self-triggering is always
rejected; set `automation_allow_chains: false` to reject dependency cycles too.

---

## TUI Keybindings

### Pipeline list

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `Page Up` / `Page Down` | Move by one visible page |
| `Home` / `g` | Jump to the first pipeline |
| `End` / `G` | Jump to the last pipeline |
| `/` | Fuzzy-search pipelines; `Enter` keeps the filter and `Esc` clears it |
| `Enter` | Open pipeline details |
| `o` | Open the selected pipeline or execution in a browser |
| `Space` | Pause / resume auto-refresh for pipeline |
| `f` | Focus selected pipeline: resume it, pause all others, and hide paused pipelines |
| `h` | Hide / show paused pipelines |
| `Shift+↑` / `Shift+↓` | Reorder the selected pipeline |
| `r` | Refresh all now |
| `R` | Re-run latest execution (GitHub) / start new execution (AWS) |
| `N` | Start a new run via workflow dispatch (GitHub only) |
| `p` | Pause / resume all automatic refreshes |
| `l` | Refresh expired AWS SSO credentials when the action is shown |
| `a` | Add pipeline (inline form) |
| `c` | Copy the selected pipeline and customize its settings |
| `d` | Delete the selected pipeline after confirmation |
| `s` | Open application configuration |
| `A` | Manage pipeline automation rules |
| `t` | Toggle session API-call tracking |
| `T` | Open telemetry while tracking is enabled |
| `q` / `Ctrl+C` | Quit from the pipeline list |

### Pipeline details and logs

| Key | Action |
|-----|--------|
| `↑` / `↓` | Select a recent execution or CodeBuild log source |
| `o` | Open the selected execution in a browser |
| `l` | Load selected logs; CodeBuild logs are downloaded and opened in a pager |
| `e` | Download and open the selected CodeBuild logs in an editor |
| `/` | Search CodeBuild logs across recent AWS executions |
| `Esc` / `q` | Return to the previous screen |

Log-search results are grouped by pipeline execution, with every matching
CodeBuild project/action shown as an individually selectable row. Searches are
literal and case-sensitive. Choose a depth from 1–100 executions; progress and
partial failures are reported while the search runs. Closing a pager returns
to the same results screen. GUI editor launchers return immediately, allowing
multiple matching logs to be opened.

### Automation and telemetry screens

- Automation rules: `n`/`a` adds, `d` deletes, `e` enables/disables, and `r`
  resets the selected rule.
- Telemetry: `c` clears session data and `t` toggles tracking.
- `Esc` or `q` returns to the previous screen.

## Provider notes

### AWS CodePipeline and CodeBuild

For AWS CodePipeline executions, log viewing currently supports CodeBuild
actions backed by CloudWatch Logs. If an execution contains multiple CodeBuild
actions, select the stage/action whose logs you want. Logs are written with
private permissions to a temporary `joca-logs-*` or `joca-log-search-*`
directory. They are opened with `$PAGER`, `bat`, or `less`; the built-in viewer
is used when none is available.
Press `e` instead to open downloaded logs in a code editor. The editor command
is selected from the `log_editor` application setting, then `$CODE`, then
`code`. Configure it from the in-app configuration screen; commands may include
arguments. GUI editor launchers return immediately so you can open multiple
logs; add `--wait` explicitly if you prefer blocking behavior.

Depending on the features used, the AWS identity needs:

- `codepipeline:GetPipelineState` and `codepipeline:ListPipelineExecutions` for monitoring
- `codepipeline:StartPipelineExecution` to start runs
- `codepipeline:ListPipelines` for pipeline discovery in the add wizard
- `codepipeline:ListActionExecutions`, `codebuild:BatchGetBuilds`, and
  `logs:GetLogEvents` for CodeBuild log viewing and search

Expired AWS SSO sessions are identified in the dashboard. Press `l` when the
SSO login action appears to run `aws sso login` for the affected profile.

### GitHub Actions

`GITHUB_TOKEN` takes precedence over the token returned by `gh auth token`.
Starting a new run requires the configured workflow to support
`workflow_dispatch`. GitHub currently returns log archives through a redirect;
joca shows that download URL rather than extracting and displaying the archive.

---

## Status Colors

| Status | Color | Description |
|--------|-------|-------------|
| `running` | 🟡 Yellow | Execution in progress |
| `success` | 🟢 Green | Completed successfully |
| `failed` | 🔴 Red | Completed with failure |
| `approval` | 🟣 Purple | Waiting for manual approval |
| `pending` | 🔵 Cyan | Queued, not yet started |
| `cancelled` | ⚫ Grey | Execution was cancelled |
| `idle` | ⚫ Grey | No recent executions |
| `unknown` | ⚫ Grey | Status could not be determined |

---

## Roadmap

### Recently shipped

| Feature | Notes |
|---------|-------|
| Pipeline search and long-list navigation | Fuzzy filtering, paging, first/last navigation, and hide/focus controls |
| AWS CodeBuild logs | Discover logs by execution and action, open them in a pager or configurable editor, and search across recent executions |
| AWS SSO recovery | Detect expired SSO sessions and launch login for the affected profile without leaving joca |
| Pipeline management | Add, copy, reorder, pause, focus, and delete pipelines from the TUI |
| Application configuration | Edit refresh timing, AWS defaults, and the log editor from within the app |
| Pipeline automations | Trigger pipelines from status transitions, with multi-target rules and chain detection |

### Planned

| Feature | Notes |
|---------|-------|
| AWS approval actions | Approve or reject manual approval steps from the TUI |
| Real GitHub log content | Download and extract GitHub Actions log archives instead of showing the download URL |
| Dead code cleanup | Remove the unused `addform/` and `watch/` packages |

The planned list is directional and is not ordered by release date.
