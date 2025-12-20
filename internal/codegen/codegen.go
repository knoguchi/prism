// Package codegen provides deterministic code generation from OpenAPI specs.
// Unlike AI-generated code, these converters produce consistent, reliable output
// that can be trusted because OpenAPI itself is validated against real traffic.
package codegen

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Generator converts OpenAPI specs to various code formats
type Generator struct {
	spec *openapi3.T
}

// NewGenerator creates a generator from an OpenAPI spec JSON/YAML string
func NewGenerator(specContent string) (*Generator, error) {
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData([]byte(specContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}
	return &Generator{spec: spec}, nil
}

// NewGeneratorFromSpec creates a generator from a parsed OpenAPI spec
func NewGeneratorFromSpec(spec *openapi3.T) *Generator {
	return &Generator{spec: spec}
}

// GenerateTypeScript generates TypeScript interfaces from the OpenAPI spec
func (g *Generator) GenerateTypeScript() (string, error) {
	return generateTypeScript(g.spec)
}

// GenerateGo generates Go struct definitions from the OpenAPI spec
func (g *Generator) GenerateGo(packageName string) (string, error) {
	return generateGo(g.spec, packageName)
}

// GenerateProtobuf generates Protocol Buffer definitions from the OpenAPI spec
func (g *Generator) GenerateProtobuf(packageName string) (string, error) {
	return generateProtobuf(g.spec, packageName)
}

// GenerateJSONSchema extracts JSON Schema from the OpenAPI spec components
func (g *Generator) GenerateJSONSchema() (string, error) {
	return generateJSONSchema(g.spec)
}

// Helper functions

// schemaToType converts an OpenAPI schema to a type name/definition
type typeInfo struct {
	name       string
	isArray    bool
	isNullable bool
	isRef      bool
}

func getSchemaType(schema *openapi3.SchemaRef) typeInfo {
	if schema == nil {
		return typeInfo{name: "any"}
	}

	// Handle $ref
	if schema.Ref != "" {
		parts := strings.Split(schema.Ref, "/")
		name := parts[len(parts)-1]
		return typeInfo{name: name, isRef: true}
	}

	s := schema.Value
	if s == nil {
		return typeInfo{name: "any"}
	}

	// Handle nullable
	nullable := s.Nullable

	// Handle arrays
	if s.Type != nil && len(*s.Type) > 0 && (*s.Type)[0] == "array" {
		itemType := getSchemaType(s.Items)
		return typeInfo{name: itemType.name, isArray: true, isNullable: nullable, isRef: itemType.isRef}
	}

	// Handle basic types
	if s.Type != nil && len(*s.Type) > 0 {
		switch (*s.Type)[0] {
		case "string":
			if s.Format == "date-time" {
				return typeInfo{name: "datetime", isNullable: nullable}
			}
			if s.Format == "date" {
				return typeInfo{name: "date", isNullable: nullable}
			}
			return typeInfo{name: "string", isNullable: nullable}
		case "integer":
			if s.Format == "int64" {
				return typeInfo{name: "int64", isNullable: nullable}
			}
			return typeInfo{name: "int32", isNullable: nullable}
		case "number":
			if s.Format == "float" {
				return typeInfo{name: "float32", isNullable: nullable}
			}
			return typeInfo{name: "float64", isNullable: nullable}
		case "boolean":
			return typeInfo{name: "boolean", isNullable: nullable}
		case "object":
			return typeInfo{name: "object", isNullable: nullable}
		}
	}

	return typeInfo{name: "any", isNullable: nullable}
}

// getSortedKeys returns sorted keys from a map for deterministic output
func getSortedSchemaKeys(schemas openapi3.Schemas) []string {
	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func getSortedPropertyKeys(props openapi3.Schemas) []string {
	return getSortedSchemaKeys(props)
}

// toPascalCase converts a string to PascalCase
func toPascalCase(s string) string {
	words := splitWords(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, "")
}

// toCamelCase converts a string to camelCase
func toCamelCase(s string) string {
	pascal := toPascalCase(s)
	if len(pascal) > 0 {
		return strings.ToLower(pascal[:1]) + pascal[1:]
	}
	return pascal
}

// toSnakeCase converts a string to snake_case
func toSnakeCase(s string) string {
	words := splitWords(s)
	for i, word := range words {
		words[i] = strings.ToLower(word)
	}
	return strings.Join(words, "_")
}

// splitWords splits a string into words (handles camelCase, PascalCase, snake_case, kebab-case)
func splitWords(s string) []string {
	// Replace common separators with spaces
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")

	// Insert space before uppercase letters (for camelCase/PascalCase)
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune(' ')
		}
		result.WriteRune(r)
	}

	// Split and filter empty strings
	words := strings.Fields(result.String())
	return words
}

