package models

import (
	"time"
)

// SchemaFormat represents the output format for inferred schemas
type SchemaFormat string

const (
	SchemaFormatOpenAPI    SchemaFormat = "openapi"
	SchemaFormatProtobuf   SchemaFormat = "protobuf"
	SchemaFormatAvro       SchemaFormat = "avro"
	SchemaFormatSQL        SchemaFormat = "sql"
	SchemaFormatJSONSchema SchemaFormat = "json-schema"
	SchemaFormatTypeScript SchemaFormat = "typescript"
	SchemaFormatGo         SchemaFormat = "go"
	SchemaFormatGraphQL    SchemaFormat = "graphql"
)

// InferredSchema represents a schema inferred from captured traffic
type InferredSchema struct {
	ID          int64        `json:"id"`
	Host        string       `json:"host"`
	Method      string       `json:"method"`
	PathPattern string       `json:"path_pattern"`
	Format      SchemaFormat `json:"format"`
	Content     string       `json:"content"`
	SampleCount int          `json:"sample_count"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// EndpointPattern represents a detected API endpoint pattern
type EndpointPattern struct {
	ID              int64                `json:"id"`
	Host            string               `json:"host"`
	Method          string               `json:"method"`
	PathPattern     string               `json:"path_pattern"`
	PathRegex       string               `json:"path_regex"`
	RequestSchema   *JSONSchema          `json:"request_schema,omitempty"`
	ResponseSchemas map[int]*JSONSchema  `json:"response_schemas,omitempty"` // keyed by status code
	QueryParams     []*QueryParam        `json:"query_params,omitempty"`
	AuthType        string               `json:"auth_type,omitempty"`
	SampleCount     int                  `json:"sample_count"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// QueryParam represents a query parameter in an endpoint
type QueryParam struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Example  string `json:"example,omitempty"`
}

// JSONSchema represents an inferred JSON schema
type JSONSchema struct {
	Type        string                 `json:"type"`
	Format      string                 `json:"format,omitempty"`
	Properties  map[string]*JSONSchema `json:"properties,omitempty"`
	Items       *JSONSchema            `json:"items,omitempty"`
	Required    []string               `json:"required,omitempty"`
	Enum        []interface{}          `json:"enum,omitempty"`
	Description string                 `json:"description,omitempty"`
	Example     interface{}            `json:"example,omitempty"`
	Nullable    bool                   `json:"nullable,omitempty"`
	MinValue    *float64               `json:"minimum,omitempty"`
	MaxValue    *float64               `json:"maximum,omitempty"`
}

// SchemaListItem is a summary for listing schemas
type SchemaListItem struct {
	Host      string             `json:"host"`
	Endpoints []*EndpointSummary `json:"endpoints"`
}

// EndpointSummary is a brief summary of an endpoint
type EndpointSummary struct {
	Method      string   `json:"method"`
	PathPattern string   `json:"path_pattern"`
	SampleCount int      `json:"sample_count"`
	Formats     []string `json:"formats"`
}
