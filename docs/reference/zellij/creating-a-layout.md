# Zellij — Creating a Layout (vendored reference)

> **Provenance.** Extracted and normalized from
> <https://zellij.dev/documentation/creating-a-layout.html> on 2026-07-03 for
> zellij **0.44.3** (the version this project targets — see `contrib`/`ax`). This is
> a working reference for the AgentX multi-surface harness layout
> (`config/seed/agentx.kdl`), not a byte-verbatim mirror; when in doubt consult the
> upstream page. Re-fetch and bump the date if we move to a newer zellij.

## Key findings for AgentX

- **Layout files describe pane/tab *structure* only.** This page documents no way to
  embed global configuration (`keybinds`, `mouse_mode`, `default_mode`,
  `support_kitty_keyboard_protocol`) inside a layout, and **no pane-level mouse or
  keyboard attributes.** Input/keyboard/mouse behavior lives in `config.kdl`
  (separate docs), not the layout. → to ship TUI-friendly keyboard/mouse settings we
  must deliver a companion config, not bake it into the layout.
- Panes run commands via `command` + `args` (args must be in a child block).
- `focus`, `borderless`, `name`, `cwd`, `close_on_exit`, `split_direction`, `size`
  are the structural knobs we use.

## Root node

```kdl
layout {
    cwd "/path/to/directory"
    // child nodes here
}
```

## Panes

```kdl
pane                         // bare pane
pane command="htop"
pane {
    command "exa"
    cwd "/"
}
pane command="ls" {
    cwd "/"
}

pane split_direction="vertical" {   // logical container
    pane
    pane
}
```

### Command + args (args only valid in a child block)

```kdl
pane command="tail" {
    args "-f" "/path/to/my/logfile"
}
pane command="bash" {
    args "-c" "tail -f /path/to/my/logfile"
}
```

### Pane attributes

| Attribute | Type | Values | Default | Notes |
|-----------|------|--------|---------|-------|
| `split_direction` | string | `"vertical"` \| `"horizontal"` | `"horizontal"` | container panes only |
| `size` | string/int | `"50%"` \| fixed number | — | percentages recommended; fixed values unstable |
| `borderless` | boolean | `true` \| `false` | `false` | frame display |
| `focus` | boolean | `true` \| `false` | `false` | first focused pane wins |
| `name` | string | quoted string | — | custom pane title |
| `cwd` | string | absolute/relative path | — | working directory |
| `command` | string | executable | — | run instead of the shell |
| `args` | strings | `"a" "b"` | — | child-block only |
| `close_on_exit` | boolean | `true` \| `false` | `false` | close pane when command exits |
| `start_suspended` | boolean | `true` \| `false` | `false` | require ENTER to start the command |
| `edit` | string | file path | — | open file in `$EDITOR`/`$VISUAL` |
| `plugin` | location | `zellij:<name>` \| `file:/path.wasm` | — | load a plugin |
| `default_fg` / `default_bg` | string | `"#rrggbb"` \| `"rgb:rr/gg/bb"` | — | colors |
| `stacked` | boolean | `true` \| `false` | `false` | stack child panes |
| `expanded` | boolean | `true` \| `false` | `false` | expanded child in a stack (needs parent `stacked=true`) |

### Floating panes

```kdl
layout {
    floating_panes {
        pane
        pane command="ls"
        pane {
            x 1
            y "10%"
            width 200
            height "50%"
        }
    }
}
```

## Tabs

```kdl
layout {
    tab
    tab {
        pane
        pane
    }
    tab name="my third tab" split_direction="vertical" {
        pane
        pane
    }
}
```

Tab attributes: `split_direction`, `focus`, `name`, `cwd`, `hide_floating_panes`.

`cwd` composes (relative appended to container): pane → tab → global `cwd` → the cwd
where the command was executed.

```kdl
layout {
    cwd "/hi"
    tab cwd="there" {
        pane cwd="friend" // /hi/there/friend
    }
}
```

## Templates

### Pane templates

```kdl
layout {
    pane_template name="htop" {
        command "htop"
    }
    pane_template name="htop-tree" {
        command "htop"
        args "--tree"
        borderless true
    }
    htop
    htop-tree
}
```

Templates can accept `args`/`cwd` from the consumer, contain `children` (where the
consumer's panes are spliced in), and nest:

```kdl
layout {
    pane_template name="vertical-sandwich" split_direction="vertical" {
        pane
        children
        pane
    }
    vertical-sandwich {
        pane command="htop"
    }
}
```

### Tab templates / default_tab_template / new_tab_template

```kdl
layout {
    default_tab_template {
        pane size=1 borderless=true {
            plugin location="zellij:tab-bar"
        }
        children
        pane size=2 borderless=true {
            plugin location="zellij:status-bar"
        }
    }
    tab
    tab name="second tab"
}
```

`new_tab_template { ... }` is the blueprint for tabs opened during the session.
