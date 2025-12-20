package inference

import (
	"fmt"
	"sort"
	"strings"
)

// ProtobufGenerator generates Protocol Buffer definitions
type ProtobufGenerator struct {
	engine      *Engine
	packageName string
}

// NewProtobufGenerator creates a new Protobuf generator
func NewProtobufGenerator(engine *Engine, packageName string) *ProtobufGenerator {
	if packageName == "" {
		packageName = "api"
	}
	return &ProtobufGenerator{
		engine:      engine,
		packageName: packageName,
	}
}

// Generate creates Protobuf definitions for a host
func (g *ProtobufGenerator) Generate(host string) (string, error) {
	hostSchema := g.engine.GetHostSchema(host)
	if hostSchema == nil {
		return "", fmt.Errorf("no schema found for host: %s", host)
	}

	var sb strings.Builder
	sb.WriteString("// Auto-generated Protocol Buffer definitions from captured traffic\n")
	sb.WriteString(fmt.Sprintf("// Host: %s\n\n", host))
	sb.WriteString("syntax = \"proto3\";\n\n")
	sb.WriteString(fmt.Sprintf("package %s;\n\n", g.packageName))
	sb.WriteString(fmt.Sprintf("option go_package = \"./%s\";\n\n", g.packageName))

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

	generatedTypes := make(map[string]bool)
	var messages []string

	for _, endpoint := range endpoints {
		typeName := g.generateTypeName(endpoint.Method, endpoint.PathPattern)

		// Generate request message
		if endpoint.Request != nil && endpoint.Request.Type == "object" {
			reqTypeName := typeName + "Request"
			if !generatedTypes[reqTypeName] {
				messages = append(messages, g.generateMessage(reqTypeName, endpoint.Request))
				generatedTypes[reqTypeName] = true
			}
		}

		// Generate response message
		if endpoint.Response != nil && endpoint.Response.Type == "object" {
			respTypeName := typeName + "Response"
			if !generatedTypes[respTypeName] {
				messages = append(messages, g.generateMessage(respTypeName, endpoint.Response))
				generatedTypes[respTypeName] = true
			}
		}
	}

	for _, msg := range messages {
		sb.WriteString(msg)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// GenerateForEndpoint generates Protobuf for a specific endpoint
func (g *ProtobufGenerator) GenerateForEndpoint(host, method, pathPattern string) (string, error) {
	endpoint := g.engine.GetEndpointSchema(host, method, pathPattern)
	if endpoint == nil {
		return "", fmt.Errorf("endpoint not found: %s %s", method, pathPattern)
	}

	var sb strings.Builder
	typeName := g.generateTypeName(method, pathPattern)

	if endpoint.Request != nil && endpoint.Request.Type == "object" {
		sb.WriteString(g.generateMessage(typeName+"Request", endpoint.Request))
		sb.WriteString("\n")
	}

	if endpoint.Response != nil && endpoint.Response.Type == "object" {
		sb.WriteString(g.generateMessage(typeName+"Response", endpoint.Response))
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// generateTypeName creates a Protobuf-friendly message name
func (g *ProtobufGenerator) generateTypeName(method, pathPattern string) string {
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

// generateMessage generates a Protobuf message definition
func (g *ProtobufGenerator) generateMessage(name string, schema *InferredType) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("message %s {\n", name))

	// Sort properties for consistent output
	var propNames []string
	for name := range schema.Properties {
		propNames = append(propNames, name)
	}
	sort.Strings(propNames)

	fieldNum := 1
	for _, propName := range propNames {
		prop := schema.Properties[propName]
		protoType := g.inferredTypeToProto(prop)
		protoFieldName := toSnakeCase(propName)

		sb.WriteString(fmt.Sprintf("  %s %s = %d;\n", protoType, protoFieldName, fieldNum))
		fieldNum++
	}

	sb.WriteString("}\n")
	return sb.String()
}

// inferredTypeToProto converts an InferredType to a Protobuf type
func (g *ProtobufGenerator) inferredTypeToProto(schema *InferredType) string {
	if schema == nil {
		return "string"
	}

	switch schema.Type {
	case "string":
		return "string"

	case "integer":
		intType := g.engine.InferIntegerType(schema)
		switch intType {
		case "int32":
			return "int32"
		default:
			return "int64"
		}

	case "number":
		return "double"

	case "boolean":
		return "bool"

	case "array":
		if schema.Items != nil {
			itemType := g.inferredTypeToProto(schema.Items)
			return "repeated " + itemType
		}
		return "repeated string"

	case "object":
		// For nested objects, we'd need to generate nested messages
		// For now, use a simple map or bytes
		return "bytes"

	default:
		return "string"
	}
}

// toSnakeCase converts a string to snake_case
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result.WriteRune('_')
			}
			result.WriteRune(c - 'A' + 'a')
		} else if c == '-' {
			result.WriteRune('_')
		} else {
			result.WriteRune(c)
		}
	}
	return result.String()
}
