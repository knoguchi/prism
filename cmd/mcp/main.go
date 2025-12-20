package main

import (
	"flag"
	"fmt"
	"os"

	"ai-proxy/internal/mcp"
	"ai-proxy/internal/storage"
)

func main() {
	// Parse command line flags
	dbPath := flag.String("db", "./data/proxy.db", "Path to SQLite database")
	flag.Parse()

	// Open database
	db, err := storage.New(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create and run MCP server
	server := mcp.NewServer(db)
	if err := server.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}
