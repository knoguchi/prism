package inference

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ai-proxy/pkg/models"
)

// Generator provides a unified interface for schema generation
type Generator struct {
	engine     *Engine
	openapi    *OpenAPIGenerator
	typescript *TypeScriptGenerator
	golang     *GoGenerator
	protobuf   *ProtobufGenerator
	sql        *SQLGenerator
}

// NewGenerator creates a new unified generator
func NewGenerator(engine *Engine) *Generator {
	return &Generator{
		engine:     engine,
		openapi:    NewOpenAPIGenerator(engine),
		typescript: NewTypeScriptGenerator(engine),
		golang:     NewGoGenerator(engine, "models"),
		protobuf:   NewProtobufGenerator(engine, "api"),
		sql:        NewSQLGenerator(engine, "postgres"),
	}
}

// Generate creates a schema in the specified format
func (g *Generator) Generate(host string, format models.SchemaFormat) (string, error) {
	switch format {
	case models.SchemaFormatOpenAPI:
		return g.openapi.GenerateYAML(host)

	case models.SchemaFormatTypeScript:
		return g.typescript.Generate(host)

	case models.SchemaFormatGo:
		return g.golang.Generate(host)

	case models.SchemaFormatProtobuf:
		return g.protobuf.Generate(host)

	case models.SchemaFormatSQL:
		return g.sql.Generate(host)

	case models.SchemaFormatJSONSchema:
		return g.generateJSONSchema(host)

	case models.SchemaFormatAvro:
		return g.generateAvro(host)

	case models.SchemaFormatGraphQL:
		return g.generateGraphQL(host)

	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
}

// GenerateForEndpoint creates a schema for a specific endpoint
func (g *Generator) GenerateForEndpoint(host, method, pathPattern string, format models.SchemaFormat) (string, error) {
	switch format {
	case models.SchemaFormatOpenAPI:
		pathItem, err := g.openapi.GenerateForEndpoint(host, method, pathPattern)
		if err != nil {
			return "", err
		}
		// Convert to YAML
		data, err := json.MarshalIndent(pathItem, "", "  ")
		return string(data), err

	case models.SchemaFormatTypeScript:
		return g.typescript.GenerateForEndpoint(host, method, pathPattern)

	case models.SchemaFormatGo:
		return g.golang.GenerateForEndpoint(host, method, pathPattern)

	case models.SchemaFormatProtobuf:
		return g.protobuf.GenerateForEndpoint(host, method, pathPattern)

	case models.SchemaFormatSQL:
		return g.sql.GenerateForEndpoint(host, method, pathPattern)

	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
}

// ListEndpoints returns all endpoints for a host
func (g *Generator) ListEndpoints(host string) []EndpointSummary {
	return g.openapi.ListEndpoints(host)
}

// ListHosts returns all hosts with schemas
func (g *Generator) ListHosts() []string {
	return g.engine.ListHosts()
}

// generateJSONSchema generates JSON Schema draft-07
func (g *Generator) generateJSONSchema(host string) (string, error) {
	hostSchema := g.engine.GetHostSchema(host)
	if hostSchema == nil {
		return "", fmt.Errorf("no schema found for host: %s", host)
	}

	schemas := make(map[string]interface{})

	for _, endpoint := range hostSchema.Endpoints {
		typeName := g.generateTypeName(endpoint.Method, endpoint.PathPattern)

		if endpoint.Request != nil && endpoint.Request.Type == "object" {
			schemas[typeName+"Request"] = g.inferredToJSONSchema(endpoint.Request)
		}

		if endpoint.Response != nil && endpoint.Response.Type == "object" {
			schemas[typeName+"Response"] = g.inferredToJSONSchema(endpoint.Response)
		}
	}

	result := map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"title":       fmt.Sprintf("%s API Schemas", host),
		"definitions": schemas,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	return string(data), err
}

// inferredToJSONSchema converts InferredType to JSON Schema
func (g *Generator) inferredToJSONSchema(schema *InferredType) map[string]interface{} {
	if schema == nil {
		return map[string]interface{}{"type": "object"}
	}

	result := make(map[string]interface{})

	switch schema.Type {
	case "object":
		result["type"] = "object"
		if len(schema.Properties) > 0 {
			props := make(map[string]interface{})
			for name, prop := range schema.Properties {
				props[name] = g.inferredToJSONSchema(prop)
			}
			result["properties"] = props
		}
		if len(schema.Required) > 0 {
			result["required"] = schema.Required
		}

	case "array":
		result["type"] = "array"
		if schema.Items != nil {
			result["items"] = g.inferredToJSONSchema(schema.Items)
		}

	case "string":
		result["type"] = "string"
		if schema.Format != "" {
			result["format"] = schema.Format
		}
		if g.engine.IsEnumField(schema) {
			result["enum"] = schema.Enum
		}

	case "integer":
		result["type"] = "integer"
		intType := g.engine.InferIntegerType(schema)
		if intType == "int64" {
			result["format"] = "int64"
		}

	case "number":
		result["type"] = "number"

	case "boolean":
		result["type"] = "boolean"

	case "null":
		result["type"] = "null"
	}

	if schema.Nullable && schema.Type != "null" {
		// JSON Schema draft-07 uses oneOf for nullable
		result["nullable"] = true
	}

	if len(schema.Examples) > 0 {
		result["examples"] = schema.Examples
	}

	return result
}

// generateAvro generates Avro schema
func (g *Generator) generateAvro(host string) (string, error) {
	hostSchema := g.engine.GetHostSchema(host)
	if hostSchema == nil {
		return "", fmt.Errorf("no schema found for host: %s", host)
	}

	var records []interface{}

	var endpoints []*EndpointSchema
	for _, ep := range hostSchema.Endpoints {
		endpoints = append(endpoints, ep)
	}
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].PathPattern < endpoints[j].PathPattern
	})

	for _, endpoint := range endpoints {
		typeName := g.generateTypeName(endpoint.Method, endpoint.PathPattern)

		if endpoint.Response != nil && endpoint.Response.Type == "object" {
			records = append(records, g.inferredToAvro(typeName+"Response", endpoint.Response))
		}
	}

	data, err := json.MarshalIndent(records, "", "  ")
	return string(data), err
}

