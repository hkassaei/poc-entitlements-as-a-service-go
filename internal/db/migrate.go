package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate reads and executes the schema.sql file to bootstrap the database.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	schemaPath := findSchemaPath()
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema.sql: %w", err)
	}

	_, err = pool.Exec(ctx, string(schema))
	if err != nil {
		return fmt.Errorf("execute schema.sql: %w", err)
	}

	return nil
}

// findSchemaPath locates sql/schema.sql relative to this file or the working directory.
func findSchemaPath() string {
	// Try relative to this source file first (for tests)
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		dir := filepath.Dir(filename)
		candidate := filepath.Join(dir, "..", "..", "sql", "schema.sql")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Fall back to working directory
	return filepath.Join("sql", "schema.sql")
}
