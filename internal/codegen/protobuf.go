package codegen

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// generateProtobuf generates Protocol Buffer definitions from OpenAPI spec
func generateProtobuf(spec *openapi3.T, packageName string) (string, error) {
	var sb strings.Builder

	sb.WriteString("// Auto-generated Protocol Buffer definitions from OpenAPI spec\n")
	sb.WriteString("// Do not edit manually\n\n")
	sb.WriteString("syntax = \"proto3\";\n\n")
	sb.WriteString(fmt.Sprintf("package %s;\n\n", toSnakeCase(packageName)))

	// Check if we need google.protobuf.Timestamp
	needsTimestamp := false
	if spec.Components != nil && spec.Components.Schemas != nil {
		for _, schemaRef := range spec.Components.Schemas {
			if schemaUsesDateTime(schemaRef) {
				needsTimestamp = true
				break
			}
		}
	}

	if needsTimestamp {
		sb.WriteString("import \"google/protobuf/timestamp.proto\";\n\n")
	}

	// Generate messages for each schema in components
	if spec.Components != nil && spec.Components.Schemas != nil {
		for _, name := range getSortedSchemaKeys(spec.Components.Schemas) {
			schemaRef := spec.Components.Schemas[name]
			if err := generateProtoMessage(&sb, name, schemaRef); err != nil {
				return "", fmt.Errorf("failed to generate message %s: %w", name, err)
			}
			sb.WriteString("\n")
		}
	}

	// Generate service definition from paths
	if spec.Paths != nil {
		if err := generateProtoService(&sb, spec.Paths, packageName); err != nil {
			return "", err
		}
	}

	return sb.String(), nil
}

func generateProtoMessage(sb *strings.Builder, name string, schemaRef *openapi3.SchemaRef) error {
	if schemaRef == nil || schemaRef.Value == nil {
		return nil
	}

	schema := schemaRef.Value
	messageName := toPascalCase(name)

	// Handle enums
	if len(schema.Enum) > 0 {
		sb.WriteString(fmt.Sprintf("enum %s {\n", messageName))
		sb.WriteString(fmt.Sprintf("  %s_UNSPECIFIED = 0;\n", strings.ToUpper(toSnakeCase(name))))
		for i, v := range schema.Enum {
			if s, ok := v.(string); ok {
				enumName := strings.ToUpper(toSnakeCase(name) + "_" + toSnakeCase(s))
				sb.WriteString(fmt.Sprintf("  %s = %d;\n", enumName, i+1))
			}
		}
		sb.WriteString("}\n")
		return nil
	}

	// Handle object types as messages
	if schema.Type != nil && len(*schema.Type) > 0 && (*schema.Type)[0] == "object" {
		sb.WriteString(fmt.Sprintf("message %s {\n", messageName))

		fieldNum := 1
		for _, propName := range getSortedPropertyKeys(schema.Properties) {
			propSchema := schema.Properties[propName]
			protoType := schemaToProtoType(propSchema)
			fieldName := toSnakeCase(propName)

			// Handle repeated (array) fields
			if propSchema.Value != nil && propSchema.Value.Type != nil &&
				len(*propSchema.Value.Type) > 0 && (*propSchema.Value.Type)[0] == "array" {
				if propSchema.Value.Items != nil {
					itemType := schemaToProtoType(propSchema.Value.Items)
					sb.WriteString(fmt.Sprintf("  repeated %s %s = %d;\n", itemType, fieldName, fieldNum))
				} else {
					sb.WriteString(fmt.Sprintf("  repeated string %s = %d;\n", fieldName, fieldNum))
				}
			} else {
				// Handle optional fields
				optional := ""
				if !isRequired(propName, schema.Required) {
					optional = "optional "
				}
				sb.WriteString(fmt.Sprintf("  %s%s %s = %d;\n", optional, protoType, fieldName, fieldNum))
			}
			fieldNum++
		}

		// Handle additionalProperties as map
		if schema.AdditionalProperties.Has != nil && *schema.AdditionalProperties.Has {
			valueType := "string"
			if schema.AdditionalProperties.Schema != nil {
				valueType = schemaToProtoType(schema.AdditionalProperties.Schema)
			}
			sb.WriteString(fmt.Sprintf("  map<string, %s> additional_properties = %d;\n", valueType, fieldNum))
		}

		sb.WriteString("}\n")
		return nil
	}

	// For non-object types, create a wrapper message
	if schema.Type != nil && len(*schema.Type) > 0 {
		protoType := schemaToProtoType(schemaRef)
		sb.WriteString(fmt.Sprintf("message %s {\n", messageName))
		sb.WriteString(fmt.Sprintf("  %s value = 1;\n", protoType))
		sb.WriteString("}\n")
	}

	return nil
}

