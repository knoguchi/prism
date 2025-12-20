#!/usr/bin/env python3
"""
Analyze how often path segments can be matched to response body field values.
This tests the feasibility of inferring parameter names from response data.
"""

import sqlite3
import json
import re
from collections import defaultdict
from urllib.parse import urlparse

# Known literal path segments (won't be parameters)
KNOWN_LITERALS = {
    'api', 'v1', 'v2', 'v3', 'v4', 'v5', 'graphql', 'rest',
    'users', 'user', 'posts', 'post', 'comments', 'comment',
    'videos', 'video', 'images', 'image', 'files', 'file',
    'auth', 'login', 'logout', 'profile', 'profiles', 'settings',
    'search', 'list', 'create', 'update', 'delete', 'edit',
    'admin', 'public', 'private', 'internal', 'me', 'self',
    'status', 'health', 'ping', 'info', 'docs', 'help',
}

def is_potential_param(segment):
    """Check if a path segment could be a parameter (not a known literal)."""
    if not segment:
        return False
    lower = segment.lower()
    if lower in KNOWN_LITERALS:
        return False
    # Version patterns like v1.2.3
    if re.match(r'^v\d+(\.\d+)*$', lower):
        return False
    return True

def find_value_in_json(obj, target_value, path=""):
    """Recursively search for a value in JSON, return field path if found."""
    matches = []

    if isinstance(obj, dict):
        for key, value in obj.items():
            current_path = f"{path}.{key}" if path else key
            if value == target_value:
                matches.append(current_path)
            elif isinstance(value, str) and str(target_value) == value:
                matches.append(current_path)
            elif isinstance(value, (int, float)) and str(value) == str(target_value):
                matches.append(current_path)
            # Recurse into nested objects/arrays
            matches.extend(find_value_in_json(value, target_value, current_path))
    elif isinstance(obj, list):
        for i, item in enumerate(obj):
            matches.extend(find_value_in_json(item, target_value, f"{path}[{i}]"))

    return matches

def analyze_request(path, response_body):
    """Analyze a single request/response pair for parameter name matches."""
    segments = [s for s in path.split('/') if s]
    potential_params = [(i, s) for i, s in enumerate(segments) if is_potential_param(s)]

    if not potential_params or not response_body:
        return []

    try:
        body = json.loads(response_body)
    except (json.JSONDecodeError, TypeError):
        return []

    matches = []
    for idx, segment in potential_params:
        field_paths = find_value_in_json(body, segment)
        if field_paths:
            # Get the leaf field name (last part of path)
            field_names = [fp.split('.')[-1].split('[')[0] for fp in field_paths]
            matches.append({
                'segment': segment,
                'segment_index': idx,
                'matched_fields': field_paths,
                'inferred_param': field_names[0] if field_names else None
            })

    return matches

def main():
    conn = sqlite3.connect('data/proxy.db')
    cursor = conn.cursor()

    # Get requests with JSON responses
    cursor.execute("""
        SELECT r.path, resp.body
        FROM requests r
        JOIN responses resp ON r.id = resp.request_id
        WHERE resp.content_type LIKE '%json%'
        AND resp.body IS NOT NULL
        AND length(resp.body) > 2
        AND length(resp.body) < 100000
        LIMIT 10000
    """)

    stats = {
        'total_requests': 0,
        'requests_with_potential_params': 0,
        'requests_with_matches': 0,
        'total_potential_params': 0,
        'total_matches': 0,
        'field_names': defaultdict(int),
        'examples': []
    }

    for path, response_body in cursor.fetchall():
        stats['total_requests'] += 1

        segments = [s for s in path.split('/') if s]
        potential_params = [s for s in segments if is_potential_param(s)]

        if potential_params:
            stats['requests_with_potential_params'] += 1
            stats['total_potential_params'] += len(potential_params)

        matches = analyze_request(path, response_body)

        if matches:
            stats['requests_with_matches'] += 1
            stats['total_matches'] += len(matches)

            for m in matches:
                if m['inferred_param']:
                    stats['field_names'][m['inferred_param']] += 1

            # Save some examples
            if len(stats['examples']) < 20:
                stats['examples'].append({
                    'path': path,
                    'matches': matches
                })

    conn.close()

    # Print results
    print("=" * 60)
    print("PATH PARAMETER INFERENCE ANALYSIS")
    print("=" * 60)
    print(f"\nTotal requests analyzed: {stats['total_requests']}")
    print(f"Requests with potential parameters: {stats['requests_with_potential_params']}")
    print(f"Requests where segment matched response field: {stats['requests_with_matches']}")
    print(f"\nTotal potential parameter segments: {stats['total_potential_params']}")
    print(f"Total segments that matched response fields: {stats['total_matches']}")

    if stats['requests_with_potential_params'] > 0:
        match_rate = stats['requests_with_matches'] / stats['requests_with_potential_params'] * 100
        print(f"\nMatch rate: {match_rate:.1f}%")

    if stats['total_potential_params'] > 0:
        segment_match_rate = stats['total_matches'] / stats['total_potential_params'] * 100
        print(f"Segment match rate: {segment_match_rate:.1f}%")

    print("\n" + "-" * 60)
    print("TOP INFERRED PARAMETER NAMES:")
    print("-" * 60)
    for name, count in sorted(stats['field_names'].items(), key=lambda x: -x[1])[:20]:
        print(f"  {name}: {count}")

    print("\n" + "-" * 60)
    print("EXAMPLE MATCHES:")
    print("-" * 60)
    for ex in stats['examples'][:10]:
        print(f"\nPath: {ex['path']}")
        for m in ex['matches']:
            print(f"  Segment '{m['segment']}' → field '{m['inferred_param']}' (found at: {m['matched_fields']})")

if __name__ == '__main__':
    main()