// isRequired checks if a property is in the required list
func isRequired(name string, required []string) bool {
	for _, r := range required {
		if r == name {
			return true
		}
	}
	return false
}

// SanitizeOpenAPI fixes common issues in AI-generated OpenAPI specs.
// This includes:
// - Adding missing 'items' to array types
// - Ensuring required OpenAPI fields are present
func SanitizeOpenAPI(specContent string) (string, error) {
	var spec map[string]interface{}
	if err := json.Unmarshal([]byte(specContent), &spec); err != nil {
		return specContent, err // Return original if can't parse
	}

	modified := false

	// Fix schemas in components
	if components, ok := spec["components"].(map[string]interface{}); ok {
		if schemas, ok := components["schemas"].(map[string]interface{}); ok {
			for name, schemaRaw := range schemas {
				if schema, ok := schemaRaw.(map[string]interface{}); ok {
					if fixArraySchema(schema, name) {
						modified = true
					}
					// Also fix nested properties within this schema
					if fixNestedSchemas(schema) {
						modified = true
					}
				}
			}
		}
	}

	// Fix schemas in paths (inline schemas in responses/requestBody)
	if paths, ok := spec["paths"].(map[string]interface{}); ok {
		for _, pathItem := range paths {
			if pathItemMap, ok := pathItem.(map[string]interface{}); ok {
				for _, op := range pathItemMap {
					if opMap, ok := op.(map[string]interface{}); ok {
						// Fix request body schema
						if reqBody, ok := opMap["requestBody"].(map[string]interface{}); ok {
							if fixContentSchemas(reqBody) {
								modified = true
							}
						}
						// Fix response schemas
						if responses, ok := opMap["responses"].(map[string]interface{}); ok {
							for _, resp := range responses {
								if respMap, ok := resp.(map[string]interface{}); ok {
									if fixContentSchemas(respMap) {
										modified = true
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if !modified {
		return specContent, nil
	}

	// Re-serialize with proper formatting
	output, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return specContent, err
	}
	return string(output), nil
}

// fixContentSchemas fixes schemas within content/application/json structure
func fixContentSchemas(container map[string]interface{}) bool {
	modified := false
	if content, ok := container["content"].(map[string]interface{}); ok {
		for _, mediaType := range content {
			if mediaMap, ok := mediaType.(map[string]interface{}); ok {
				if schema, ok := mediaMap["schema"].(map[string]interface{}); ok {
					if fixArraySchema(schema, "inline") {
						modified = true
					}
					// Also fix nested properties
					if fixNestedSchemas(schema) {
						modified = true
					}
				}
			}
		}
	}
	return modified
}

// fixNestedSchemas recursively fixes schemas in properties and items
func fixNestedSchemas(schema map[string]interface{}) bool {
	modified := false

	// Fix properties
	if props, ok := schema["properties"].(map[string]interface{}); ok {
		for name, propRaw := range props {
			if prop, ok := propRaw.(map[string]interface{}); ok {
				if fixArraySchema(prop, name) {
					modified = true
				}
				if fixNestedSchemas(prop) {
					modified = true
				}
			}
		}
	}

	// Fix items (for arrays)
	if items, ok := schema["items"].(map[string]interface{}); ok {
		if fixArraySchema(items, "items") {
			modified = true
		}
		if fixNestedSchemas(items) {
			modified = true
		}
	}

	// Fix allOf/anyOf/oneOf
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if arr, ok := schema[key].([]interface{}); ok {
			for _, item := range arr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if fixArraySchema(itemMap, key) {
						modified = true
					}
					if fixNestedSchemas(itemMap) {
						modified = true
					}
				}
			}
		}
	}

	return modified
}

// fixArraySchema ensures array types have an items property
func fixArraySchema(schema map[string]interface{}, name string) bool {
	// Check for type: "array" (string) or type: ["array"] (array)
	isArray := false
	if schemaType, ok := schema["type"].(string); ok && schemaType == "array" {
		isArray = true
	} else if typeArr, ok := schema["type"].([]interface{}); ok {
		for _, t := range typeArr {
			if ts, ok := t.(string); ok && ts == "array" {
				isArray = true
				break
			}
		}
	}

	if !isArray {
		return false
	}

	// Check if items is missing, null, or empty object
	items, hasItems := schema["items"]
	if hasItems && items != nil {
		// Also check if items is an empty object {}
		if itemsMap, ok := items.(map[string]interface{}); ok {
			if len(itemsMap) > 0 {
				return false // Has valid items
			}
			// items is empty object - needs fixing
		} else {
			return false // Has items (probably a $ref or something)
		}
	}

	// Add default items schema
	schema["items"] = map[string]interface{}{
		"type": "object",
	}
	return true
}