package codegen

import (
	"encoding/json"

	"github.com/getkin/kin-openapi/openapi3"
)

// generateJSONSchema extracts JSON Schema from OpenAPI components
func generateJSONSchema(spec *openapi3.T) (string, error) {
	if spec.Components == nil || spec.Components.Schemas == nil {
		return "{}", nil
	}

	// Build a JSON Schema document with definitions
	schemaDoc := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"definitions": make(map[string]interface{}),
	}

	definitions := schemaDoc["definitions"].(map[string]interface{})

	for name, schemaRef := range spec.Components.Schemas {
		if schemaRef.Value != nil {
			definitions[name] = convertOpenAPISchemaToJSONSchema(schemaRef)
		}
	}

	// Pretty print JSON
	output, err := json.MarshalIndent(schemaDoc, "", "  ")
	if err != nil {
		return "", err
	}

	return string(output), nil
}

func convertOpenAPISchemaToJSONSchema(schemaRef *openapi3.SchemaRef) map[string]interface{} {
	if schemaRef == nil {
		return map[string]interface{}{"type": "object"}
	}

	// Handle $ref
	if schemaRef.Ref != "" {
		// Convert OpenAPI ref to JSON Schema ref
		// #/components/schemas/User -> #/definitions/User
		ref := schemaRef.Ref
		ref = replacePrefix(ref, "#/components/schemas/", "#/definitions/")
		return map[string]interface{}{"$ref": ref}
	}

	schema := schemaRef.Value
	if schema == nil {
		return map[string]interface{}{"type": "object"}
	}

	result := make(map[string]interface{})

	// Handle oneOf/anyOf/allOf
	if len(schema.OneOf) > 0 {
		var oneOf []interface{}
		for _, s := range schema.OneOf {
			oneOf = append(oneOf, convertOpenAPISchemaToJSONSchema(s))
		}
		result["oneOf"] = oneOf
		return result
	}
	if len(schema.AnyOf) > 0 {
		var anyOf []interface{}
		for _, s := range schema.AnyOf {
			anyOf = append(anyOf, convertOpenAPISchemaToJSONSchema(s))
		}
		result["anyOf"] = anyOf
		return result
	}
	if len(schema.AllOf) > 0 {
		var allOf []interface{}
		for _, s := range schema.AllOf {
			allOf = append(allOf, convertOpenAPISchemaToJSONSchema(s))
		}
		result["allOf"] = allOf
		return result
	}

	// Type
	if schema.Type != nil && len(*schema.Type) > 0 {
		result["type"] = (*schema.Type)[0]
	}

	// Format
	if schema.Format != "" {
		result["format"] = schema.Format
	}

	// Description
	if schema.Description != "" {
		result["description"] = schema.Description
	}

	// Title
	if schema.Title != "" {
		result["title"] = schema.Title
	}

	// Enum
	if len(schema.Enum) > 0 {
		result["enum"] = schema.Enum
	}

	// Default
	if schema.Default != nil {
		result["default"] = schema.Default
	}

	// Nullable
	if schema.Nullable {
		// In JSON Schema draft-07, use type array
		if t, ok := result["type"].(string); ok {
			result["type"] = []string{t, "null"}
		}
	}

	// String validations
	if schema.MinLength != 0 {
		result["minLength"] = schema.MinLength
	}
	if schema.MaxLength != nil {
		result["maxLength"] = *schema.MaxLength
	}
	if schema.Pattern != "" {
		result["pattern"] = schema.Pattern
	}

	// Number validations
	if schema.Min != nil {
		result["minimum"] = *schema.Min
	}
	if schema.Max != nil {
		result["maximum"] = *schema.Max
	}
	if schema.ExclusiveMin {
		result["exclusiveMinimum"] = true
	}
	if schema.ExclusiveMax {
		result["exclusiveMaximum"] = true
	}
	if schema.MultipleOf != nil {
		result["multipleOf"] = *schema.MultipleOf
	}

	// Array validations
	if schema.MinItems != 0 {
		result["minItems"] = schema.MinItems
	}
	if schema.MaxItems != nil {
		result["maxItems"] = *schema.MaxItems
	}
	if schema.UniqueItems {
		result["uniqueItems"] = true
	}

	// Array items
	if schema.Items != nil {
		result["items"] = convertOpenAPISchemaToJSONSchema(schema.Items)
	}

	// Object properties
	if len(schema.Properties) > 0 {
		properties := make(map[string]interface{})
		for name, propSchema := range schema.Properties {
			properties[name] = convertOpenAPISchemaToJSONSchema(propSchema)
		}
		result["properties"] = properties
	}

	// Required properties
	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}

	// Additional properties
	if schema.AdditionalProperties.Has != nil {
		if !*schema.AdditionalProperties.Has {
			result["additionalProperties"] = false
		} else if schema.AdditionalProperties.Schema != nil {
			result["additionalProperties"] = convertOpenAPISchemaToJSONSchema(schema.AdditionalProperties.Schema)
		}
	}

	// Min/Max properties
	if schema.MinProps != 0 {
		result["minProperties"] = schema.MinProps
	}
	if schema.MaxProps != nil {
		result["maxProperties"] = *schema.MaxProps
	}

	return result
}

func replacePrefix(s, oldPrefix, newPrefix string) string {
	if len(s) >= len(oldPrefix) && s[:len(oldPrefix)] == oldPrefix {
		return newPrefix + s[len(oldPrefix):]
	}
	return s
}