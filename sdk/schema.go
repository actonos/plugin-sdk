package sdk

import (
	"encoding/json"
	"fmt"
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

	schemaMap := structToSchema(t)
	b, _ := json.Marshal(schemaMap)
	return json.RawMessage(b)
}

func structToSchema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
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

		fieldSchema, isRequired := parseFieldSchema(field)
		if isRequired {
			requiredFields = append(requiredFields, fieldName)
		}

		properties[fieldName] = fieldSchema
	}

	if len(requiredFields) > 0 {
		schemaMap["required"] = requiredFields
	}

	return schemaMap
}

func parseFieldSchema(field reflect.StructField) (map[string]any, bool) {
	fieldSchema := typeToSchema(field.Type)
	var isRequired bool

	// Parse `jsonschema` tag options e.g. `jsonschema:"title=Bot Token,secret,widget=password,required"`
	jsTag := field.Tag.Get("jsonschema")
	if jsTag != "" {
		options := strings.Split(jsTag, ",")
		for _, opt := range options {
			opt = strings.TrimSpace(opt)
			if opt == "required" {
				isRequired = true
			} else if opt == "secret" || opt == "secret=true" {
				fieldSchema["x-secret"] = true
			} else if strings.HasPrefix(opt, "title=") {
				fieldSchema["title"] = strings.TrimPrefix(opt, "title=")
			} else if strings.HasPrefix(opt, "description=") {
				fieldSchema["description"] = strings.TrimPrefix(opt, "description=")
			} else if strings.HasPrefix(opt, "widget=") {
				fieldSchema["x-ui-widget"] = strings.TrimPrefix(opt, "widget=")
			} else if strings.HasPrefix(opt, "group=") {
				fieldSchema["x-ui-group"] = strings.TrimPrefix(opt, "group=")
			} else if strings.HasPrefix(opt, "placeholder=") {
				fieldSchema["x-ui-placeholder"] = strings.TrimPrefix(opt, "placeholder=")
			} else if strings.HasPrefix(opt, "order=") {
				var order int
				if _, err := fmt.Sscanf(strings.TrimPrefix(opt, "order="), "%d", &order); err == nil {
					fieldSchema["x-order"] = order
				}
			} else if strings.HasPrefix(opt, "enum=") {
				enumVals := strings.Split(strings.TrimPrefix(opt, "enum="), "|")
				fieldSchema["enum"] = enumVals
			}
		}
	}

	return fieldSchema, isRequired
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
		if t.Elem().Kind() == reflect.Uint8 {
			schema["type"] = "string"
		} else {
			schema["type"] = "array"
			schema["items"] = typeToSchema(t.Elem())
		}
	case reflect.Map:
		schema["type"] = "object"
	case reflect.Interface:
		schema["type"] = "object"
	case reflect.Struct:
		return structToSchema(t)
	default:
		schema["type"] = "string"
	}

	return schema
}
