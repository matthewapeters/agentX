package toolsteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/session"
	"agentx/internal/tools"
)

type execWorld struct {
	dir  string
	art  *session.Artifacts
	exec *tools.Executor
	reg  *tools.Registry
	res  tools.Result

	// artifact-only world state
	artRef   string
	artLines int
	file     string
}

func registerExecSteps(sc *godog.ScenarioContext) {
	w := &execWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = execWorld{}
		return ctx, nil
	})

	// Artifact store (@unit)
	sc.Step(`^an artifact store$`, w.newArtifactStore)
	sc.Step(`^an artifact of (\d+) numbered lines is written$`, w.writeNumberedArtifact)
	sc.Step(`^the artifact has (\d+) lines$`, w.artifactHasLines)
	sc.Step(`^reading the artifact at offset (\d+) limit (\d+) yields "([^"]*)"$`, w.readArtifactYields)

	// Executor (@integration)
	sc.Step(`^a tool executor$`, w.newExecutor)
	sc.Step(`^a tool executor with a (\d+) byte output cap$`, w.newExecutorWithByteCap)
	sc.Step(`^a file "([^"]*)" containing "([^"]*)"$`, w.createFile)
	sc.Step(`^the tool "([^"]*)" runs on that file$`, w.runOnFile)
	sc.Step(`^the tool "([^"]*)" runs on a missing file$`, w.runOnMissing)
	sc.Step(`^a command sleeps for (\d+) seconds with a (\d+) second timeout$`, w.runSleep)
	sc.Step(`^a command echoes the literal "([^"]*)"$`, w.runEcho)
	sc.Step(`^a command emits (\d+) numbered lines$`, w.runSeq)
	sc.Step(`^the result status is "([^"]*)"$`, w.statusIs)
	sc.Step(`^the result exit code is (\d+)$`, w.exitIs)
	sc.Step(`^the result exit code is not 0$`, w.exitNonZero)
	sc.Step(`^the result preview contains "([^"]*)"$`, w.previewContains)
	sc.Step(`^the result preview does not contain "([^"]*)"$`, w.previewNotContains)
	sc.Step(`^the result reports (\d+) lines$`, w.reportsLines)
	sc.Step(`^the result is marked truncated$`, w.resultIsTruncated)
	sc.Step(`^the result has an artifact ref$`, w.hasRef)
	sc.Step(`^reading the result output yields "([^"]*)"$`, w.readOutputYields)
	sc.Step(`^reading the result output at offset (\d+) limit (\d+) yields "([^"]*)"$`, w.readOutputRangeYields)
}

// --- shared setup ---

func (w *execWorld) setup() error {
	dir, err := os.MkdirTemp("", "agentx-tool-")
	if err != nil {
		return err
	}
	w.dir = dir
	store := session.NewStore(filepath.Join(dir, "sessions"))
	id, err := store.Create()
	if err != nil {
		return err
	}
	art, err := store.Artifacts(id.ID)
	if err != nil {
		return err
	}
	w.art = art
	w.exec = tools.NewExecutor(art, 0)
	w.reg = tools.DefaultRegistry()
	return nil
}

// newExecutorWithByteCap builds an executor with a small maxBytes so capture
// truncation (the one remaining, honestly-labeled safety net) is observable.
func (w *execWorld) newExecutorWithByteCap(n int) error {
	if err := w.setup(); err != nil {
		return err
	}
	w.exec = tools.NewExecutor(w.art, n)
	return nil
}

// --- artifact store steps ---

func (w *execWorld) newArtifactStore() error { return w.setup() }

func (w *execWorld) writeNumberedArtifact(n int) error {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%03d", i+1)
	}
	ref, err := w.art.Write([]byte(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		return err
	}
	w.artRef = ref
	w.artLines = n
	return nil
}

func (w *execWorld) artifactHasLines(want int) error {
	data, err := w.art.Read(w.artRef, 0, 0)
	if err != nil {
		return err
	}
	if got := countLines(data); got != want {
		return fmt.Errorf("artifact has %d lines, want %d", got, want)
	}
	return nil
}

func (w *execWorld) readArtifactYields(offset, limit int, want string) error {
	data, err := w.art.Read(w.artRef, offset, limit)
	if err != nil {
		return err
	}
	if got := firstLine(string(data)); got != want {
		return fmt.Errorf("artifact window first line = %q, want %q", got, want)
	}
	return nil
}

// --- executor steps ---

func (w *execWorld) newExecutor() error { return w.setup() }

