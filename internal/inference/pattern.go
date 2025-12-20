package inference

import (
	"regexp"
	"strconv"
	"strings"
)

// PathSegmentType identifies what kind of path segment we're dealing with
type PathSegmentType int

const (
	SegmentLiteral PathSegmentType = iota
	SegmentUUID
	SegmentULID
	SegmentInteger
	SegmentSlug
	SegmentDate
	SegmentTimestamp
)

// DetectPathPattern converts a concrete path to a parameterized pattern
// e.g., "/users/123/posts/abc-def" -> "/users/{id}/posts/{slug}"
func DetectPathPattern(path string) string {
	if path == "" || path == "/" {
		return path
	}

	segments := strings.Split(strings.Trim(path, "/"), "/")
	result := make([]string, len(segments))

	for i, segment := range segments {
		segType := classifySegment(segment)
		result[i] = patternForSegment(segType, segment)
	}

	return "/" + strings.Join(result, "/")
}

// classifySegment determines what type of segment this is
func classifySegment(segment string) PathSegmentType {
	if segment == "" {
		return SegmentLiteral
	}

	// Check for known literal patterns first (versions, actions, etc.)
	if isKnownLiteral(segment) {
		return SegmentLiteral
	}

	// UUID (v1, v4, v5, v7) - 36 chars with dashes
	if uuidPathRegex.MatchString(segment) {
		return SegmentUUID
	}

	// ULID - 26 chars, Crockford base32
	if ulidRegex.MatchString(segment) {
		return SegmentULID
	}

	// ISO Date (YYYY-MM-DD)
	if isoDateRegex.MatchString(segment) {
		return SegmentDate
	}

	// Unix timestamp (10 or 13 digits)
	if timestampRegex.MatchString(segment) {
		return SegmentTimestamp
	}

	// Pure integer
	if integerRegex.MatchString(segment) {
		return SegmentInteger
	}

	// Slug (alphanumeric with dashes/underscores, but not purely alpha)
	if slugRegex.MatchString(segment) && hasDigit(segment) {
		return SegmentSlug
	}

	// Default to literal if it looks like a word
	return SegmentLiteral
}

// patternForSegment returns the parameter pattern for a segment type
func patternForSegment(segType PathSegmentType, original string) string {
	switch segType {
	case SegmentUUID:
		return "{uuid}"
	case SegmentULID:
		return "{ulid}"
	case SegmentInteger:
		return "{id}"
	case SegmentDate:
		return "{date}"
	case SegmentTimestamp:
		return "{timestamp}"
	case SegmentSlug:
		return "{slug}"
	default:
		return original
	}
}

// isKnownLiteral checks if segment matches known literal patterns
func isKnownLiteral(segment string) bool {
	lower := strings.ToLower(segment)

	// API versions
	if versionRegex.MatchString(lower) {
		return true
	}

	// Common literal path segments
	literals := map[string]bool{
		"api": true, "graphql": true, "rest": true,
		"admin": true, "public": true, "private": true, "internal": true,
		"auth": true, "login": true, "logout": true, "register": true, "signup": true,
		"users": true, "user": true, "accounts": true, "account": true,
		"posts": true, "post": true, "articles": true, "article": true,
		"comments": true, "comment": true, "replies": true, "reply": true,
		"products": true, "product": true, "items": true, "item": true,
		"orders": true, "order": true, "cart": true, "checkout": true,
		"search": true, "filter": true, "sort": true, "page": true,
		"list": true, "create": true, "update": true, "delete": true,
		"new": true, "edit": true, "view": true, "show": true,
		"index": true, "home": true, "dashboard": true,
		"settings": true, "config": true, "preferences": true,
		"profile": true, "profiles": true, "me": true,
		"files": true, "file": true, "uploads": true, "upload": true,
		"images": true, "image": true, "media": true, "assets": true,
		"docs": true, "documentation": true, "help": true,
		"status": true, "health": true, "ping": true, "info": true,
		"webhook": true, "webhooks": true, "callback": true, "callbacks": true,
		"events": true, "event": true, "notifications": true, "notification": true,
		"messages": true, "message": true, "inbox": true, "outbox": true,
		"groups": true, "group": true, "teams": true, "team": true,
		"roles": true, "role": true, "permissions": true, "permission": true,
		"tags": true, "tag": true, "categories": true, "category": true,
		"reports": true, "report": true, "analytics": true, "stats": true,
		"export": true, "import": true, "download": true,
		"batch": true, "bulk": true,
		"ws": true, "socket": true, "stream": true,
	}

	return literals[lower]
}