// inferredToAvro converts InferredType to Avro schema
func (g *Generator) inferredToAvro(name string, schema *InferredType) map[string]interface{} {
	if schema == nil {
		return map[string]interface{}{"type": "string"}
	}

	switch schema.Type {
	case "object":
		fields := make([]map[string]interface{}, 0)
		var propNames []string
		for n := range schema.Properties {
			propNames = append(propNames, n)
		}
		sort.Strings(propNames)

		requiredSet := make(map[string]bool)
		for _, r := range schema.Required {
			requiredSet[r] = true
		}

		for _, propName := range propNames {
			prop := schema.Properties[propName]
			avroType := g.inferredTypeToAvro(prop)

			// Make optional fields nullable
			if !requiredSet[propName] || prop.Nullable {
				avroType = []interface{}{"null", avroType}
			}

			fields = append(fields, map[string]interface{}{
				"name": propName,
				"type": avroType,
			})
		}

		return map[string]interface{}{
			"type":   "record",
			"name":   name,
			"fields": fields,
		}

	default:
		return map[string]interface{}{"type": g.inferredTypeToAvro(schema)}
	}
}

// inferredTypeToAvro returns Avro type for primitive types
func (g *Generator) inferredTypeToAvro(schema *InferredType) interface{} {
	if schema == nil {
		return "string"
	}

	switch schema.Type {
	case "string":
		return "string"
	case "integer":
		intType := g.engine.InferIntegerType(schema)
		if intType == "int32" {
			return "int"
		}
		return "long"
	case "number":
		return "double"
	case "boolean":
		return "boolean"
	case "array":
		if schema.Items != nil {
			return map[string]interface{}{
				"type":  "array",
				"items": g.inferredTypeToAvro(schema.Items),
			}
		}
		return map[string]interface{}{
			"type":  "array",
			"items": "string",
		}
	case "object":
		return "string" // Serialize as JSON string
	default:
		return "string"
	}
}

// generateGraphQL generates GraphQL SDL
func (g *Generator) generateGraphQL(host string) (string, error) {
	hostSchema := g.engine.GetHostSchema(host)
	if hostSchema == nil {
		return "", fmt.Errorf("no schema found for host: %s", host)
	}

	var sb strings.Builder
	sb.WriteString("# Auto-generated GraphQL schema from captured traffic\n")
	sb.WriteString(fmt.Sprintf("# Host: %s\n\n", host))

	generatedTypes := make(map[string]bool)

	var endpoints []*EndpointSchema
	for _, ep := range hostSchema.Endpoints {
		endpoints = append(endpoints, ep)
	}
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].PathPattern < endpoints[j].PathPattern
	})

	for _, endpoint := range endpoints {
		typeName := g.generateTypeName(endpoint.Method, endpoint.PathPattern)

		if endpoint.Response != nil && endpoint.Response.Type == "object" {
			respTypeName := typeName + "Response"
			if !generatedTypes[respTypeName] {
				sb.WriteString(g.generateGraphQLType(respTypeName, endpoint.Response))
				sb.WriteString("\n")
				generatedTypes[respTypeName] = true
			}
		}
	}

	return sb.String(), nil
}

// generateGraphQLType generates a GraphQL type definition
func (g *Generator) generateGraphQLType(name string, schema *InferredType) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("type %s {\n", name))

	var propNames []string
	for n := range schema.Properties {
		propNames = append(propNames, n)
	}
	sort.Strings(propNames)

	requiredSet := make(map[string]bool)
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	for _, propName := range propNames {
		prop := schema.Properties[propName]
		gqlType := g.inferredTypeToGraphQL(prop)

		// Add ! for required non-nullable fields
		if requiredSet[propName] && !prop.Nullable {
			gqlType += "!"
		}

		sb.WriteString(fmt.Sprintf("  %s: %s\n", propName, gqlType))
	}

	sb.WriteString("}\n")
	return sb.String()
}

// inferredTypeToGraphQL converts InferredType to GraphQL type
func (g *Generator) inferredTypeToGraphQL(schema *InferredType) string {
	if schema == nil {
		return "String"
	}

	switch schema.Type {
	case "string":
		if schema.Format == "uuid" {
			return "ID"
		}
		return "String"
	case "integer":
		return "Int"
	case "number":
		return "Float"
	case "boolean":
		return "Boolean"
	case "array":
		if schema.Items != nil {
			itemType := g.inferredTypeToGraphQL(schema.Items)
			return "[" + itemType + "]"
		}
		return "[String]"
	case "object":
		return "String" // JSON string
	default:
		return "String"
	}
}

// generateTypeName creates a type name from method and path
func (g *Generator) generateTypeName(method, pathPattern string) string {
	parts := strings.Split(strings.Trim(pathPattern, "/"), "/")
	var result strings.Builder

	for _, part := range parts {
		if strings.HasPrefix(part, "{") {
			continue
		}
		result.WriteString(toPascalCase(part))
	}

	result.WriteString(toPascalCase(method))
	return result.String()
}
