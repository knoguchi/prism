package inference

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// GoGenerator generates Go struct definitions
type GoGenerator struct {
	engine      *Engine
	packageName string
}

// NewGoGenerator creates a new Go generator
func NewGoGenerator(engine *Engine, packageName string) *GoGenerator {
	if packageName == "" {
		packageName = "models"
	}
	return &GoGenerator{
		engine:      engine,
		packageName: packageName,
	}
}

// Generate creates Go structs for a host
func (g *GoGenerator) Generate(host string) (string, error) {
	hostSchema := g.engine.GetHostSchema(host)
	if hostSchema == nil {
		return "", fmt.Errorf("no schema found for host: %s", host)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("// Auto-generated Go structs from captured traffic\n"))
	sb.WriteString(fmt.Sprintf("// Host: %s\n\n", host))
	sb.WriteString(fmt.Sprintf("package %s\n\n", g.packageName))

	// Collect imports
	imports := make(map[string]bool)

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

	var structs []string
	generatedTypes := make(map[string]bool)

	for _, endpoint := range endpoints {
		typeName := g.generateTypeName(endpoint.Method, endpoint.PathPattern)

		// Generate request type
		if endpoint.Request != nil && endpoint.Request.Type == "object" {
			reqTypeName := typeName + "Request"
			if !generatedTypes[reqTypeName] {
				structDef, structImports := g.generateStruct(reqTypeName, endpoint.Request)
				structs = append(structs, structDef)
				for imp := range structImports {
					imports[imp] = true
				}
				generatedTypes[reqTypeName] = true
			}
		}

		// Generate response type
		if endpoint.Response != nil && endpoint.Response.Type == "object" {
			respTypeName := typeName + "Response"
			if !generatedTypes[respTypeName] {
				structDef, structImports := g.generateStruct(respTypeName, endpoint.Response)
				structs = append(structs, structDef)
				for imp := range structImports {
					imports[imp] = true
				}
				generatedTypes[respTypeName] = true
			}
		}
	}

	// Add imports
	if len(imports) > 0 {
		sb.WriteString("import (\n")
		var importList []string
		for imp := range imports {
			importList = append(importList, imp)
		}
		sort.Strings(importList)
		for _, imp := range importList {
			sb.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
		sb.WriteString(")\n\n")
	}

	// Add structs
	for _, structDef := range structs {
		sb.WriteString(structDef)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// GenerateForEndpoint generates Go structs for a specific endpoint
func (g *GoGenerator) GenerateForEndpoint(host, method, pathPattern string) (string, error) {
	endpoint := g.engine.GetEndpointSchema(host, method, pathPattern)
	if endpoint == nil {
		return "", fmt.Errorf("endpoint not found: %s %s", method, pathPattern)
	}

	var sb strings.Builder
	typeName := g.generateTypeName(method, pathPattern)
	imports := make(map[string]bool)

	if endpoint.Request != nil && endpoint.Request.Type == "object" {
		structDef, structImports := g.generateStruct(typeName+"Request", endpoint.Request)
		sb.WriteString(structDef)
		sb.WriteString("\n")
		for imp := range structImports {
			imports[imp] = true
		}
	}

	if endpoint.Response != nil && endpoint.Response.Type == "object" {
		structDef, structImports := g.generateStruct(typeName+"Response", endpoint.Response)
		sb.WriteString(structDef)
		sb.WriteString("\n")
		for imp := range structImports {
			imports[imp] = true
		}
	}

	return sb.String(), nil
}

// generateTypeName creates a Go-friendly type name
func (g *GoGenerator) generateTypeName(method, pathPattern string) string {
	parts := strings.Split(strings.Trim(pathPattern, "/"), "/")
	var result strings.Builder

	for _, part := range parts {
		if strings.HasPrefix(part, "{") {
			continue // Skip parameters
		}
		result.WriteString(toGoPublicName(part))
	}

	result.WriteString(toGoPublicName(method))
	return result.String()
}

// generateStruct generates a Go struct definition
func (g *GoGenerator) generateStruct(name string, schema *InferredType) (string, map[string]bool) {
	imports := make(map[string]bool)
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("type %s struct {\n", name))

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
		goFieldName := toGoPublicName(propName)
		goType, typeImports := g.inferredTypeToGo(prop, !requiredSet[propName] || prop.Nullable)

		for imp := range typeImports {
			imports[imp] = true
		}

		// Generate JSON tag
		jsonTag := propName
		if !requiredSet[propName] {
			jsonTag += ",omitempty"
		}

		sb.WriteString(fmt.Sprintf("\t%s %s `json:\"%s\"`\n", goFieldName, goType, jsonTag))
	}

	sb.WriteString("}\n")
	return sb.String(), imports
}

// inferredTypeToGo converts an InferredType to a Go type
func (g *GoGenerator) inferredTypeToGo(schema *InferredType, pointer bool) (string, map[string]bool) {
	imports := make(map[string]bool)

	if schema == nil {
		return "interface{}", imports
	}

	var baseType string

	switch schema.Type {
	case "string":
		switch schema.Format {
		case "date-time":
			imports["time"] = true
			baseType = "time.Time"
		case "uuid":
			baseType = "string" // Could use google/uuid
		default:
			baseType = "string"
		}

	case "integer":
		intType := g.engine.InferIntegerType(schema)
		switch intType {
		case "int32":
			baseType = "int32"
		default:
			baseType = "int64"
		}

	case "number":
		baseType = "float64"

	case "boolean":
		baseType = "bool"

	case "null":
		return "interface{}", imports

	case "array":
		if schema.Items != nil {
			itemType, itemImports := g.inferredTypeToGo(schema.Items, false)
			for imp := range itemImports {
				imports[imp] = true
			}
			baseType = "[]" + itemType
		} else {
			baseType = "[]interface{}"
		}

	case "object":
		if len(schema.Properties) > 0 {
			// Inline struct
			var fields []string
			var propNames []string
			for name := range schema.Properties {
				propNames = append(propNames, name)
			}
			sort.Strings(propNames)

			for _, name := range propNames {
				prop := schema.Properties[name]
				goType, typeImports := g.inferredTypeToGo(prop, prop.Nullable)
				for imp := range typeImports {
					imports[imp] = true
				}
				goFieldName := toGoPublicName(name)
				fields = append(fields, fmt.Sprintf("%s %s `json:\"%s\"`", goFieldName, goType, name))
			}
			baseType = "struct {\n\t\t" + strings.Join(fields, "\n\t\t") + "\n\t}"
		} else {
			baseType = "map[string]interface{}"
		}

	default:
		baseType = "interface{}"
	}

	// Use pointer for nullable/optional fields (except slices and maps)
	if pointer && !strings.HasPrefix(baseType, "[]") && !strings.HasPrefix(baseType, "map[") {
		return "*" + baseType, imports
	}

	return baseType, imports
}

// toGoPublicName converts a string to a public Go identifier
func toGoPublicName(s string) string {
	if s == "" {
		return ""
	}

	// Handle common abbreviations
	acronyms := map[string]string{
		"id":   "ID",
		"url":  "URL",
		"uri":  "URI",
		"api":  "API",
		"http": "HTTP",
		"uuid": "UUID",
		"json": "JSON",
		"xml":  "XML",
		"sql":  "SQL",
		"ip":   "IP",
		"tcp":  "TCP",
		"udp":  "UDP",
		"html": "HTML",
		"css":  "CSS",
	}

	// Split on non-alphanumeric
	words := strings.FieldsFunc(s, func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsDigit(c)
	})

	var result strings.Builder
	for _, word := range words {
		lower := strings.ToLower(word)
		if acronym, ok := acronyms[lower]; ok {
			result.WriteString(acronym)
		} else if len(word) > 0 {
			result.WriteString(strings.ToUpper(word[:1]))
			result.WriteString(strings.ToLower(word[1:]))
		}
	}

	name := result.String()

	// Ensure starts with letter
	if len(name) > 0 && !unicode.IsLetter(rune(name[0])) {
		name = "X" + name
	}

	return name
}
