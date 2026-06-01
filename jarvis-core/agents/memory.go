package agents

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

// SensorLog represents a recorded state from one of the ESP32 sensors or kettle.
type SensorLog struct {
	ID        int
	Sensor    string
	Value     float64
	Timestamp time.Time
}

// GateQuestion represents a study card for the GATE instrumentation preparation.
type GateQuestion struct {
	ID         int
	Topic      string
	Question   string
	Answer     string
	Difficulty string
}

// MemoryAgent acts as a clean interface to the local SQLite database.
type MemoryAgent struct {
	db *sql.DB
}

// NewMemoryAgent opens a SQLite connection and initializes the database tables.
func NewMemoryAgent(dbPath string) (*MemoryAgent, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite connection failed: %w", err)
	}

	agent := &MemoryAgent{db: db}
	if err := agent.initSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite init schema failed: %w", err)
	}

	log.Printf("[MemoryAgent] Connected to database at: %s", dbPath)
	return agent, nil
}

// Close closes the database connection.
func (m *MemoryAgent) Close() error {
	return m.db.Close()
}

func (m *MemoryAgent) initSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS sensor_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sensor TEXT NOT NULL,
			value REAL NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS gate_notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			topic TEXT NOT NULL,
			question TEXT NOT NULL,
			answer TEXT NOT NULL,
			difficulty TEXT DEFAULT 'medium'
		);`,
	}

	for _, query := range queries {
		if _, err := m.db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

// SaveSetting writes a key-value setting.
func (m *MemoryAgent) SaveSetting(key, val string) error {
	query := `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value;`
	_, err := m.db.Exec(query, key, val)
	return err
}

// GetSetting retrieves a configuration value. Returns empty string if not found.
func (m *MemoryAgent) GetSetting(key string) (string, error) {
	var val string
	err := m.db.QueryRow(`SELECT value FROM settings WHERE key = ?;`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

// LogSensor logs a reading from local sensors.
func (m *MemoryAgent) LogSensor(sensor string, value float64) error {
	_, err := m.db.Exec(`INSERT INTO sensor_logs (sensor, value) VALUES (?, ?);`, sensor, value)
	return err
}

// GetRecentSensorLogs fetches the last N readings for a specific sensor.
func (m *MemoryAgent) GetRecentSensorLogs(sensor string, limit int) ([]SensorLog, error) {
	rows, err := m.db.Query(`SELECT id, sensor, value, timestamp FROM sensor_logs 
		WHERE sensor = ? ORDER BY timestamp DESC LIMIT ?;`, sensor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []SensorLog
	for rows.Next() {
		var sl SensorLog
		var tsStr string
		if err := rows.Scan(&sl.ID, &sl.Sensor, &sl.Value, &tsStr); err != nil {
			return nil, err
		}
		// Parse timestamp from SQLite format
		t, err := time.Parse("2006-01-02 15:04:05", tsStr)
		if err != nil {
			// Fallback to parsed format with T or other common SQLite formats
			t, err = time.Parse(time.RFC3339, tsStr)
			if err != nil {
				t = time.Now() // default
			}
		}
		sl.Timestamp = t
		logs = append(logs, sl)
	}
	return logs, nil
}

// AddGateQuestion saves a instrumentation study query card.
func (m *MemoryAgent) AddGateQuestion(topic, question, answer, difficulty string) error {
	_, err := m.db.Exec(`INSERT INTO gate_notes (topic, question, answer, difficulty) 
		VALUES (?, ?, ?, ?);`, topic, question, answer, difficulty)
	return err
}

// GetRandomGateQuestion picks a random card from a given topic for self-quizzing.
func (m *MemoryAgent) GetRandomGateQuestion(topic string) (*GateQuestion, error) {
	var gq GateQuestion
	var row *sql.Row
	if topic != "" {
		row = m.db.QueryRow(`SELECT id, topic, question, answer, difficulty FROM gate_notes 
			WHERE topic = ? ORDER BY RANDOM() LIMIT 1;`, topic)
	} else {
		row = m.db.QueryRow(`SELECT id, topic, question, answer, difficulty FROM gate_notes 
			ORDER BY RANDOM() LIMIT 1;`)
	}

	err := row.Scan(&gq.ID, &gq.Topic, &gq.Question, &gq.Answer, &gq.Difficulty)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &gq, err
}
