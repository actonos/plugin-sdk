package sdk

import (
	"encoding/json"
	"reflect"
	"strings"
)

// GenerateSchema generates a JSON Schema from a Go struct definition.
func GenerateSchema(v any) json.RawMessage {
	if v == nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}

	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}

	schemaMap := map[string]any{
		"type":       "object",
		"properties": make(map[string]any),
	}

	var requiredFields []string
	properties := schemaMap["properties"].(map[string]any)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		fieldName := field.Name
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				fieldName = parts[0]
			}
		}

		fieldSchema := typeToSchema(field.Type)

		// Parse `jsonschema` tag options e.g. `jsonschema:"description=City name,required"`
		jsTag := field.Tag.Get("jsonschema")
		if jsTag != "" {
			options := strings.Split(jsTag, ",")
			for _, opt := range options {
				opt = strings.TrimSpace(opt)
				if opt == "required" {
					requiredFields = append(requiredFields, fieldName)
				} else if strings.HasPrefix(opt, "description=") {
					desc := strings.TrimPrefix(opt, "description=")
					fieldSchema["description"] = desc
				} else if strings.HasPrefix(opt, "enum=") {
					enumVals := strings.Split(strings.TrimPrefix(opt, "enum="), "|")
					fieldSchema["enum"] = enumVals
				}
			}
		}

		properties[fieldName] = fieldSchema
	}

	if len(requiredFields) > 0 {
		schemaMap["required"] = requiredFields
	}

	b, _ := json.Marshal(schemaMap)
	return json.RawMessage(b)
}

func typeToSchema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	schema := make(map[string]any)

	switch t.Kind() {
	case reflect.String:
		schema["type"] = "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema["type"] = "integer"
	case reflect.Float32, reflect.Float64:
		schema["type"] = "number"
	case reflect.Bool:
		schema["type"] = "boolean"
	case reflect.Slice, reflect.Array:
		schema["type"] = "array"
		schema["items"] = typeToSchema(t.Elem())
	case reflect.Struct:
		schema["type"] = "object"
		subProps := make(map[string]any)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			fName := f.Name
			jTag := f.Tag.Get("json")
			if jTag != "" && jTag != "-" {
				parts := strings.Split(jTag, ",")
				if parts[0] != "" {
					fName = parts[0]
				}
			}
			subProps[fName] = typeToSchema(f.Type)
		}
		schema["properties"] = subProps
	default:
		schema["type"] = "string"
	}

	return schema
}
