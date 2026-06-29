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
| `agentx-shell-commands.md` | `~/.config/agentx/agentx-shell-commands.md` | `tools.DefaultCatalog` (planned, M3b) | uses the constant |
| `agentx.toml` | `~/.config/agentx/agentx.toml` | `config.Default()` | seeded on first run |

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
