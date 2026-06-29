package cli

import "fmt"

// Command is the parsed result of the agentx command line.
type Command struct {
	// ShowVersion requests version output and no runtime launch.
	ShowVersion bool
}

// Parse interprets process arguments (excluding the program name). The default
// (no recognized flags) launches the runtime + chat surface.
func Parse(args []string) (Command, error) {
	var cmd Command
	for _, a := range args {
		switch a {
		case "--version", "-v":
			cmd.ShowVersion = true
		case "-h", "--help":
			// Help is handled by the caller; treat as no-op here.
		default:
			return Command{}, fmt.Errorf("unknown argument %q", a)
		}
	}
	return cmd, nil
}
