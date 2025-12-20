package models

// Stats represents overall traffic statistics
type Stats struct {
	TotalRequests          int64            `json:"total_requests"`
	TotalResponses         int64            `json:"total_responses"`
	TotalWebSocketMessages int64            `json:"total_websocket_messages"`
	StorageSizeBytes       int64            `json:"storage_size_bytes"`
	ByMethod               map[string]int64 `json:"by_method"`
	ByStatus               map[string]int64 `json:"by_status"`
	TopHosts               []*HostCount     `json:"top_hosts"`
	AverageLatencyMs       float64          `json:"average_latency_ms"`
}

// HostCount represents a host and its request count
type HostCount struct {
	Host  string `json:"host"`
	Count int64  `json:"count"`
}

// TimelinePoint represents a point on the traffic timeline
type TimelinePoint struct {
	Timestamp string `json:"timestamp"`
	Count     int64  `json:"count"`
	AvgLatency float64 `json:"avg_latency_ms,omitempty"`
}

// AuthAnalysis represents authentication pattern analysis
type AuthAnalysis struct {
	Host        string        `json:"host,omitempty"`
	AuthSchemes []*AuthScheme `json:"auth_schemes"`
}

// AuthScheme represents a detected authentication scheme
type AuthScheme struct {
	Type        string   `json:"type"` // "bearer", "api_key", "basic", "cookie"
	Location    string   `json:"location"` // "header", "query", "cookie"
	Name        string   `json:"name"` // header name or query param
	TokenCount  int      `json:"token_count"`
	SampleToken string   `json:"sample_token,omitempty"` // masked
	JWTAnalysis *JWTInfo `json:"jwt_analysis,omitempty"`
}

// JWTInfo represents analyzed JWT token information
type JWTInfo struct {
	Algorithm  string                 `json:"algorithm"`
	Issuer     string                 `json:"issuer,omitempty"`
	Audience   string                 `json:"audience,omitempty"`
	Subject    string                 `json:"subject,omitempty"`
	Claims     map[string]interface{} `json:"claims,omitempty"`
	ExpiresAt  string                 `json:"expires_at,omitempty"`
	IsExpired  bool                   `json:"is_expired"`
}
