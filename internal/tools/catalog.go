package tools

// DefaultCatalog is the built-in LLM-facing tool catalog, used when no
// agentx-shell-commands.md is configured. It is kept 1:1 with
// config/seed/agentx-shell-commands.md (see that file's README).
const DefaultCatalog = `# AgentX tools — available commands

This catalog is injected into your context when a request is routed to ` + "`single_tool`" + `.
It tells you which tools you may call and how to call them.

## How to call a tool

Reply with EXACTLY ONE JSON object and nothing else:

{"tool": "<id>", "args": { ... }}

If none of the listed tools fits the request, reply:

{"tool": "none"}

## Rules

- Commands run **without a shell**: no pipes, redirects, globs, quote or variable
  expansion, or command chaining. Pass explicit arguments only; provide file content
  or patches inline in the JSON.
- You may call **one** tool per turn. Its result is returned to you so you can compose
  the final answer for the user.
- Every call is checked against the user's command policy and may require interactive
  approval before it runs. Prefer the least-privileged tool for the task.
- Use absolute paths, or paths relative to the session working directory.

## Available tools

### Read & search  (read-only; lowest risk)

- read_file — return a file's contents. args: {"path": string}
- list_dir — list a directory. args: {"path": string}
- tree — show a directory's structure (depth-limited to 3, vendored dirs excluded). args: {"path": string}
- find_path — find files by name under a directory. args: {"root": string, "name": string}
- date — return the current date/time. args: none
- git_status — show a directory's git working-tree status. args: {"path": string}
- read_output — re-read a previous tool result stored in the session.
  args: {"ref": string, "offset": int (optional), "limit": int (optional)}

### Write & modify  (mutating; requires approval)

- write_file — create or overwrite a file. args: {"path": string, "content": string}
- apply_patch — apply a unified diff. args: {"patch": string}
- edit_file — apply a single in-place substitution. args: {"path": string, "script": string}

### Network  (egress; highest risk; always requires approval)

- http_get — fetch a URL and return the body. args: {"url": string}
- download — download a URL to a file. args: {"url": string, "output": string}

## Notes

- Arguments are schema-validated; destructive or shell-escaping flags are rejected.
- Output is captured and persisted to the session; large outputs are returned as a
  preview plus a ref. Use read_output to page the full result.
`
