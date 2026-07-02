package cli

import (
	"fmt"
	"strings"
)

// Command is the parsed result of the agentx command line.
type Command struct {
	// ShowVersion requests version output and no runtime launch.
	ShowVersion bool
	// Launch, when non-nil, requests `agentx surface launch` (canonical or alias
	// form) instead of the default runtime + chat surface.
	Launch *LaunchArgs
	// SessionName, when set, names the booted session instead of the generated
	// adjective-noun — so scripted multiplexer layouts get predictable names.
	SessionName string
}

// Parse interprets process arguments (excluding the program name). The default
// (no recognized flags) launches the runtime + chat surface.
func Parse(args []string) (Command, error) {
	// Canonical subcommand: surface launch <kind> --session ... --connect ... --token ...
	if len(args) >= 2 && args[0] == "surface" && args[1] == "launch" {
		la, err := parseLaunch(args[2:])
		if err != nil {
			return Command{}, err
		}
		return Command{Launch: la}, nil
	}
	// Compatibility alias: -l|--launch <kind> -s|--session <s> -p|--port <port>
	if hasLaunchAlias(args) {
		la, err := parseAlias(args)
		if err != nil {
			return Command{}, err
		}
		return Command{Launch: la}, nil
	}

	var cmd Command
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--version" || a == "-v":
			cmd.ShowVersion = true
		case a == "-h" || a == "--help":
			// Help is handled by the caller; treat as no-op here.
		case a == "--session" || a == "-s":
			if i+1 >= len(args) {
				return Command{}, fmt.Errorf("%s requires a value", a)
			}
			i++
			cmd.SessionName = args[i]
		case strings.HasPrefix(a, "--session="):
			cmd.SessionName = strings.TrimPrefix(a, "--session=")
		default:
			return Command{}, fmt.Errorf("unknown argument %q", a)
		}
	}
	return cmd, nil
}

// parseLaunch reads the canonical form: a positional kind followed by
// --session/--connect/--token flags.
func parseLaunch(args []string) (*LaunchArgs, error) {
	la := &LaunchArgs{}
	i := 0
	// Optional leading positional surface kind.
	if i < len(args) && !isFlag(args[i]) {
		la.SurfaceKind = args[i]
		i++
	}
	for i < len(args) {
		a := args[i]
		val, ok := nextValue(args, &i)
		switch a {
		case "--session", "-s":
			if !ok {
				return nil, fmt.Errorf("%s requires a value", a)
			}
			la.Session = val
		case "--connect":
			if !ok {
				return nil, fmt.Errorf("%s requires a value", a)
			}
			la.Connect = val
		case "--token":
			if !ok {
				return nil, fmt.Errorf("%s requires a value", a)
			}
			la.Token = val
		default:
			return nil, fmt.Errorf("unknown surface launch argument %q", a)
		}
	}
	return la, nil
}

// parseAlias reads the compatibility alias and maps --port to a loopback endpoint.
func parseAlias(args []string) (*LaunchArgs, error) {
	la := &LaunchArgs{}
	port := ""
	for i := 0; i < len(args); {
		a := args[i]
		val, ok := nextValue(args, &i)
		switch a {
		case "--launch", "-l":
			if !ok {
				return nil, fmt.Errorf("%s requires a surface kind", a)
			}
			la.SurfaceKind = val
		case "--session", "-s":
			if !ok {
				return nil, fmt.Errorf("%s requires a value", a)
			}
			la.Session = val
		case "--port", "-p":
			if !ok {
				return nil, fmt.Errorf("%s requires a value", a)
			}
			port = val
		case "--token", "-t":
			if !ok {
				return nil, fmt.Errorf("%s requires a value", a)
			}
			la.Token = val
		default:
			return nil, fmt.Errorf("unknown launch argument %q", a)
		}
	}
	if port != "" {
		la.Connect = "http://127.0.0.1:" + port
	}
	return la, nil
}

// hasLaunchAlias reports whether args use the -l/--launch alias.
func hasLaunchAlias(args []string) bool {
	for _, a := range args {
		if a == "-l" || a == "--launch" {
			return true
		}
	}
	return false
}

func isFlag(s string) bool { return len(s) > 0 && s[0] == '-' }

// nextValue advances past the flag at *i and returns the following value when it
// is not itself a flag. *i is advanced past whatever was consumed.
func nextValue(args []string, i *int) (string, bool) {
	*i++
	if *i < len(args) && !isFlag(args[*i]) {
		v := args[*i]
		*i++
		return v, true
	}
	return "", false
}
