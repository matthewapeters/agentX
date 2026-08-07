package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Artifacts persists full tool output and reads it back by ref. It is satisfied
// by internal/session.Artifacts.
type Artifacts interface {
	Write(data []byte) (ref string, err error)
	Read(ref string, offset, limit int) ([]byte, error)
}

// Result is the structured outcome of a tool execution. Preview is the FULL
// captured output text (never a further-truncated excerpt of it) — the only
// thing that can shrink it below the process's real stdout is Truncated (the
// maxBytes capture safety net), and that is always called out explicitly in
// the text itself, never silently. Bounding the SIZE of a result is the tool
// call's job (narrower commands, -maxdepth, rg over cat, ...), not something
// the framework does after the fact by quietly clipping what it hands back.
type Result struct {
	ToolID    string
	Command   string // rendered argv (for audit/display)
	Exit      int
	Status    string // "ok" | "error" | "timeout"
	Stderr    string
	Bytes     int
	Lines     int
	Preview   string
	Ref       string
	Truncated bool
}

// Executor runs approved descriptors with argv/no-shell execution and persists
// full output as session artifacts.
type Executor struct {
	art      Artifacts
	maxBytes int
}

const defaultMaxBytes = 65536

// NewExecutor returns an executor writing artifacts to art. A non-positive
// maxBytes falls back to the default.
func NewExecutor(art Artifacts, maxBytes int) *Executor {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	return &Executor{art: art, maxBytes: maxBytes}
}

// Run executes a descriptor with the given (already policy-approved) args. A
// non-zero exit or a timeout is reported in the Result, not as an error; only
// infrastructure failures (bad argv template, unknown builtin) return an error.
func (e *Executor) Run(ctx context.Context, d Descriptor, args map[string]string) (Result, error) {
	if d.Builtin != "" {
		return e.runBuiltin(d, args)
	}

	argv, err := d.BuildArgv(args)
	if err != nil {
		return Result{}, err
	}
	if len(argv) == 0 {
		return Result{}, fmt.Errorf("tool %q has no argv", d.ID)
	}
	if d.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(d.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if d.StdinArg != "" {
		cmd.Stdin = strings.NewReader(args[d.StdinArg])
	}
	out := &cappedBuffer{limit: e.maxBytes}
	var errBuf bytes.Buffer
	cmd.Stdout = out
	cmd.Stderr = &errBuf
	runErr := cmd.Run()

	res := Result{
		ToolID:    d.ID,
		Command:   strings.Join(argv, " "),
		Stderr:    capString(errBuf.String(), 2048),
		Bytes:     out.written,
		Truncated: out.truncated,
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Status, res.Exit = "timeout", -1
	case runErr != nil:
		res.Status, res.Exit = "error", exitCode(runErr)
	default:
		res.Status, res.Exit = "ok", 0
	}

	data := out.buf.Bytes()
	res.Lines = countLines(data)
	res.Preview = textOf(data)
	if res.Truncated {
		res.Preview += fmt.Sprintf("\n…(capture stopped at %d bytes — output_max_bytes limit; not the tool's full output. Narrow the command, e.g. rg/-maxdepth/head, rather than assuming this is complete)", e.maxBytes)
	}
	if ref, werr := e.art.Write(data); werr == nil {
		res.Ref = ref
	}
	return res, nil
}

// runBuiltin handles Go-native tools (no subprocess).
func (e *Executor) runBuiltin(d Descriptor, args map[string]string) (Result, error) {
	res := Result{ToolID: d.ID, Status: "ok"}
	switch d.Builtin {
	case "write_file":
		path, content := args["path"], args["content"]
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return failed(res, err), nil
		}
		msg := fmt.Sprintf("wrote %d bytes to %s", len(content), path)
		res.Command = "write_file " + path
		res.Bytes, res.Lines, res.Preview = len(content), countLines([]byte(content)), msg
		if ref, err := e.art.Write([]byte(msg)); err == nil {
			res.Ref = ref
		}
		return res, nil
	case "read_output":
		ref := args["ref"]
		data, err := e.art.Read(ref, atoiOr(args["offset"]), atoiOr(args["limit"]))
		if err != nil {
			return failed(res, err), nil
		}
		res.Command = "read_output " + ref
		res.Bytes, res.Lines = len(data), countLines(data)
		res.Preview = textOf(data)
		res.Ref = ref // references the existing artifact; no new one is created
		return res, nil
	case "edit_file":
		return e.editFile(res, args)
	default:
		return Result{}, fmt.Errorf("unknown builtin %q", d.Builtin)
	}
}

