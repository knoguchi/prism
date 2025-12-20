package codegen

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// generateGo generates Go struct definitions from OpenAPI spec
func generateGo(spec *openapi3.T, packageName string) (string, error) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("// Package %s contains auto-generated types from OpenAPI spec\n", packageName))
	sb.WriteString("// Do not edit manually\n")
	sb.WriteString(fmt.Sprintf("package %s\n\n", packageName))

	// Track if we need time import
	needsTime := false
	if spec.Components != nil && spec.Components.Schemas != nil {
		for _, schemaRef := range spec.Components.Schemas {
			if schemaUsesDateTime(schemaRef) {
				needsTime = true
				break
			}
		}
	}

	if needsTime {
		sb.WriteString("import \"time\"\n\n")
	}

	// Generate structs for each schema in components
	if spec.Components != nil && spec.Components.Schemas != nil {
		for _, name := range getSortedSchemaKeys(spec.Components.Schemas) {
			schemaRef := spec.Components.Schemas[name]
			if err := generateGoStruct(&sb, name, schemaRef); err != nil {
				return "", fmt.Errorf("failed to generate struct %s: %w", name, err)
			}
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

func schemaUsesDateTime(schemaRef *openapi3.SchemaRef) bool {
	if schemaRef == nil || schemaRef.Value == nil {
		return false
	}
	schema := schemaRef.Value

	// Check this schema
	if schema.Format == "date-time" || schema.Format == "date" {
		return true
	}

	// Check properties
	for _, prop := range schema.Properties {
		if schemaUsesDateTime(prop) {
			return true
		}
	}

	// Check array items
	if schema.Items != nil {
		if schemaUsesDateTime(schema.Items) {
			return true
		}
	}

	return false
}

func generateGoStruct(sb *strings.Builder, name string, schemaRef *openapi3.SchemaRef) error {
	if schemaRef == nil || schemaRef.Value == nil {
		return nil
	}

	schema := schemaRef.Value
	structName := toPascalCase(name)

	// Handle enums as string type with constants
	if len(schema.Enum) > 0 {
		sb.WriteString(fmt.Sprintf("type %s string\n\n", structName))
		sb.WriteString("const (\n")
		for _, v := range schema.Enum {
			if s, ok := v.(string); ok {
				constName := structName + toPascalCase(s)
				sb.WriteString(fmt.Sprintf("\t%s %s = \"%s\"\n", constName, structName, s))
			}
		}
		sb.WriteString(")\n")
		return nil
	}

	// Handle object types
	if schema.Type != nil && len(*schema.Type) > 0 && (*schema.Type)[0] == "object" {
		sb.WriteString(fmt.Sprintf("type %s struct {\n", structName))

		for _, propName := range getSortedPropertyKeys(schema.Properties) {
			propSchema := schema.Properties[propName]
			goType := schemaToGoType(propSchema, !isRequired(propName, schema.Required))
			fieldName := toPascalCase(propName)
			jsonTag := propName
			omitempty := ""
			if !isRequired(propName, schema.Required) {
				omitempty = ",omitempty"
			}
			sb.WriteString(fmt.Sprintf("\t%s %s `json:\"%s%s\"`\n", fieldName, goType, jsonTag, omitempty))
		}

		sb.WriteString("}\n")
		return nil
	}

	// Handle array types as type aliases
	if schema.Type != nil && len(*schema.Type) > 0 && (*schema.Type)[0] == "array" {
		itemType := schemaToGoType(schema.Items, false)
		sb.WriteString(fmt.Sprintf("type %s []%s\n", structName, itemType))
		return nil
	}

	// Handle primitive type aliases
	goType := schemaToGoType(schemaRef, false)
	sb.WriteString(fmt.Sprintf("type %s %s\n", structName, goType))
	return nil
}

func schemaToGoType(schemaRef *openapi3.SchemaRef, optional bool) string {
	if schemaRef == nil {
		return "interface{}"
	}

	// Handle $ref
	if schemaRef.Ref != "" {
		parts := strings.Split(schemaRef.Ref, "/")
		typeName := toPascalCase(parts[len(parts)-1])
		if optional {
			return "*" + typeName
		}
		return typeName
	}

	schema := schemaRef.Value
	if schema == nil {
		return "interface{}"
	}

	// Handle oneOf/anyOf as interface{}
	if len(schema.OneOf) > 0 || len(schema.AnyOf) > 0 {
		return "interface{}"
	}

	// Handle allOf - take first type (simplified)
	if len(schema.AllOf) > 0 {
		return schemaToGoType(schema.AllOf[0], optional)
	}

	if schema.Type == nil || len(*schema.Type) == 0 {
		return "interface{}"
	}

	baseType := (*schema.Type)[0]
	var goType string

	switch baseType {
	case "string":
		switch schema.Format {
		case "date-time":
			goType = "time.Time"
		case "date":
			goType = "time.Time"
		case "uuid":
			goType = "string"
		case "email":
			goType = "string"
		case "uri":
			goType = "string"
		case "byte":
			goType = "[]byte"
		case "binary":
			goType = "[]byte"
		default:
			goType = "string"
		}

	case "integer":
		switch schema.Format {
		case "int32":
			goType = "int32"
		case "int64":
			goType = "int64"
		default:
			goType = "int"
		}

	case "number":
		switch schema.Format {
		case "float":
			goType = "float32"
		case "double":
			goType = "float64"
		default:
			goType = "float64"
		}

	case "boolean":
		goType = "bool"

	case "array":
		if schema.Items != nil {
			itemType := schemaToGoType(schema.Items, false)
			goType = "[]" + itemType
		} else {
			goType = "[]interface{}"
		}

	case "object":
		if len(schema.Properties) == 0 {
			if schema.AdditionalProperties.Schema != nil {
				valueType := schemaToGoType(schema.AdditionalProperties.Schema, false)
				goType = fmt.Sprintf("map[string]%s", valueType)
			} else {
				goType = "map[string]interface{}"
			}
		} else {
			// Inline struct - simplified, just use interface{}
			goType = "interface{}"
		}

	default:
		goType = "interface{}"
	}

	// Handle nullable/optional with pointer
	if optional && !strings.HasPrefix(goType, "[]") && !strings.HasPrefix(goType, "map") {
		return "*" + goType
	}

	return goType
}