package stats

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// validateDataDirPath checks if a path is within the data directory.
// This prevents directory traversal for programmatically constructed paths.
func validateDataDirPath(dataDir, filePath string) error {
	// Convert both to absolute paths
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("cannot resolve data directory: %w", err)
	}
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("cannot resolve file path: %w", err)
	}

	// Check if file path is within data directory
	relPath, err := filepath.Rel(absDataDir, absFilePath)
	if err != nil {
		return fmt.Errorf("cannot determine relative path: %w", err)
	}

	// If relative path starts with "..", it's outside the data directory
	if strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("file path is outside data directory: %s", filePath)
	}

	return nil
}

// MigrateFromFiles migrates data from NDJSON and JSON files to SQLite database.
//
// This function reads existing requests.ndjson and stats.json files from the
// data directory and imports them into the SQLite database. It should be called
// once during the transition from file-based to database-based persistence.
//
// The migration process:
// 1. Checks if NDJSON/JSON files exist
// 2. Reads requests from requests.ndjson
// 3. Reads aggregated stats from stats.json
// 4. Imports all data into SQLite database
// 5. Backs up original files (renamed with .migrated extension)
//
// Parameters:
//   - db: the database instance to import data into
//   - dataDir: directory containing the NDJSON/JSON files
//   - logger: structured logger for migration progress
//
// Returns an error if migration fails, or nil if successful or no files to migrate.
func MigrateFromFiles(db *Database, dataDir string, logger *slog.Logger) error {
	if dataDir == "" {
		return nil // No data directory configured
	}

	requestsFile := filepath.Join(dataDir, "requests.ndjson")
	statsFile := filepath.Join(dataDir, "stats.json")

	// Check if files exist
	requestsExist := fileExists(requestsFile)
	statsExist := fileExists(statsFile)

	if !requestsExist && !statsExist {
		logger.Debug("No existing data files to migrate")
		return nil
	}

	logger.Info("Starting migration from files to SQLite", "requestsFile", requestsExist, "statsFile", statsExist)

	// Migrate requests from NDJSON
	if requestsExist {
		if err := migrateRequestsNDJSON(db, requestsFile, logger); err != nil {
			return fmt.Errorf("failed to migrate requests: %w", err)
		}
		// Backup original file
		if err := os.Rename(requestsFile, requestsFile+".migrated"); err != nil {
			logger.Warn("Failed to backup requests file", "error", err)
		} else {
			logger.Info("Backed up requests file", "backup", requestsFile+".migrated")
		}
	}

	// Migrate aggregated stats from JSON (if we didn't already have data from NDJSON)
	if statsExist {
		if err := migrateStatsJSON(db, statsFile, logger); err != nil {
			return fmt.Errorf("failed to migrate stats: %w", err)
		}
		// Backup original file
		if err := os.Rename(statsFile, statsFile+".migrated"); err != nil {
			logger.Warn("Failed to backup stats file", "error", err)
		} else {
			logger.Info("Backed up stats file", "backup", statsFile+".migrated")
		}
	}

	logger.Info("Migration completed successfully")
	return nil
}

// migrateRequestsNDJSON reads requests from NDJSON file and imports into database.
func migrateRequestsNDJSON(db *Database, filename string, logger *slog.Logger) error {
	// Validate path - extract dataDir from filename (it's constructed as filepath.Join(dataDir, "requests.ndjson"))
	dataDir := filepath.Dir(filename)
	if err := validateDataDirPath(dataDir, filename); err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	// #nosec G304 -- path validated by validateDataDirPath to prevent traversal
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open requests file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0

	// Read and import each request
	for scanner.Scan() {
		var req RequestInfo
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			logger.Warn("Failed to parse request line", "error", err)
			continue
		}

		if err := db.RecordRequest(req); err != nil {
			logger.Warn("Failed to import request", "error", err)
			continue
		}

		count++
		if count%1000 == 0 {
			logger.Debug("Migration progress", "requests", count)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading requests file: %w", err)
	}

	logger.Info("Migrated requests from NDJSON", "count", count)
	return nil
}

// migrateStatsJSON reads aggregated stats from JSON file and updates database.
//
// Note: This is mainly for preserving the start time if NDJSON migration didn't
// provide it. The counts will be recalculated from the request log.
func migrateStatsJSON(db *Database, filename string, logger *slog.Logger) error {
	// Validate path - extract dataDir from filename (it's constructed as filepath.Join(dataDir, "stats.json"))
	dataDir := filepath.Dir(filename)
	if err := validateDataDirPath(dataDir, filename); err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	// #nosec G304 -- path validated by validateDataDirPath to prevent traversal
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read stats file: %w", err)
	}

	var persisted PersistedStats
	if err := json.Unmarshal(data, &persisted); err != nil {
		return fmt.Errorf("failed to unmarshal stats: %w", err)
	}

	// Update start time in database
	_, err = db.db.Exec(`
		UPDATE stats
		SET start_time = ?
		WHERE id = 1 AND start_time > ?
	`, persisted.StartTime, persisted.StartTime)
	if err != nil {
		return fmt.Errorf("failed to update start time: %w", err)
	}

	logger.Info("Migrated stats from JSON", "startTime", persisted.StartTime)
	return nil
}

// fileExists checks if a file exists.
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}
