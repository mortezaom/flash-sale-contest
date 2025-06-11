package database

import (
	"embed"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func (s *service) RunMigrations() error {
	if err := s.createMigrationsTable(); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	files, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrationNames []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".sql" {
			migrationNames = append(migrationNames, file.Name())
		}
	}
	sort.Strings(migrationNames)

	for _, filename := range migrationNames {
		if err := s.runMigration(filename); err != nil {
			return fmt.Errorf("failed to run migration %s: %w", filename, err)
		}
	}

	return nil
}

func (s *service) createMigrationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`
	_, err := s.db.Exec(query)
	return err
}

func (s *service) runMigration(filename string) error {
	migrationName := strings.TrimSuffix(filename, ".sql")

	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", migrationName).Scan(&exists)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	content, err := migrationFiles.ReadFile("migrations/" + filename)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(string(content)); err != nil {
		return fmt.Errorf("migration %s failed: %w", filename, err)
	}

	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", migrationName); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("Applied migration: %s", filename)
	return nil
}
