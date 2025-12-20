package inference

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ParamMatch represents a matched path parameter from response body analysis
type ParamMatch struct {
	SegmentIndex int      // Index of the segment in the path
	SegmentValue string   // The actual value in the path
	FieldName    string   // The inferred parameter name from response
	FieldPaths   []string // Full paths where the value was found (e.g., "data.username")
}

// InferParamNamesFromResponse analyzes a path and response body to infer parameter names.
// It looks for path segment values that appear in the response JSON and uses the field name.
//
// Example:
//   Path: /v3/profile/kenji
//   Response: {"data": {"username": "kenji", "id": 123}}
//   Returns: [{SegmentIndex: 2, SegmentValue: "kenji", FieldName: "username", FieldPaths: ["data.username"]}]
func InferParamNamesFromResponse(path string, responseBody []byte) []ParamMatch {
	if len(responseBody) == 0 {
		return nil
	}

	// Parse response JSON
	var body interface{}
	if err := json.Unmarshal(responseBody, &body); err != nil {
		return nil
	}

	segments := strings.Split(strings.Trim(path, "/"), "/")
	var matches []ParamMatch

	for i, segment := range segments {
		// Skip known literals and empty segments
		if segment == "" || isKnownLiteral(segment) {
			continue
		}

		// Skip version prefixes
		if versionRegex.MatchString(strings.ToLower(segment)) {
			continue
		}

		// Search for this value in the response
		fieldPaths := findValueInJSON(body, segment, "")
		if len(fieldPaths) > 0 {
			// Extract the leaf field name from the first match
			fieldName := extractFieldName(fieldPaths[0])
			matches = append(matches, ParamMatch{
				SegmentIndex: i,
				SegmentValue: segment,
				FieldName:    fieldName,
				FieldPaths:   fieldPaths,
			})
		}
	}

	return matches
}

// findValueInJSON recursively searches for a value in JSON and returns all field paths where it's found
func findValueInJSON(obj interface{}, targetValue string, currentPath string) []string {
	var matches []string

	switch v := obj.(type) {
	case map[string]interface{}:
		for key, value := range v {
			newPath := key
			if currentPath != "" {
				newPath = currentPath + "." + key
			}

			// Check if this field's value matches
			if matchesValue(value, targetValue) {
				matches = append(matches, newPath)
			}

			// Recurse into nested objects/arrays
			matches = append(matches, findValueInJSON(value, targetValue, newPath)...)
		}

	case []interface{}:
		for i, item := range v {
			newPath := fmt.Sprintf("%s[%d]", currentPath, i)
			if currentPath == "" {
				newPath = fmt.Sprintf("[%d]", i)
			}
			matches = append(matches, findValueInJSON(item, targetValue, newPath)...)
		}
	}

	return matches
}

// matchesValue checks if a JSON value matches the target string
func matchesValue(value interface{}, target string) bool {
	switch v := value.(type) {
	case string:
		return v == target
	case float64:
		// Handle numeric IDs - compare as string
		// Integer check
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10) == target
		}
		return strconv.FormatFloat(v, 'f', -1, 64) == target
	case int:
		return strconv.Itoa(v) == target
	case int64:
		return strconv.FormatInt(v, 10) == target
	case bool:
		return strconv.FormatBool(v) == target
	}
	return false
}

// extractFieldName gets the leaf field name from a path like "data.user.username"
func extractFieldName(fieldPath string) string {
	// Remove array indices
	path := fieldPath
	for strings.Contains(path, "[") {
		start := strings.LastIndex(path, "[")
		end := strings.Index(path[start:], "]")
		if end > 0 {
			path = path[:start] + path[start+end+1:]
		} else {
			break
		}
	}

	// Get last segment
	parts := strings.Split(path, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

// RefinePathPattern takes a pattern with generic names like {id} and refines them
// using matched parameter names from response analysis.
//
// Example:
//   pattern: /v3/profile/{slug}
//   matches: [{SegmentIndex: 2, FieldName: "username"}]
//   Returns: /v3/profile/{username}
func RefinePathPattern(pattern string, matches []ParamMatch) string {
	if len(matches) == 0 {
		return pattern
	}

	segments := strings.Split(strings.Trim(pattern, "/"), "/")

	for _, match := range matches {
		if match.SegmentIndex < len(segments) {
			seg := segments[match.SegmentIndex]
			// Only replace if it's a parameter placeholder
			if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
				segments[match.SegmentIndex] = "{" + match.FieldName + "}"
			}
		}
	}

	return "/" + strings.Join(segments, "/")
}

// AggregateParamMatches combines matches from multiple request/response pairs
// and returns the most common field name for each segment position.
//
// This handles cases where different responses might use slightly different field names.
func AggregateParamMatches(allMatches [][]ParamMatch) map[int]string {
	// Count field names per segment index
	counts := make(map[int]map[string]int)

	for _, matches := range allMatches {
		for _, m := range matches {
			if counts[m.SegmentIndex] == nil {
				counts[m.SegmentIndex] = make(map[string]int)
			}
			counts[m.SegmentIndex][m.FieldName]++
		}
	}

	// Pick the most common field name for each segment
	result := make(map[int]string)
	for segIdx, nameCounts := range counts {
		var bestName string
		var bestCount int
		for name, count := range nameCounts {
			if count > bestCount {
				bestName = name
				bestCount = count
			}
		}
		if bestName != "" {
			result[segIdx] = bestName
		}
	}

	return result
}
