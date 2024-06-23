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

	SyncReplyRecord(record *ReplyRecord) error

	GetFloorRecord(bid string, floorIndex int) (*FloorRecord, error)
	UpdateFloorRecordContent(fid, content string) error
	CreateFloorRecord(record *FloorRecord) error

	GetPageRecord(bid string, pageIndex int) (*PageRecord, error)
	CreatePageRecord(record *PageRecord) error

	GetBuildingRecord(bsn, sna int) (*BuildingRecord, error)
	UpdateBuildingRecord(record *BuildingRecord) error
	CreateBuildingRecord(record *BuildingRecord) error
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
			page_index INTEGER NOT NULL,
			PRIMARY KEY (bid, pid, page_index),
			FOREIGN KEY (bid) REFERENCES building_record(id)
		);`,
		`CREATE TABLE IF NOT EXISTS building_record (
			id TEXT PRIMARY KEY,
			bsn INTEGER NOT NULL,
			sna INTEGER NOT NULL,
			building_title TEXT NOT NULL,
			last_page_index INTEGER
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
	} else {
		driver, err := sql.Open("sqlite3", buildingDbPath)
		if err != nil {
			logrus.WithError(err).Error("sql.Open failed")
			return err
		}
		db.driver = driver
	}
	return nil
}

func (db *BuildingDb) getReplyRecord(fid string, replyIndex int) (*ReplyRecord, error) {
	query := `SELECT author_name, author_id, content FROM reply_record WHERE fid = ? AND reply_index = ?;`

	var record ReplyRecord
	if err := db.driver.QueryRow(query, fid, replyIndex).Scan(
		&record.AuthorName, &record.AuthorId, &record.Content); err != nil {

		if err != sql.ErrNoRows {
			logrus.WithError(err).Error("db.driver.QueryRow.Scan failed")
		}

		return nil, err
	}
	return &record, nil
}

func (db *BuildingDb) updateReplyRecordContent(fid string, replyIndex int, content string) error {
	stat := `UPDATE reply_record SET content = ? WHERE fid = ? AND reply_index = ?;`

	if _, err := db.driver.Exec(
		stat,
		content, replyIndex, fid); err != nil {

		logrus.WithError(err).Error("db.driver.Exec failed")
		return err
	}
	return nil
}

func (db *BuildingDb) createReplyRecord(record *ReplyRecord) error {
	stat := `INSERT INTO reply_record (fid, reply_index, author_name, author_id, content) VALUES (?, ?, ?, ?, ?);`

	if _, err := db.driver.Exec(
		stat,
		record.Fid, record.ReplyIndex,
		record.AuthorName, record.AuthorId, record.Content); err != nil {

		logrus.WithError(err).Error("db.driver.Exec failed")
		return err
	}
	return nil
}

func (db *BuildingDb) SyncReplyRecord(record *ReplyRecord) error {
	rr, err := db.getReplyRecord(record.Fid, record.ReplyIndex)
	if err != nil {
		if err != sql.ErrNoRows {
			logrus.WithError(err).Error("getReplyRecord failed")
			return err
		}

		// Handle sql.ErrNoRows
		if err := db.createReplyRecord(record); err != nil {
			logrus.WithError(err).Error("createReplyRecord failed")
			return err
		}
		return nil
	}

	if rr.Content != record.Content || rr.ReplyIndex != record.ReplyIndex {
		if err := db.updateReplyRecordContent(record.Fid, record.ReplyIndex, record.Content); err != nil {
			logrus.WithError(err).Error("updateReplyRecordContent failed")
			return err
		}
	}

	return nil
}

func (db *BuildingDb) GetFloorRecord(bid string, floorIndex int) (*FloorRecord, error) {
	query := `SELECT pid, fid, author_name, author_id, content FROM floor_record WHERE bid = ? AND floor_index = ?;`

	var record FloorRecord
	if err := db.driver.QueryRow(query, bid, floorIndex).Scan(
		&record.Pid, &record.Fid,
		&record.AuthorName, &record.AuthorId, &record.Content); err != nil {

		if err != sql.ErrNoRows {
			logrus.WithError(err).Error("db.driver.QueryRow.Scan failed")
		}

		return nil, err
	}
	return &record, nil
}

// Only content has possibility to be updated
// Another fields are not allowed to be updated
func (db *BuildingDb) UpdateFloorRecordContent(fid, content string) error {
	stat := `UPDATE floor_record SET content = ? WHERE fid = ?;`

	if _, err := db.driver.Exec(
		stat,
		content, fid); err != nil {

		logrus.WithError(err).Error("db.driver.Exec failed")
		return err
	}
	return nil
}

func (db *BuildingDb) CreateFloorRecord(record *FloorRecord) error {
	stat := `INSERT INTO floor_record (bid, pid, fid, floor_index, author_name, author_id, content) VALUES (?, ?, ?, ?, ?, ?, ?);`

	if _, err := db.driver.Exec(
		stat,
		record.Bid, record.Pid, record.Fid, record.FloorIndex,
		record.AuthorName, record.AuthorId, record.Content); err != nil {

		logrus.WithError(err).Error("db.driver.Exec failed")
		return err
	}
	return nil
}

func (db *BuildingDb) GetPageRecord(bid string, pageIndex int) (*PageRecord, error) {
	query := `SELECT pid FROM page_record WHERE bid = ? AND page_index = ?;`

	var record PageRecord
	if err := db.driver.QueryRow(query, bid, pageIndex).Scan(
		&record.Pid); err != nil {

		if err != sql.ErrNoRows {
			logrus.WithError(err).Error("db.driver.QueryRow.Scan failed")
		}

		return nil, err
	}
	return &record, nil
}

func (db *BuildingDb) CreatePageRecord(record *PageRecord) error {
	stat := `INSERT INTO page_record (bid, pid, page_index) VALUES (?, ?, ?);`

	if _, err := db.driver.Exec(
		stat,
		record.Bid, record.Pid, record.PageIndex); err != nil {

		logrus.WithError(err).Error("db.driver.Exec failed")
		return err
	}
	return nil

}

func (db *BuildingDb) GetBuildingRecord(bsn, sna int) (*BuildingRecord, error) {
	query := `SELECT id, building_title, last_page_index FROM building_record WHERE bsn = ? AND sna = ?;`

	var record BuildingRecord
	if err := db.driver.QueryRow(query, bsn, sna).Scan(
		&record.Id,
		&record.BuildingTitle,
		&record.LastPageIndex); err != nil {

		if err != sql.ErrNoRows {
			logrus.WithError(err).Error("db.driver.QueryRow.Scan failed")
		}

		return nil, err
	}
	return &record, nil
}

func (db *BuildingDb) UpdateBuildingRecord(record *BuildingRecord) error {
	stat := `UPDATE building_record SET building_title = ?, last_page_index = ? WHERE id = ?;`

	if _, err := db.driver.Exec(
		stat,
		record.BuildingTitle,
		record.LastPageIndex,
		record.Id); err != nil {

		logrus.WithError(err).Error("db.driver.Exec failed")
		return err
	}
	return nil
}

func (db *BuildingDb) CreateBuildingRecord(record *BuildingRecord) error {
	stat := `INSERT INTO building_record (id, bsn, sna, building_title, last_page_index) VALUES (?, ?, ?, ?, ?);`

	if _, err := db.driver.Exec(
		stat,
		record.Id,
		record.Bsn, record.Sna,
		record.BuildingTitle,
		record.LastPageIndex); err != nil {

		logrus.WithError(err).Error("db.driver.Exec failed")
		return err
	}
	return nil
}
