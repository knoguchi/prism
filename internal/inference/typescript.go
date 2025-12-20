package inference

import (
	"fmt"
	"sort"
	"strings"
)

// TypeScriptGenerator generates TypeScript interfaces
type TypeScriptGenerator struct {
	engine *Engine
}

// NewTypeScriptGenerator creates a new TypeScript generator
func NewTypeScriptGenerator(engine *Engine) *TypeScriptGenerator {
	return &TypeScriptGenerator{engine: engine}
}

// Generate creates TypeScript interfaces for a host
func (g *TypeScriptGenerator) Generate(host string) (string, error) {
	hostSchema := g.engine.GetHostSchema(host)
	if hostSchema == nil {
		return "", fmt.Errorf("no schema found for host: %s", host)
	}

	var sb strings.Builder
	sb.WriteString("// Auto-generated TypeScript interfaces from captured traffic\n")
	sb.WriteString(fmt.Sprintf("// Host: %s\n\n", host))

	// Collect all unique types
	generatedTypes := make(map[string]bool)

	// Sort endpoints for consistent output
	var endpoints []*EndpointSchema
	for _, ep := range hostSchema.Endpoints {
		endpoints = append(endpoints, ep)
	}
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].PathPattern == endpoints[j].PathPattern {
			return endpoints[i].Method < endpoints[j].Method
		}
		return endpoints[i].PathPattern < endpoints[j].PathPattern
	})

	for _, endpoint := range endpoints {
		typeName := g.generateTypeName(endpoint.Method, endpoint.PathPattern)

		// Generate request type
		if endpoint.Request != nil && endpoint.Request.Type == "object" {
			reqTypeName := typeName + "Request"
			if !generatedTypes[reqTypeName] {
				sb.WriteString(g.generateInterface(reqTypeName, endpoint.Request))
				sb.WriteString("\n")
				generatedTypes[reqTypeName] = true
			}
		}

		// Generate response type
		if endpoint.Response != nil && endpoint.Response.Type == "object" {
			respTypeName := typeName + "Response"
			if !generatedTypes[respTypeName] {
				sb.WriteString(g.generateInterface(respTypeName, endpoint.Response))
				sb.WriteString("\n")
				generatedTypes[respTypeName] = true
			}
		}
	}

	return sb.String(), nil
}

// GenerateForEndpoint generates TypeScript for a specific endpoint
func (g *TypeScriptGenerator) GenerateForEndpoint(host, method, pathPattern string) (string, error) {
	endpoint := g.engine.GetEndpointSchema(host, method, pathPattern)
	if endpoint == nil {
		return "", fmt.Errorf("endpoint not found: %s %s", method, pathPattern)
	}

	var sb strings.Builder
	typeName := g.generateTypeName(method, pathPattern)

	if endpoint.Request != nil && endpoint.Request.Type == "object" {
		sb.WriteString(g.generateInterface(typeName+"Request", endpoint.Request))
		sb.WriteString("\n")
	}

	if endpoint.Response != nil && endpoint.Response.Type == "object" {
		sb.WriteString(g.generateInterface(typeName+"Response", endpoint.Response))
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// generateTypeName creates a TypeScript-friendly type name from method and path
func (g *TypeScriptGenerator) generateTypeName(method, pathPattern string) string {
	parts := strings.Split(strings.Trim(pathPattern, "/"), "/")
	var result strings.Builder

	for _, part := range parts {
		if strings.HasPrefix(part, "{") {
			continue // Skip parameters
		}
		result.WriteString(toPascalCase(part))
	}

	result.WriteString(toPascalCase(method))
	return result.String()
}

// generateInterface generates a TypeScript interface
func (g *TypeScriptGenerator) generateInterface(name string, schema *InferredType) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("export interface %s {\n", name))

	// Sort properties for consistent output
	var propNames []string
	for name := range schema.Properties {
		propNames = append(propNames, name)
	}
	sort.Strings(propNames)

	requiredSet := make(map[string]bool)
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	for _, propName := range propNames {
		prop := schema.Properties[propName]
		optional := ""
		if !requiredSet[propName] || prop.Nullable {
			optional = "?"
		}

		tsType := g.inferredTypeToTS(prop)
		sb.WriteString(fmt.Sprintf("  %s%s: %s;\n", propName, optional, tsType))
	}

	sb.WriteString("}\n")
	return sb.String()
}

// inferredTypeToTS converts an InferredType to a TypeScript type
func (g *TypeScriptGenerator) inferredTypeToTS(schema *InferredType) string {
	if schema == nil {
		return "unknown"
	}

	// Check for enums first
	if g.engine.IsEnumField(schema) {
		var values []string
		for _, v := range schema.Enum {
			if s, ok := v.(string); ok {
				values = append(values, fmt.Sprintf("'%s'", s))
			}
		}
		return strings.Join(values, " | ")
	}

	baseType := ""
	switch schema.Type {
	case "string":
		baseType = "string"
	case "integer", "number":
		baseType = "number"
	case "boolean":
		baseType = "boolean"
	case "null":
		return "null"
	case "array":
		if schema.Items != nil {
			itemType := g.inferredTypeToTS(schema.Items)
			baseType = itemType + "[]"
		} else {
			baseType = "unknown[]"
		}
	case "object":
		if len(schema.Properties) > 0 {
			// Inline object type
			var props []string
			var propNames []string
			for name := range schema.Properties {
				propNames = append(propNames, name)
			}
			sort.Strings(propNames)

			for _, name := range propNames {
				prop := schema.Properties[name]
				tsType := g.inferredTypeToTS(prop)
				props = append(props, fmt.Sprintf("%s: %s", name, tsType))
			}
			baseType = "{ " + strings.Join(props, "; ") + " }"
		} else {
			baseType = "Record<string, unknown>"
		}
	default:
		baseType = "unknown"
	}

	if schema.Nullable && baseType != "null" {
		return baseType + " | null"
	}
	return baseType
}

// toPascalCase converts a string to PascalCase
func toPascalCase(s string) string {
	if s == "" {
		return ""
	}
	// Split on non-alphanumeric
	words := strings.FieldsFunc(s, func(c rune) bool {
		return !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'))
	})

	var result strings.Builder
	for _, word := range words {
		if len(word) > 0 {
			result.WriteString(strings.ToUpper(word[:1]))
			result.WriteString(strings.ToLower(word[1:]))
		}
	}
	return result.String()
}
