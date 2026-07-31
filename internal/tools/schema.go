package tools

import (
	"encoding/json"
	"sort"
)

// jsonSchemaProp is one property in a tool's JSON Schema parameters object.
type jsonSchemaProp struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// jsonSchemaObject is a JSON Schema object type, the shape every native
// tool-calling API expects for a tool's "parameters" field.
type jsonSchemaObject struct {
	Type       string                    `json:"type"`
	Properties map[string]jsonSchemaProp `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

// ToolSchemas returns every registered tool's native tool-calling schema, sorted
// by id for stable output. When readOnly is true, only read-risk tools are
// included — mirrors Available's existing readOnly filter.
func (r *Registry) ToolSchemas(readOnly bool) []ToolSchema {
	descs := r.Available(readOnly)
	out := make([]ToolSchema, 0, len(descs))
	for _, d := range descs {
		out = append(out, d.toolSchema())
	}
	return out
}

func (d Descriptor) toolSchema() ToolSchema {
	props := make(map[string]jsonSchemaProp, len(d.Args))
	required := make([]string, 0, len(d.Args))
	names := make([]string, 0, len(d.Args))
	for _, a := range d.Args {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	specs := make(map[string]ArgSpec, len(d.Args))
	for _, a := range d.Args {
		specs[a.Name] = a
	}
	for _, name := range names {
		a := specs[name]
		prop := jsonSchemaProp{Type: jsonSchemaType(a.Kind)}
		if a.Kind == KindPath {
			prop.Description = "filesystem path; must not start with '-' or contain a newline"
		}
		props[a.Name] = prop
		if a.Required {
			required = append(required, a.Name)
		}
	}
	schema := jsonSchemaObject{Type: "object", Properties: props, Required: required}
	raw, _ := json.Marshal(schema) // a literal struct of known fields cannot fail to marshal
	return ToolSchema{Name: d.ID, Description: d.Description, Parameters: raw}
}

func jsonSchemaType(k ArgKind) string {
	if k == KindInt {
		return "integer"
	}
	return "string"
}
