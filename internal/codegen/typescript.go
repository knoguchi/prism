package codegen

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// generateTypeScript generates TypeScript interfaces from OpenAPI spec
func generateTypeScript(spec *openapi3.T) (string, error) {
	var sb strings.Builder

	sb.WriteString("// Auto-generated TypeScript types from OpenAPI spec\n")
	sb.WriteString("// Do not edit manually\n\n")

	// Generate interfaces for each schema in components
	if spec.Components != nil && spec.Components.Schemas != nil {
		for _, name := range getSortedSchemaKeys(spec.Components.Schemas) {
			schemaRef := spec.Components.Schemas[name]
			if err := generateTSInterface(&sb, name, schemaRef); err != nil {
				return "", fmt.Errorf("failed to generate interface %s: %w", name, err)
			}
			sb.WriteString("\n")
		}
	}

	// Generate request/response types from paths
	if spec.Paths != nil {
		if err := generateTSPathTypes(&sb, spec.Paths); err != nil {
			return "", err
		}
	}

	return sb.String(), nil
}

func generateTSInterface(sb *strings.Builder, name string, schemaRef *openapi3.SchemaRef) error {
	if schemaRef == nil || schemaRef.Value == nil {
		return nil
	}

	schema := schemaRef.Value

	// Handle enums
	if len(schema.Enum) > 0 {
		sb.WriteString(fmt.Sprintf("export type %s = ", name))
		var enumValues []string
		for _, v := range schema.Enum {
			if s, ok := v.(string); ok {
				enumValues = append(enumValues, fmt.Sprintf("'%s'", s))
			} else {
				enumValues = append(enumValues, fmt.Sprintf("%v", v))
			}
		}
		sb.WriteString(strings.Join(enumValues, " | "))
		sb.WriteString(";\n")
		return nil
	}

	// Handle object types
	if schema.Type != nil && len(*schema.Type) > 0 && (*schema.Type)[0] == "object" {
		sb.WriteString(fmt.Sprintf("export interface %s {\n", name))

		for _, propName := range getSortedPropertyKeys(schema.Properties) {
			propSchema := schema.Properties[propName]
			tsType := schemaToTSType(propSchema)
			optional := ""
			if !isRequired(propName, schema.Required) {
				optional = "?"
			}
			sb.WriteString(fmt.Sprintf("  %s%s: %s;\n", propName, optional, tsType))
		}

		// Handle additionalProperties
		if schema.AdditionalProperties.Has != nil && *schema.AdditionalProperties.Has {
			if schema.AdditionalProperties.Schema != nil {
				valueType := schemaToTSType(schema.AdditionalProperties.Schema)
				sb.WriteString(fmt.Sprintf("  [key: string]: %s;\n", valueType))
			} else {
				sb.WriteString("  [key: string]: unknown;\n")
			}
		}

		sb.WriteString("}\n")
		return nil
	}

	// Handle array types as type aliases
	if schema.Type != nil && len(*schema.Type) > 0 && (*schema.Type)[0] == "array" {
		itemType := schemaToTSType(schema.Items)
		sb.WriteString(fmt.Sprintf("export type %s = %s[];\n", name, itemType))
		return nil
	}

	// Handle primitive type aliases
	tsType := schemaToTSType(schemaRef)
	sb.WriteString(fmt.Sprintf("export type %s = %s;\n", name, tsType))
	return nil
}