func (w *execWorld) createFile(name, content string) error {
	w.file = filepath.Join(w.dir, name)
	return os.WriteFile(w.file, []byte(content), 0o644)
}

func (w *execWorld) run(d tools.Descriptor, args map[string]string) error {
	res, err := w.exec.Run(context.Background(), d, args)
	if err != nil {
		return err
	}
	w.res = res
	return nil
}

func (w *execWorld) runOnFile(id string) error {
	d, ok := w.reg.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	return w.run(d, map[string]string{"path": w.file})
}

func (w *execWorld) runOnMissing(id string) error {
	d, ok := w.reg.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	return w.run(d, map[string]string{"path": filepath.Join(w.dir, "does-not-exist")})
}

func (w *execWorld) runSleep(seconds, timeout int) error {
	d := tools.Descriptor{
		ID: "sleep", Command: "sleep", Argv: []string{"sleep", "{n}"},
		TimeoutSeconds: timeout,
		Args:           []tools.ArgSpec{{Name: "n", Kind: tools.KindString, Required: true}},
	}
	return w.run(d, map[string]string{"n": strconv.Itoa(seconds)})
}

func (w *execWorld) runEcho(literal string) error {
	d := tools.Descriptor{
		ID: "echo", Command: "echo", Argv: []string{"echo", "{v}"},
		TimeoutSeconds: 30,
		Args:           []tools.ArgSpec{{Name: "v", Kind: tools.KindString, Required: true}},
	}
	return w.run(d, map[string]string{"v": literal})
}

func (w *execWorld) runSeq(n int) error {
	d := tools.Descriptor{
		ID: "seq", Command: "seq", Argv: []string{"seq", "1", "{n}"},
		TimeoutSeconds: 30,
		Args:           []tools.ArgSpec{{Name: "n", Kind: tools.KindString, Required: true}},
	}
	return w.run(d, map[string]string{"n": strconv.Itoa(n)})
}

func (w *execWorld) statusIs(want string) error {
	if w.res.Status != want {
		return fmt.Errorf("status = %q (stderr=%q), want %q", w.res.Status, w.res.Stderr, want)
	}
	return nil
}

func (w *execWorld) exitIs(want int) error {
	if w.res.Exit != want {
		return fmt.Errorf("exit = %d, want %d", w.res.Exit, want)
	}
	return nil
}

func (w *execWorld) exitNonZero() error {
	if w.res.Exit == 0 {
		return fmt.Errorf("exit = 0, want non-zero")
	}
	return nil
}

func (w *execWorld) previewContains(want string) error {
	if !strings.Contains(w.res.Preview, want) {
		return fmt.Errorf("preview %q does not contain %q", w.res.Preview, want)
	}
	return nil
}

func (w *execWorld) previewNotContains(unwanted string) error {
	if strings.Contains(w.res.Preview, unwanted) {
		return fmt.Errorf("preview %q unexpectedly contains %q", w.res.Preview, unwanted)
	}
	return nil
}

func (w *execWorld) reportsLines(want int) error {
	if w.res.Lines != want {
		return fmt.Errorf("result reports %d lines, want %d", w.res.Lines, want)
	}
	return nil
}

func (w *execWorld) resultIsTruncated() error {
	if !w.res.Truncated {
		return fmt.Errorf("result not marked truncated")
	}
	return nil
}

func (w *execWorld) hasRef() error {
	if w.res.Ref == "" {
		return fmt.Errorf("result has no artifact ref")
	}
	return nil
}

func (w *execWorld) readOutput(ref string, offset, limit int) (string, error) {
	d, _ := w.reg.Lookup("read_output")
	res, err := w.exec.Run(context.Background(), d, map[string]string{
		"ref": ref, "offset": strconv.Itoa(offset), "limit": strconv.Itoa(limit),
	})
	return res.Preview, err
}

func (w *execWorld) readOutputYields(want string) error {
	out, err := w.readOutput(w.res.Ref, 0, 0)
	if err != nil {
		return err
	}
	if !strings.Contains(out, want) {
		return fmt.Errorf("read_output %q does not contain %q", out, want)
	}
	return nil
}

func (w *execWorld) readOutputRangeYields(offset, limit int, want string) error {
	out, err := w.readOutput(w.res.Ref, offset, limit)
	if err != nil {
		return err
	}
	if got := firstLine(out); got != want {
		return fmt.Errorf("read_output window first line = %q, want %q", got, want)
	}
	return nil
}

// --- helpers ---

func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := strings.Count(string(b), "\n")
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