func schemaToProtoType(schemaRef *openapi3.SchemaRef) string {
	if schemaRef == nil {
		return "string"
	}

	// Handle $ref
	if schemaRef.Ref != "" {
		parts := strings.Split(schemaRef.Ref, "/")
		return toPascalCase(parts[len(parts)-1])
	}

	schema := schemaRef.Value
	if schema == nil {
		return "string"
	}

	// Handle oneOf/anyOf - use google.protobuf.Any or first type
	if len(schema.OneOf) > 0 {
		return schemaToProtoType(schema.OneOf[0])
	}
	if len(schema.AnyOf) > 0 {
		return schemaToProtoType(schema.AnyOf[0])
	}

	if schema.Type == nil || len(*schema.Type) == 0 {
		return "string"
	}

	baseType := (*schema.Type)[0]

	switch baseType {
	case "string":
		switch schema.Format {
		case "date-time", "date":
			return "google.protobuf.Timestamp"
		case "byte", "binary":
			return "bytes"
		default:
			return "string"
		}

	case "integer":
		switch schema.Format {
		case "int32":
			return "int32"
		case "int64":
			return "int64"
		default:
			return "int32"
		}

	case "number":
		switch schema.Format {
		case "float":
			return "float"
		case "double":
			return "double"
		default:
			return "double"
		}

	case "boolean":
		return "bool"

	case "array":
		// Arrays are handled at field level with 'repeated'
		if schema.Items != nil {
			return schemaToProtoType(schema.Items)
		}
		return "string"

	case "object":
		if len(schema.Properties) == 0 {
			// Generic object - use map or struct
			return "string" // Simplified
		}
		return "string" // Inline objects should be extracted

	default:
		return "string"
	}
}

func generateProtoService(sb *strings.Builder, paths *openapi3.Paths, serviceName string) error {
	sb.WriteString(fmt.Sprintf("service %sService {\n", toPascalCase(serviceName)))

	for path, pathItem := range paths.Map() {
		operations := map[string]*openapi3.Operation{
			"Get":    pathItem.Get,
			"Post":   pathItem.Post,
			"Put":    pathItem.Put,
			"Delete": pathItem.Delete,
			"Patch":  pathItem.Patch,
		}

		for method, op := range operations {
			if op == nil {
				continue
			}

			rpcName := generateRPCName(method, path)
			requestType := rpcName + "Request"
			responseType := rpcName + "Response"

			// Generate request message
			sb.WriteString(fmt.Sprintf("  // %s %s\n", method, path))
			sb.WriteString(fmt.Sprintf("  rpc %s(%s) returns (%s);\n", rpcName, requestType, responseType))
		}
	}

	sb.WriteString("}\n\n")

	// Generate request/response messages for each RPC
	for path, pathItem := range paths.Map() {
		operations := map[string]*openapi3.Operation{
			"Get":    pathItem.Get,
			"Post":   pathItem.Post,
			"Put":    pathItem.Put,
			"Delete": pathItem.Delete,
			"Patch":  pathItem.Patch,
		}

		for method, op := range operations {
			if op == nil {
				continue
			}

			rpcName := generateRPCName(method, path)
			generateProtoRPCMessages(sb, rpcName, op)
		}
	}

	return nil
}

func generateRPCName(method, path string) string {
	// Convert path to RPC name: /users/{id}/posts -> UsersIdPosts
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

func generateProtoRPCMessages(sb *strings.Builder, rpcName string, op *openapi3.Operation) {
	// Request message
	sb.WriteString(fmt.Sprintf("message %sRequest {\n", rpcName))
	fieldNum := 1

	// Add parameters
	for _, param := range op.Parameters {
		if param.Value != nil {
			protoType := "string"
			if param.Value.Schema != nil {
				protoType = schemaToProtoType(param.Value.Schema)
			}
			fieldName := toSnakeCase(param.Value.Name)
			optional := ""
			if !param.Value.Required {
				optional = "optional "
			}
			sb.WriteString(fmt.Sprintf("  %s%s %s = %d;\n", optional, protoType, fieldName, fieldNum))
			fieldNum++
		}
	}

	// Add request body
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		for _, content := range op.RequestBody.Value.Content {
			if content.Schema != nil {
				bodyType := schemaToProtoType(content.Schema)
				sb.WriteString(fmt.Sprintf("  %s body = %d;\n", bodyType, fieldNum))
				break
			}
		}
	}

	sb.WriteString("}\n\n")

	// Response message
	sb.WriteString(fmt.Sprintf("message %sResponse {\n", rpcName))
	if op.Responses != nil {
		for _, status := range []string{"200", "201"} {
			if resp := op.Responses.Value(status); resp != nil && resp.Value != nil {
				for _, content := range resp.Value.Content {
					if content.Schema != nil {
						respType := schemaToProtoType(content.Schema)
						sb.WriteString(fmt.Sprintf("  %s data = 1;\n", respType))
						break
					}
				}
				break
			}
		}
	}
	sb.WriteString("}\n\n")
}