func schemaToTSType(schemaRef *openapi3.SchemaRef) string {
	if schemaRef == nil {
		return "unknown"
	}

	// Handle $ref
	if schemaRef.Ref != "" {
		parts := strings.Split(schemaRef.Ref, "/")
		return parts[len(parts)-1]
	}

	schema := schemaRef.Value
	if schema == nil {
		return "unknown"
	}

	// Handle oneOf/anyOf
	if len(schema.OneOf) > 0 {
		var types []string
		for _, s := range schema.OneOf {
			types = append(types, schemaToTSType(s))
		}
		return strings.Join(types, " | ")
	}
	if len(schema.AnyOf) > 0 {
		var types []string
		for _, s := range schema.AnyOf {
			types = append(types, schemaToTSType(s))
		}
		return strings.Join(types, " | ")
	}

	// Handle allOf (intersection type)
	if len(schema.AllOf) > 0 {
		var types []string
		for _, s := range schema.AllOf {
			types = append(types, schemaToTSType(s))
		}
		return strings.Join(types, " & ")
	}

	if schema.Type == nil || len(*schema.Type) == 0 {
		return "unknown"
	}

	baseType := (*schema.Type)[0]

	switch baseType {
	case "string":
		if len(schema.Enum) > 0 {
			var enumValues []string
			for _, v := range schema.Enum {
				if s, ok := v.(string); ok {
					enumValues = append(enumValues, fmt.Sprintf("'%s'", s))
				}
			}
			return strings.Join(enumValues, " | ")
		}
		return "string"

	case "integer", "number":
		return "number"

	case "boolean":
		return "boolean"

	case "array":
		if schema.Items != nil {
			itemType := schemaToTSType(schema.Items)
			return itemType + "[]"
		}
		return "unknown[]"

	case "object":
		if len(schema.Properties) == 0 {
			if schema.AdditionalProperties.Schema != nil {
				valueType := schemaToTSType(schema.AdditionalProperties.Schema)
				return fmt.Sprintf("Record<string, %s>", valueType)
			}
			return "Record<string, unknown>"
		}
		// Inline object type
		var props []string
		for _, propName := range getSortedPropertyKeys(schema.Properties) {
			propSchema := schema.Properties[propName]
			propType := schemaToTSType(propSchema)
			optional := ""
			if !isRequired(propName, schema.Required) {
				optional = "?"
			}
			props = append(props, fmt.Sprintf("%s%s: %s", propName, optional, propType))
		}
		return "{ " + strings.Join(props, "; ") + " }"

	case "null":
		return "null"
	}

	return "unknown"
}

func generateTSPathTypes(sb *strings.Builder, paths *openapi3.Paths) error {
	sb.WriteString("// API endpoint types\n\n")

	// Collect all operations
	type opInfo struct {
		method string
		path   string
		op     *openapi3.Operation
	}
	var ops []opInfo

	for path, pathItem := range paths.Map() {
		if pathItem.Get != nil {
			ops = append(ops, opInfo{"Get", path, pathItem.Get})
		}
		if pathItem.Post != nil {
			ops = append(ops, opInfo{"Post", path, pathItem.Post})
		}
		if pathItem.Put != nil {
			ops = append(ops, opInfo{"Put", path, pathItem.Put})
		}
		if pathItem.Delete != nil {
			ops = append(ops, opInfo{"Delete", path, pathItem.Delete})
		}
		if pathItem.Patch != nil {
			ops = append(ops, opInfo{"Patch", path, pathItem.Patch})
		}
	}

	for _, op := range ops {
		typeName := generateOperationTypeName(op.method, op.path)

		// Generate request params type
		if len(op.op.Parameters) > 0 {
			sb.WriteString(fmt.Sprintf("export interface %sParams {\n", typeName))
			for _, param := range op.op.Parameters {
				if param.Value != nil {
					paramType := "string"
					if param.Value.Schema != nil {
						paramType = schemaToTSType(param.Value.Schema)
					}
					optional := ""
					if !param.Value.Required {
						optional = "?"
					}
					sb.WriteString(fmt.Sprintf("  %s%s: %s;\n", param.Value.Name, optional, paramType))
				}
			}
			sb.WriteString("}\n\n")
		}

		// Generate request body type
		if op.op.RequestBody != nil && op.op.RequestBody.Value != nil {
			for _, content := range op.op.RequestBody.Value.Content {
				if content.Schema != nil {
					bodyType := schemaToTSType(content.Schema)
					sb.WriteString(fmt.Sprintf("export type %sRequest = %s;\n\n", typeName, bodyType))
					break
				}
			}
		}

		// Generate response type (use 200/201 response)
		if op.op.Responses != nil {
			for _, status := range []string{"200", "201"} {
				if resp := op.op.Responses.Value(status); resp != nil && resp.Value != nil {
					for _, content := range resp.Value.Content {
						if content.Schema != nil {
							respType := schemaToTSType(content.Schema)
							sb.WriteString(fmt.Sprintf("export type %sResponse = %s;\n\n", typeName, respType))
							break
						}
					}
					break
				}
			}
		}
	}

	return nil
}

func generateOperationTypeName(method, path string) string {
	// Convert path to type name: /users/{id}/posts -> UsersIdPosts
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	parts := strings.Split(path, "/")

	var nameParts []string
	for _, part := range parts {
		if part != "" {
			nameParts = append(nameParts, toPascalCase(part))
		}
	}

	return method + strings.Join(nameParts, "")
}