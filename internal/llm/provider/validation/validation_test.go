package validation

import "testing"

func TestValidateInt(t *testing.T) {
	tests := []struct {
		name  string
		input string
		min   int
		max   int
		want  string // empty = nil
	}{
		{"ok", "5", 0, 100, ""},
		{"eq-min", "0", 0, 100, ""},
		{"eq-max", "100", 0, 100, ""},
		{"below-min", "-1", 0, 100, "must be ≥ 0"},
		{"above-max", "101", 0, 100, "must be ≤ 100"},
		{"empty", "", 0, 100, "is required"},
		{"non-int", "abc", 0, 100, "must be an integer"},
		{"whitespace", " 5 ", 0, 100, ""},
		{"negative-min", "-1", -5, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateInt(tc.input, tc.min, tc.max)
			if tc.want == "" {
				if got != nil {
					t.Errorf("ValidateInt(%q) = %v, want nil", tc.input, got)
				}
				return
			}
			if got == nil {
				t.Errorf("ValidateInt(%q) = nil, want error %q", tc.input, tc.want)
				return
			}
			if got.Message != tc.want {
				t.Errorf("ValidateInt(%q) message = %q, want %q", tc.input, got.Message, tc.want)
			}
		})
	}
}

func TestValidateNonEmpty(t *testing.T) {
	if got := ValidateNonEmpty("hello"); got != nil {
		t.Errorf("ValidateNonEmpty(\"hello\") = %v, want nil", got)
	}
	if got := ValidateNonEmpty("  "); got == nil || got.Message != "is required" {
		if got == nil {
			t.Error("ValidateNonEmpty(\"  \") = nil, want error")
		} else {
			t.Errorf("ValidateNonEmpty(\"  \") = %v, want 'is required'", got.Message)
		}
	}
}

func TestValidateBool(t *testing.T) {
	for _, v := range []string{"true", "false", "True", "FALSE", "  true  "} {
		if got := ValidateBool(v); got != nil {
			t.Errorf("ValidateBool(%q) = %v, want nil", v, got)
		}
	}
	if got := ValidateBool("yes"); got == nil || got.Message != "must be true or false" {
		if got == nil {
			t.Error("ValidateBool(\"yes\") = nil, want error")
		} else {
			t.Errorf("ValidateBool(\"yes\") = %v, want 'must be true or false'", got.Message)
		}
	}
}

func TestValidateEnum(t *testing.T) {
	allowed := []string{"ollama", "llamacpp"}
	if got := ValidateEnum("ollama", allowed); got != nil {
		t.Errorf("ValidateEnum(\"ollama\") = %v, want nil", got)
	}
	if got := ValidateEnum("OLLAMA", allowed); got != nil {
		t.Errorf("ValidateEnum(\"OLLAMA\") = %v, want nil", got)
	}
	if got := ValidateEnum("unknown", allowed); got == nil {
		t.Error("ValidateEnum(\"unknown\") = nil, want error")
	}
	if got := ValidateEnum("", allowed); got == nil || got.Message != "is required" {
		if got == nil {
			t.Error("ValidateEnum(\"\") = nil, want error")
		} else {
			t.Errorf("ValidateEnum(\"\") = %v, want 'is required'", got.Message)
		}
	}
}

func TestValidateColor(t *testing.T) {
	if got := ValidateColor("cyan"); got != nil {
		t.Errorf("ValidateColor(\"cyan\") = %v, want nil", got)
	}
	if got := ValidateColor("#00afaf"); got != nil {
		t.Errorf("ValidateColor(\"#00afaf\") = %v, want nil", got)
	}
	if got := ValidateColor("240"); got != nil {
		t.Errorf("ValidateColor(\"240\") = %v, want nil", got)
	}
	if got := ValidateColor("300"); got == nil || got.Message != "must be 0–255" {
		if got == nil {
			t.Error("ValidateColor(\"300\") = nil, want error")
		} else {
			t.Errorf("ValidateColor(\"300\") = %v, want 'must be 0–255'", got.Message)
		}
	}
	if got := ValidateColor(""); got == nil || got.Message != "is required" {
		if got == nil {
			t.Error("ValidateColor(\"\") = nil, want error")
		} else {
			t.Errorf("ValidateColor(\"\") = %v, want 'is required'", got.Message)
		}
	}
	if got := ValidateColor("#gggggg"); got == nil {
		t.Error("ValidateColor(\"#gggggg\") = nil, want error")
	}
}

func TestValidateHost(t *testing.T) {
	if got := ValidateHost("localhost:11434"); got != nil {
		t.Errorf("ValidateHost(\"localhost:11434\") = %v, want nil", got)
	}
	if got := ValidateHost("127.0.0.1:8080"); got != nil {
		t.Errorf("ValidateHost(\"127.0.0.1:8080\") = %v, want nil", got)
	}
	if got := ValidateHost(""); got == nil || got.Message != "is required" {
		if got == nil {
			t.Error("ValidateHost(\"\") = nil, want error")
		} else {
			t.Errorf("ValidateHost(\"\") = %v, want 'is required'", got.Message)
		}
	}
}

func TestValidateModelName(t *testing.T) {
	if got := ValidateModelName("phi4:latest"); got != nil {
		t.Errorf("ValidateModelName(\"phi4:latest\") = %v, want nil", got)
	}
	if got := ValidateModelName("nemotron-cascade-2:latest"); got != nil {
		t.Errorf("ValidateModelName(\"nemotron-cascade-2:latest\") = %v, want nil", got)
	}
	if got := ValidateModelName(""); got == nil || got.Message != "is required" {
		if got == nil {
			t.Error("ValidateModelName(\"\") = nil, want error")
		} else {
			t.Errorf("ValidateModelName(\"\") = %v, want 'is required'", got.Message)
		}
	}
	if got := ValidateModelName("model with spaces"); got == nil {
		t.Error("ValidateModelName(\"model with spaces\") = nil, want error")
	}
}
