"""Markdown rendering utilities for the AgentX output panel.

Soft-import guard: both tkinterweb and markdown are optional dependencies.
When either is unavailable the public flags (TKINTERWEB_AVAILABLE,
MARKDOWN_AVAILABLE) are False and callers must degrade gracefully to plain
tk.Text display.
"""

import re

try:
    from tkinterweb import HtmlFrame  # noqa: F401

    TKINTERWEB_AVAILABLE = True
except ImportError:
    HtmlFrame = None  # type: ignore[assignment,misc]
    TKINTERWEB_AVAILABLE = False

try:
    import markdown as _md_lib

    MARKDOWN_AVAILABLE = True
except ImportError:
    _md_lib = None  # type: ignore[assignment]
    MARKDOWN_AVAILABLE = False

# ---------------------------------------------------------------------------
# Markdown detection heuristic
# ---------------------------------------------------------------------------

MARKDOWN_PATTERNS = re.compile(
    r"(^#{1,6}\s)"  # ATX heading
    r"|(^\s*[-*+]\s)"  # unordered list item
    r"|(^\s*\d+\.\s)"  # ordered list item
    r"|(\*\*|__)"  # bold
    r"|(\*[^*\s]|_[^_\s])"  # italic (non-whitespace char after marker)
    r"|(`{1,3})"  # inline or fenced code
    r"|(\|.+\|)"  # table row
    r"|(^\s*>)"  # blockquote
    r"|(!\[.+\]\(.+\))",  # image
    re.MULTILINE,
)


def build_markdown_css(config: "GUIConfig") -> str:  # type: ignore[name-defined]  # noqa: F821
    """Return a self-contained CSS stylesheet string derived from *config* colors.

    Supports both Dark Mode and Light Mode palettes via the GUIConfig color tokens.
    """
    dark = config.theme_mode == "Dark Mode"

    # Code block backgrounds: slightly offset from the panel background.
    code_inline_bg = "#2d2d2d" if dark else "#f0f0f0"
    pre_bg = "#1e1e1e" if dark else "#f3f4f6"
    pre_border = "#4a9eff" if dark else "#1d4ed8"
    link_color = "#4a9eff" if dark else "#1d4ed8"
    tr_even_bg = "#2a2a2a" if dark else "#f8f9fa"
    blockquote_fg = config.muted_fg
    blockquote_border = "#4a9eff" if dark else "#1d4ed8"
    hr_color = config.muted_fg
    th_bg = config.status_bg

    return f"""
body {{
    background-color: {config.output_bg};
    color: {config.ui_fg};
    font-family: monospace;
    font-size: 10pt;
    margin: 8px;
    padding: 0;
    line-height: 1.5;
}}

h1, h2, h3, h4, h5, h6 {{
    color: {config.ui_fg};
    margin-top: 0.8em;
    margin-bottom: 0.4em;
}}
h1 {{ font-size: 1.5em; border-bottom: 1px solid {config.muted_fg}; padding-bottom: 0.2em; }}
h2 {{ font-size: 1.3em; border-bottom: 1px solid {config.muted_fg}; padding-bottom: 0.15em; }}
h3 {{ font-size: 1.15em; }}
h4 {{ font-size: 1.05em; }}
h5 {{ font-size: 0.95em; }}
h6 {{ font-size: 0.85em; }}

p {{ margin: 0.5em 0; }}

code {{
    background-color: {code_inline_bg};
    color: {config.agent_response_fg};
    font-family: monospace;
    font-size: 0.95em;
    padding: 1px 4px;
    border-radius: 3px;
}}

pre {{
    background-color: {pre_bg};
    border-left: 3px solid {pre_border};
    margin: 0.6em 0;
    padding: 0.6em 0.8em;
    overflow-x: auto;
}}
pre > code {{
    background-color: transparent;
    padding: 0;
    border-radius: 0;
    font-size: 0.93em;
}}

table {{
    border-collapse: collapse;
    width: 100%;
    margin: 0.6em 0;
}}
th {{
    background-color: {th_bg};
    color: {config.ui_fg};
    border: 1px solid {config.muted_fg};
    padding: 4px 8px;
    text-align: left;
}}
td {{
    border: 1px solid {config.muted_fg};
    padding: 4px 8px;
}}
tr:nth-child(even) {{
    background-color: {tr_even_bg};
}}

blockquote {{
    border-left: 3px solid {blockquote_border};
    color: {blockquote_fg};
    margin: 0.5em 0 0.5em 0.5em;
    padding: 0.2em 0.6em;
}}

a {{
    color: {link_color};
    text-decoration: none;
}}
a:hover {{
    text-decoration: underline;
}}

img {{
    max-width: 100%;
    height: auto;
}}

hr {{
    border: none;
    border-top: 1px solid {hr_color};
    margin: 0.8em 0;
}}

ul, ol {{
    margin: 0.4em 0;
    padding-left: 1.6em;
}}
li {{
    margin: 0.2em 0;
}}
""".strip()


def markdown_to_html(text: str, css: str) -> str:
    """Convert *text* (markdown) to a full HTML document with *css* embedded.

    Requires MARKDOWN_AVAILABLE to be True; callers must check before calling.
    If *_md_lib* is unavailable at runtime (e.g. in test environments where
    MARKDOWN_AVAILABLE is patched but the library is not installed) the text is
    wrapped in a ``<pre>`` block so the caller still receives valid HTML.
    Extensions used: tables, fenced_code, nl2br, sane_lists.
    """
    if _md_lib is None:
        escaped = text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
        body = f"<pre>{escaped}</pre>"
    else:
        body = _md_lib.markdown(  # type: ignore[union-attr]
            text,
            extensions=["tables", "fenced_code", "nl2br", "sane_lists"],
        )
    return f"<html><head><style>{css}</style></head><body>{body}</body></html>"


# ---------------------------------------------------------------------------
# Markdown detection heuristic
# ---------------------------------------------------------------------------


def has_markdown(text: str) -> bool:
    """Return True if *text* contains likely markdown constructs.

    This is a best-effort heuristic — false negatives leave the entry as
    plain text, which is always acceptable.
    """
    if not text:
        return False
    return MARKDOWN_PATTERNS.search(text) is not None
