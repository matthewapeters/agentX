package jsonx

import "testing"

func TestFirstObject(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                              `{"a":1}`,
		"```json\n{\"a\":1}\n```":              `{"a":1}`,       // fenced (the calm-pebble case)
		"Here you go:\n```\n{\"x\":true}\n```": `{"x":true}`,    // prose + bare fence
		`prefix {"n":{"m":2}} suffix`:          `{"n":{"m":2}}`, // nested, surrounded
		`{"s":"a}b{c"}`:                        `{"s":"a}b{c"}`, // braces inside a string literal
		`no object here`:                       ``,
	}
	for in, want := range cases {
		if got := FirstObject(in); got != want {
			t.Errorf("FirstObject(%q) = %q, want %q", in, got, want)
		}
	}
}
