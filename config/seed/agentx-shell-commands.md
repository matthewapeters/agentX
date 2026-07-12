# AgentX tools — available commands

This catalog is injected into your context when a request is routed to `single_tool`.
It tells you which tools you may call and how to call them.

## How to call a tool

Reply with EXACTLY ONE JSON object and nothing else:

{"tool": "<id>", "args": { ... }}

If none of the listed tools fits the request, reply:

{"tool": "none"}

## Rules

- Commands run **without a shell**: no pipes (`|`), redirects (`>`/`<`), globs (`*`),
  quote or variable expansion (`$VAR`), or command chaining (`;`, `&&`). Pass explicit
  arguments only; provide file content or patches inline in the JSON.
- You may call **one** tool per turn. Its result is returned to you so you can compose
  the final answer for the user.
- Every call is checked against the user's command policy and may require interactive
  approval before it runs. Prefer the least-privileged tool for the task.
- Use absolute paths, or paths relative to the session working directory.

## Available tools

### Read & search  (read-only; lowest risk)

- `read_file` — return a file's contents.
  args: `{"path": string}`
- `list_dir` — list a directory.
  args: `{"path": string}`
- `tree` — show a directory's structure (depth-limited to 3, common generated/vendored
  dirs excluded). Prefer this over repeated `list_dir` calls when you need to see a
  project's overall layout — one call, not a guess per directory.
  args: `{"path": string}`
- `find_path` — find files by name under a directory (name match only).
  args: `{"root": string, "name": string}`
- `date` — return the current date/time.
  args: none
- `git_status` — show a directory's git working-tree status.
  args: `{"path": string}`
- `read_output` — re-read a previous tool result that was stored in the session
  (large outputs are not pasted into your context in full — see Notes).
  args: `{"ref": string, "offset": int (optional), "limit": int (optional)}`

### Write & modify  (mutating; requires approval)

- `write_file` — create or overwrite a file with the given content.
  args: `{"path": string, "content": string}`
- `apply_patch` — apply a unified diff to the working tree.
  args: `{"patch": string}`
- `edit_file` — apply a single in-place substitution to a file.
  args: `{"path": string, "script": string}`   (e.g. `s/old/new/g`)

### Network  (egress; highest risk; always requires approval)

- `http_get` — fetch a URL and return the response body.
  args: `{"url": string}`
- `download` — download a URL to a local file.
  args: `{"url": string, "output": string}`

## Notes

- Arguments are schema-validated before execution; destructive or shell-escaping flags
  (e.g. `find -exec`/`-delete`, recursive force-delete) are rejected by policy.
- Output is captured (stdout, stderr, exit code) and persisted to the session.
- **You receive the full captured output**, not an excerpt of it — plus an opaque
  `ref`. Only a hard byte safety cap (`output_max_bytes`) can shrink what you see
  below the command's real output, and if that happens it is always called out
  explicitly in the result text itself. If a command might produce more than you
  need, narrow it yourself (add `-maxdepth`, pipe through `rg`/`head`, scope the
  path) rather than assuming the framework will trim it for you. Use `read_output`
  with the `ref` (and an optional line range) to re-read a specific window when
  useful.
- Tiers are enabled progressively by configuration; read-only tools are available first.
