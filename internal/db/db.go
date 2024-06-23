package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

const (
	buildingDbPath = "data/building.db"
)

type BuildingDB interface {
	Open() error

	Select(filter map[string]string) ([]*BuildingRecord, error)

	TaskBuildingId(bsn, sna int) (string, error)
}

type BuildingDb struct {
	driver *sql.DB
}

func NewBuildingDb() BuildingDB {
	return &BuildingDb{}
}

var (
	tableCreateStatements = []string{
		`CREATE TABLE IF NOT EXISTS reply_record (
			fid TEXT NOT NULL,
			reply_index INTEGER NOT NULL,
			author_name TEXT NOT NULL,
			author_id TEXT NOT NULL,
			content TEXT NOT NULL,
			PRIMARY KEY (fid, reply_index)
		);`,
		`CREATE TABLE IF NOT EXISTS floor_record (
			bid TEXT NOT NULL,
			pid TEXT NOT NULL,
			fid TEXT NOT NULL,
			floor_index INTEGER NOT NULL,
			author_name TEXT NOT NULL,
			author_id TEXT NOT NULL,
			content TEXT NOT NULL,
			PRIMARY KEY (bid, pid, fid),
			FOREIGN KEY (bid) REFERENCES building_record(id)
		);`,
		`CREATE TABLE IF NOT EXISTS page_record (
			bid TEXT NOT NULL,
			pid TEXT NOT NULL,
			PRIMARY KEY (bid, pid),
			FOREIGN KEY (bid) REFERENCES building_record(id)
		);`,
		`CREATE TABLE IF NOT EXISTS building_record (
			id TEXT PRIMARY KEY,
			bsn INTEGER NOT NULL,
			sna INTEGER NOT NULL,
			building_title TEXT NOT NULL,
			poster_floor_fid TEXT,
			last_page_index INTEGER,
			FOREIGN KEY (poster_floor_fid) REFERENCES floor_record(fid)
		);`,
	}
)

func (db *BuildingDb) initialSqliteDb(path string) error {
	dbPath, err := filepath.Abs(path)
	if err != nil {
		logrus.WithError(err).Error("filepath.Abs failed")
		return err
	}
	logrus.Infof("dbPath: %s", dbPath)

	// create db file
	if _, err := os.Create(dbPath); err != nil {
		logrus.WithError(err).Error("os.Create failed")
		return err
	}
	logrus.WithField("BuildingDbPath", dbPath).Info("Create db file success")

	driver, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		logrus.WithError(err).Error("sql.Open failed")
		return err
	}
	db.driver = driver
	logrus.WithField("BuildingDbPath", dbPath).Info("sql.Open success")

	for _, statement := range tableCreateStatements {
		if _, err := db.driver.Exec(statement); err != nil {
			logrus.WithError(err).Error("db.driver.Exec failed")
			return err
		}
	}
	return nil
}

func ensureDirectoryExists(path string) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logrus.Infof("Directory %s not exist, create it", dir)
		if err = os.MkdirAll(dir, 0755); err != nil {
			logrus.WithError(err).Error("os.MkdirAll")
			return err
		}
		logrus.Infof("Success create directory %s for building.db", dir)
	}
	return nil
}

func (db *BuildingDb) Open() error {
	if err := ensureDirectoryExists(buildingDbPath); err != nil {
		logrus.WithError(err).Error("ensureDirectoryExists failed")
		return err
	}

	// check path exist
	if _, err := os.Stat(buildingDbPath); os.IsNotExist(err) {
		if err = db.initialSqliteDb(buildingDbPath); err != nil {
			logrus.WithError(err).Error("initialSqliteDb failed")
			return err
		}
	}
	return nil
}

func (db *BuildingDb) Select(filter map[string]string) ([]*BuildingRecord, error) {
	return nil, nil
}

func (db *BuildingDb) TaskBuildingId(bsn, sna int) (string, error) {
	query := `SELECT id FROM building_record WHERE bsn = ? AND sna = ?;`

	var id string
	if err := db.driver.QueryRow(query, bsn, sna).Scan(&id); err != nil {
		logrus.WithError(err).Error("db.driver.QueryRow.Scan failed")
		return "", err
	}
	return id, nil
}
