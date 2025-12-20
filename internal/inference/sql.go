package inference

import (
	"fmt"
	"sort"
	"strings"
)

// SQLGenerator generates SQL CREATE TABLE statements
type SQLGenerator struct {
	engine  *Engine
	dialect string // "mysql", "postgres", "sqlite"
}

// NewSQLGenerator creates a new SQL generator
func NewSQLGenerator(engine *Engine, dialect string) *SQLGenerator {
	if dialect == "" {
		dialect = "postgres"
	}
	return &SQLGenerator{
		engine:  engine,
		dialect: dialect,
	}
}

// Generate creates SQL DDL for a host
func (g *SQLGenerator) Generate(host string) (string, error) {
	hostSchema := g.engine.GetHostSchema(host)
	if hostSchema == nil {
		return "", fmt.Errorf("no schema found for host: %s", host)
	}

	var sb strings.Builder
	sb.WriteString("-- Auto-generated SQL DDL from captured traffic\n")
	sb.WriteString(fmt.Sprintf("-- Host: %s\n", host))
	sb.WriteString(fmt.Sprintf("-- Dialect: %s\n\n", g.dialect))

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

	generatedTables := make(map[string]bool)

	for _, endpoint := range endpoints {
		// Generate table from response (usually contains the data model)
		if endpoint.Response != nil && endpoint.Response.Type == "object" {
			tableName := g.generateTableName(endpoint.PathPattern)
			if !generatedTables[tableName] {
				sb.WriteString(g.generateTable(tableName, endpoint.Response))
				sb.WriteString("\n")
				generatedTables[tableName] = true
			}
		}
	}

	return sb.String(), nil
}

// GenerateForEndpoint generates SQL for a specific endpoint
func (g *SQLGenerator) GenerateForEndpoint(host, method, pathPattern string) (string, error) {
	endpoint := g.engine.GetEndpointSchema(host, method, pathPattern)
	if endpoint == nil {
		return "", fmt.Errorf("endpoint not found: %s %s", method, pathPattern)
	}

	var sb strings.Builder
	tableName := g.generateTableName(pathPattern)

	if endpoint.Response != nil && endpoint.Response.Type == "object" {
		sb.WriteString(g.generateTable(tableName, endpoint.Response))
	}

	return sb.String(), nil
}

// generateTableName creates a SQL table name from path
func (g *SQLGenerator) generateTableName(pathPattern string) string {
	parts := strings.Split(strings.Trim(pathPattern, "/"), "/")
	var tableName string

	for _, part := range parts {
		if strings.HasPrefix(part, "{") {
			continue
		}
		// Take the first meaningful part as table name
		if tableName == "" || versionRegex.MatchString(tableName) {
			tableName = part
		}
	}

	if tableName == "" {
		tableName = "data"
	}

	return toSnakeCase(tableName)
}

// generateTable generates a CREATE TABLE statement
func (g *SQLGenerator) generateTable(name string, schema *InferredType) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", name))

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

	// Check if 'id' field exists for primary key
	hasID := false
	for _, name := range propNames {
		if strings.ToLower(name) == "id" {
			hasID = true
			break
		}
	}

	var columns []string
	for _, propName := range propNames {
		prop := schema.Properties[propName]
		columnName := toSnakeCase(propName)
		sqlType := g.inferredTypeToSQL(prop)
		nullable := ""
		if requiredSet[propName] && !prop.Nullable {
			nullable = " NOT NULL"
		}

		// Add primary key for id field
		primaryKey := ""
		if strings.ToLower(propName) == "id" {
			primaryKey = " PRIMARY KEY"
		}

		columns = append(columns, fmt.Sprintf("  %s %s%s%s", columnName, sqlType, nullable, primaryKey))
	}

	// Add auto-generated id if none exists
	if !hasID {
		idColumn := g.generateAutoID()
		columns = append([]string{idColumn}, columns...)
	}

	sb.WriteString(strings.Join(columns, ",\n"))
	sb.WriteString("\n);\n")

	return sb.String()
}

// generateAutoID generates an auto-increment ID column
func (g *SQLGenerator) generateAutoID() string {
	switch g.dialect {
	case "mysql":
		return "  id BIGINT AUTO_INCREMENT PRIMARY KEY"
	case "sqlite":
		return "  id INTEGER PRIMARY KEY AUTOINCREMENT"
	default: // postgres
		return "  id BIGSERIAL PRIMARY KEY"
	}
}

// inferredTypeToSQL converts an InferredType to a SQL type
func (g *SQLGenerator) inferredTypeToSQL(schema *InferredType) string {
	if schema == nil {
		return "TEXT"
	}

	switch schema.Type {
	case "string":
		switch schema.Format {
		case "date-time":
			if g.dialect == "mysql" {
				return "DATETIME"
			}
			return "TIMESTAMP"
		case "date":
			return "DATE"
		case "uuid":
			if g.dialect == "postgres" {
				return "UUID"
			}
			return "VARCHAR(36)"
		case "email", "uri":
			return "VARCHAR(255)"
		default:
			// Check enum
			if g.engine.IsEnumField(schema) && len(schema.Enum) <= 10 {
				// Could use ENUM for MySQL, but TEXT is safer
				return "VARCHAR(50)"
			}
			return "TEXT"
		}

	case "integer":
		intType := g.engine.InferIntegerType(schema)
		switch intType {
		case "int32":
			return "INTEGER"
		default:
			return "BIGINT"
		}

	case "number":
		return "DOUBLE PRECISION"

	case "boolean":
		if g.dialect == "mysql" {
			return "TINYINT(1)"
		}
		return "BOOLEAN"

	case "array":
		if g.dialect == "postgres" {
			// Could use array type, but JSONB is more flexible
			return "JSONB"
		}
		return "TEXT" // Store as JSON string

	case "object":
		if g.dialect == "postgres" {
			return "JSONB"
		} else if g.dialect == "mysql" {
			return "JSON"
		}
		return "TEXT"

	default:
		return "TEXT"
	}
}