// editFile replaces old_string with new_string in the file at path — a literal,
// non-regex match, required to be unique unless replace_all is set. This
// replaces edit_file's original sed-backed implementation (Command: "sed",
// Argv with a "{script}" template): a model reasoning about "put this text
// after that text" naturally produces literal text, not a sed command, and a
// wrong regex/address failed with sed's opaque exit code and no indication of
// what was actually wrong — reproducible live: a model attempting to append a
// Makefile target passed the literal block as if it were a sed script, retried
// the identical malformed command three times with no way to self-correct
// (docs/architecture — see the soft-alarming-nurse session review). Matching
// by literal substring instead removes the entire regex/escaping surface, and
// the two failure modes below (not found; not unique) give the model something
// concrete to act on instead of a bare error.
func (e *Executor) editFile(res Result, args map[string]string) (Result, error) {
	path, oldStr, newStr := args["path"], args["old_string"], args["new_string"]
	data, err := os.ReadFile(path)
	if err != nil {
		return failed(res, err), nil
	}
	if oldStr == "" {
		// strings.Count(content, "") matches at every position, never 0 — an
		// empty old_string would silently fall through to inserting new_string
		// at the very start of the file instead of hitting the "not found"
		// case below, which is never what an edit means. Reject it outright.
		return failed(res, fmt.Errorf("old_string must not be empty")), nil
	}
	content := string(data)
	count := strings.Count(content, oldStr)
	replaceAll := args["replace_all"] == "true"
	switch {
	case count == 0:
		return failed(res, fmt.Errorf("old_string not found in %s", path)), nil
	case count > 1 && !replaceAll:
		return failed(res, fmt.Errorf("old_string appears %d times in %s — make it unique, or set replace_all", count, path)), nil
	}
	updated := content
	if replaceAll {
		updated = strings.ReplaceAll(content, oldStr, newStr)
	} else {
		updated = strings.Replace(content, oldStr, newStr, 1)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return failed(res, err), nil
	}
	replaced := count
	if !replaceAll {
		replaced = 1
	}
	msg := fmt.Sprintf("replaced %d occurrence(s) in %s", replaced, path)
	res.Command = "edit_file " + path
	res.Bytes, res.Lines, res.Preview = len(updated), countLines([]byte(updated)), msg
	if ref, werr := e.art.Write([]byte(msg)); werr == nil {
		res.Ref = ref
	}
	return res, nil
}

func failed(res Result, err error) Result {
	res.Status, res.Exit = "error", 1
	res.Stderr, res.Preview = err.Error(), err.Error()
	return res
}

// cappedBuffer accumulates output up to limit bytes, flagging truncation past it.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	written   int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.limit > 0 && c.written+len(p) > c.limit {
		if allow := c.limit - c.written; allow > 0 {
			c.buf.Write(p[:allow])
			c.written += allow
		}
		c.truncated = true
		return len(p), nil // report consumed so the process is not blocked
	}
	n, err := c.buf.Write(p)
	c.written += n
	return n, err
}

func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte("\n"))
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// textOf renders captured output as context/display text. It is the FULL
// content — no line-count truncation — because bounding a result's size is
// the tool call's job (a narrower command), not something this layer imposes
// after the fact (see the Result doc comment).
func textOf(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return strings.TrimRight(string(b), "\n")
}

func capString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func atoiOr(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
