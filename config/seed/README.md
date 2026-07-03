# Configuration seed templates

Baseline copies of the files AgentX reads from `~/.config/agentx/`. They capture
the built-in code defaults **1:1** so a deployment can seed a fresh config that is
identical to the runtime fallbacks, giving a common baseline from which to tune.
(Packaging/deployment that copies these into place is future work.)

| Seed file | Installs as | Code source of truth | Runtime fallback when absent |
|-----------|-------------|----------------------|------------------------------|
| `agentx-instructions.md` | `~/.config/agentx/agentx-instructions.md` | `prompting.DefaultSystemPrompt` | uses the constant |
| `agentx-thinking.md` | `~/.config/agentx/agentx-thinking.md` | `prompting.DefaultThinkingPrompt` | uses the constant |
| `agentx-classification.md` | `~/.config/agentx/agentx-classification.md` | `classify.DefaultPrompt` | uses the constant |
| `bootstrap-prompt.md` | `~/.config/agentx/bootstrap-prompt.md` | none — seed *is* the baseline | startup auto-submit is skipped |
| `agentx-shell-commands.md` | `~/.config/agentx/agentx-shell-commands.md` | `tools.DefaultCatalog` | uses the constant |
| `agentx-tool-blacklist.toml` | `~/.config/agentx/agentx-tool-blacklist.toml` | none — seed *is* the baseline | no blacklist rules |
| `agentx.toml` | `~/.config/agentx/agentx.toml` | `config.Default()` | seeded on first run |
| `agentx.kdl` | `~/.config/agentx/agentx.kdl` | none — harness artifact | not read by AgentX (see note) |

`agentx.kdl` is the odd one out: it is **not read by the AgentX runtime** at all. It
is a [zellij](https://zellij.dev) layout consumed by the `ax` dev launcher to open
the multi-surface harness (chat + context + context-visualizer + working-memory) in
one window. It ships here so it is deployed alongside the other defaults, but it has
no code source of truth and no runtime fallback — if it is absent, only `ax` is
affected, not `agentx`. Keyboard/mouse tuning for the harness lives in zellij's
`config.kdl` (layouts cannot embed it; see `docs/reference/zellij/creating-a-layout.md`).

The command-policy **global approvals** file
(`~/.config/agentx/agentx-tool-approvals.toml`) is runtime-managed — written when you
approve a command "globally" and reloaded next session — so it has no seed template.

`bootstrap-prompt.md` is the one prompt with no built-in constant: when absent the
startup auto-submit is simply skipped, so this seed file *is* its baseline rather
than a 1:1 mirror of code. It is read verbatim like the other prompt files.

## Important: prompt files are read verbatim

Every `*.md` file is loaded raw (trimmed) and becomes part of the model's context
verbatim — there is no out-of-band commentary. So:

- `agentx-instructions.md`, `agentx-thinking.md`, `agentx-classification.md`, and
  `bootstrap-prompt.md` contain **only the prompt text** (no markdown headings or
  comments) — anything added becomes part of the prompt sent to the model.
- `agentx-shell-commands.md` is the exception by design: it is a **tool catalog**
  written *for* the model, so its markdown structure (headings, the JSON call
  contract) is intentional context, injected only when a turn routes to `single_tool`.

Only `agentx.toml` (parsed as TOML) carries explanatory comments.

If you change a default in code, update the matching seed file here so they stay 1:1.
