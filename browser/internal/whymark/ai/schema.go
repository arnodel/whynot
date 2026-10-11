package ai

import (
	"errors"
	"fmt"
	"math"
	"sort"

	rt "github.com/arnodel/golua/runtime"
)

// maxDepth bounds how deeply shapes nest, which also catches shapes that
// contain themselves.
const maxDepth = 100

// schema turns the shape of an ai{...} call's returns into a JSON
// schema.
func schema(shape rt.Value, depth int) (map[string]any, error) {
	if depth > maxDepth {
		return nil, errors.New("shapes nested too deeply, or containing themselves")
	}
	if s, ok := shape.TryString(); ok {
		switch s {
		case "string", "number", "integer", "boolean":
			return map[string]any{"type": s}, nil
		}
		return nil, fmt.Errorf("unknown type %q: want string, number, integer or boolean", s)
	}
	t, ok := shape.TryTable()
	if !ok {
		return nil, fmt.Errorf("a shape is a type name or a table, not a %s", shape.TypeName())
	}
	if choices, ok := t.Get(rt.StringValue("one_of")).TryTable(); ok {
		var values []string
		for i := int64(1); i <= choices.Len(); i++ {
			v, ok := choices.Get(rt.IntValue(i)).TryString()
			if !ok {
				return nil, errors.New("one_of's choices are strings")
			}
			values = append(values, v)
		}
		if len(values) == 0 {
			return nil, errors.New("one_of needs choices")
		}
		for k, _, _ := t.Next(rt.NilValue); !k.IsNil(); k, _, _ = t.Next(k) {
			if name, _ := k.TryString(); name != "one_of" {
				return nil, errors.New("a one_of shape has nothing else")
			}
		}
		return map[string]any{"type": "string", "enum": values}, nil
	}
	if first := t.Get(rt.IntValue(1)); !first.IsNil() {
		if k, _, _ := t.Next(rt.IntValue(1)); !k.IsNil() || t.Len() != 1 {
			return nil, errors.New("a list's shape is a table with just one shape, for its items")
		}
		items, err := schema(first, depth+1)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": items}, nil
	}
	properties := map[string]any{}
	for k, v, _ := t.Next(rt.NilValue); !k.IsNil(); k, v, _ = t.Next(k) {
		name, ok := k.TryString()
		if !ok {
			return nil, errors.New("an object's fields are named by strings")
		}
		schema, err := schema(v, depth+1)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		properties[name] = schema
	}
	if len(properties) == 0 {
		return nil, errors.New("an object needs at least one field")
	}
	return objectSchema(properties), nil
}

// objectSchema returns the schema of an object with all of properties.
func objectSchema(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for name := range properties {
		required = append(required, name)
	}
	sort.Strings(required)
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

// jsonToLua turns a decoded JSON value into a Lua value: objects and
// arrays into tables, and whole numbers into integers.
func jsonToLua(r *rt.Runtime, v any) rt.Value {
	set := func(t *rt.Table, k, v rt.Value) {
		r.RequireCPU(1)
		r.RequireMem(t.Set(k, v))
	}
	switch v := v.(type) {
	case string:
		r.RequireBytes(len(v))
		return rt.StringValue(v)
	case bool:
		return rt.BoolValue(v)
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1<<53 {
			return rt.IntValue(int64(v))
		}
		return rt.FloatValue(v)
	case []any:
		t := rt.NewTable()
		for i, item := range v {
			set(t, rt.IntValue(int64(i+1)), jsonToLua(r, item))
		}
		return rt.TableValue(t)
	case map[string]any:
		t := rt.NewTable()
		for k, item := range v {
			r.RequireBytes(len(k))
			set(t, rt.StringValue(k), jsonToLua(r, item))
		}
		return rt.TableValue(t)
	}
	return rt.NilValue
}
