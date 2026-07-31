package tools

import (
	"fmt"
	"strconv"
)

// Proposal is a resolved tool call: a tool id and its string-valued arguments.
// Produced today by native tool-calling (parsed from the model's structured
// tool_calls) or a planner-produced task.Record's pre-resolved Params; consumed
// by internal/executor and the tool policy/execution pipeline.
type Proposal struct {
	Tool string
	Args map[string]string
}

// StringifyArg normalizes a JSON-decoded arg value (string/float64/bool/nil/other) to its
// canonical string form — the executor's string-args model. Exported so any caller that
// receives typed JSON args (native tool-calling arguments, or a pre-resolved task.Record's
// Params) can convert them the same way.
func StringifyArg(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}