// MatchPaths checks if two paths match under the same pattern
func MatchPaths(path1, path2 string) bool {
	pattern1 := DetectPathPattern(path1)
	pattern2 := DetectPathPattern(path2)
	return pattern1 == pattern2
}

// ExtractPathParams extracts parameter values from a path given a pattern
func ExtractPathParams(path, pattern string) map[string]string {
	pathSegs := strings.Split(strings.Trim(path, "/"), "/")
	patternSegs := strings.Split(strings.Trim(pattern, "/"), "/")

	if len(pathSegs) != len(patternSegs) {
		return nil
	}

	params := make(map[string]string)
	for i, patSeg := range patternSegs {
		if strings.HasPrefix(patSeg, "{") && strings.HasSuffix(patSeg, "}") {
			paramName := patSeg[1 : len(patSeg)-1]
			params[paramName] = pathSegs[i]
		} else if patSeg != pathSegs[i] {
			return nil // Doesn't match
		}
	}

	return params
}

// GetPathParamType returns the OpenAPI type/format for a path parameter
func GetPathParamType(paramName string) (schemaType string, format string) {
	switch paramName {
	case "uuid":
		return "string", "uuid"
	case "ulid":
		return "string", ""
	case "id":
		return "integer", "int64"
	case "date":
		return "string", "date"
	case "timestamp":
		return "integer", "int64"
	case "slug":
		return "string", ""
	default:
		return "string", ""
	}
}

// DetectUUIDVersion attempts to detect the UUID version
func DetectUUIDVersion(uuid string) string {
	if len(uuid) != 36 {
		return ""
	}

	// Version is the first character of the 3rd group (position 14)
	versionChar := uuid[14]

	switch versionChar {
	case '1':
		return "v1" // Time-based
	case '2':
		return "v2" // DCE Security
	case '3':
		return "v3" // Name-based MD5
	case '4':
		return "v4" // Random
	case '5':
		return "v5" // Name-based SHA1
	case '6':
		return "v6" // Reordered time-based
	case '7':
		return "v7" // Unix epoch time-based
	case '8':
		return "v8" // Custom
	default:
		return ""
	}
}

// IsMonotonicID checks if a series of IDs appears monotonically increasing
// This helps detect auto-increment vs random IDs
func IsMonotonicID(ids []string) bool {
	if len(ids) < 2 {
		return false
	}

	var prev int64
	for i, idStr := range ids {
		val, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return false
		}
		if i > 0 && val <= prev {
			return false
		}
		prev = val
	}
	return true
}

// hasDigit checks if string contains at least one digit
func hasDigit(s string) bool {
	for _, c := range s {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

// Path segment detection regexes
var (
	// UUID: 8-4-4-4-12 hex chars
	uuidPathRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// ULID: 26 chars, Crockford's Base32
	ulidRegex = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

	// Version patterns: v1, v2, v1.0, v2.1.3
	versionRegex = regexp.MustCompile(`^v\d+(\.\d+)*$`)

	// ISO date: YYYY-MM-DD
	isoDateRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

	// Unix timestamp: 10 digits (seconds) or 13 digits (milliseconds)
	timestampRegex = regexp.MustCompile(`^(\d{10}|\d{13})$`)

	// Pure integer
	integerRegex = regexp.MustCompile(`^\d+$`)

	// Slug: alphanumeric with dashes/underscores
	slugRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
)
