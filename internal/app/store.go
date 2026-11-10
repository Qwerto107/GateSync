package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	aead cipher.AEAD
}

func randomID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func openStore(dir, keyEnv string) (*Store, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	keyPath := filepath.Join(dir, "master.key")
	var key []byte
	var err error
	if keyEnv != "" {
		key, err = base64.StdEncoding.DecodeString(keyEnv)
	} else {
		key, err = os.ReadFile(keyPath)
		if os.IsNotExist(err) {
			// 有数据库却缺少主密钥时必须停止，避免生成新密钥后掩盖凭证无法解密的问题。
			if _, e := os.Stat(filepath.Join(dir, "gatesync.db")); e == nil {
				return nil, fmt.Errorf("数据库已存在但 master.key 丢失，请恢复原主密钥")
			}
			key = make([]byte, 32)
			if _, err = rand.Read(key); err == nil {
				err = os.WriteFile(keyPath, key, 0600)
			}
		}
	}
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("主密钥必须是 32 字节；GATESYNC_MASTER_KEY 使用 Base64 编码")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "gatesync.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS accounts (id TEXT PRIMARY KEY, data TEXT NOT NULL, credentials BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS targets (id TEXT PRIMARY KEY, account_id TEXT NOT NULL REFERENCES accounts(id), data TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS logs (id INTEGER PRIMARY KEY AUTOINCREMENT, time TEXT NOT NULL, kind TEXT NOT NULL, trigger TEXT NOT NULL, object TEXT NOT NULL, detail TEXT NOT NULL, success INTEGER NOT NULL, changes TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS logs_time ON logs(time);
 CREATE TABLE IF NOT EXISTS sessions (token TEXT PRIMARY KEY, identity TEXT NOT NULL, expires INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS notification_channels (id TEXT PRIMARY KEY, data BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS notification_jobs (id TEXT PRIMARY KEY, channel_id TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE, event TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, next_attempt INTEGER NOT NULL);
 CREATE INDEX IF NOT EXISTS notification_jobs_due ON notification_jobs(next_attempt);
 DELETE FROM sessions; PRAGMA user_version=1;`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db, aead}, nil
}
func (s *Store) getMeta(key string, out any) error {
	var data string
	if e := s.db.QueryRow("SELECT value FROM meta WHERE key=?", key).Scan(&data); e != nil {
		return e
	}
	return json.Unmarshal([]byte(data), out)
}
func (s *Store) setMeta(key string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = s.db.Exec("INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, string(b))
	return e
}
func (s *Store) encrypt(id string, c Credentials) ([]byte, error) {
	b, e := json.Marshal(c)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return s.aead.Seal(nonce, nonce, b, []byte(id)), nil
}
func (s *Store) credentials(id string) (Credentials, error) {
	var c Credentials
	var blob []byte
	if e := s.db.QueryRow("SELECT credentials FROM accounts WHERE id=?", id).Scan(&blob); e != nil {
		return c, e
	}
	n := s.aead.NonceSize()
	if len(blob) < n {
		return c, fmt.Errorf("凭证数据损坏")
	}
	b, e := s.aead.Open(nil, blob[:n], blob[n:], []byte(id))
	if e != nil {
		return c, fmt.Errorf("凭证解密失败，请检查主密钥")
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
func (s *Store) accounts() ([]Account, error) {
	rows, e := s.db.Query("SELECT data FROM accounts ORDER BY rowid")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []Account{}
	for rows.Next() {
		var raw string
		var a Account
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &a); e != nil {
			return nil, e
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
func (s *Store) account(id string) (Account, error) {
	var raw string
	var a Account
	e := s.db.QueryRow("SELECT data FROM accounts WHERE id=?", id).Scan(&raw)
	if e == nil {
		e = json.Unmarshal([]byte(raw), &a)
	}
	return a, e
}
func (s *Store) saveAccount(a Account, c *Credentials) error {
	b, e := json.Marshal(a)
	if e != nil {
		return e
	}
	if c == nil {
		_, e = s.db.Exec("UPDATE accounts SET data=? WHERE id=?", string(b), a.ID)
		return e
	}
	encrypted, e := s.encrypt(a.ID, *c)
	if e != nil {
		return e
	}
	_, e = s.db.Exec("INSERT INTO accounts(id,data,credentials) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data,credentials=excluded.credentials", a.ID, string(b), encrypted)
	return e
}
func (s *Store) targets() ([]Target, error) {
	rows, e := s.db.Query("SELECT data FROM targets ORDER BY rowid")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []Target{}
	for rows.Next() {
		var raw string
		var t Target
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &t); e != nil {
			return nil, e
		}
		items = append(items, t)
	}
	if e := rows.Err(); e != nil {
		return nil, e
	}
	// 未指定顺序的旧目标及新目标沿用插入顺序，排在已排序目标之后。
	var order []string
	if e := rows.Close(); e != nil {
		return nil, e
	}
	if e := s.getMeta("target-order", &order); e != nil && e != sql.ErrNoRows {
		return nil, e
	}
	positions := make(map[string]int, len(order))
	for i, id := range order {
		positions[id] = i
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, ok := positions[items[i].ID]
		if !ok {
			left = len(order)
		}
		right, ok := positions[items[j].ID]
		if !ok {
			right = len(order)
		}
		return left < right
	})
	return items, nil
}
func (s *Store) target(id string) (Target, error) {
	var raw string
	var t Target
	e := s.db.QueryRow("SELECT data FROM targets WHERE id=?", id).Scan(&raw)
	if e == nil {
		e = json.Unmarshal([]byte(raw), &t)
	}
	return t, e
}
func (s *Store) saveTarget(t Target) error {
	b, e := json.Marshal(t)
	if e != nil {
		return e
	}
	_, e = s.db.Exec("INSERT INTO targets(id,account_id,data) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data,account_id=excluded.account_id", t.ID, t.AccountID, string(b))
	return e
}
func (s *Store) addLog(l LogEntry) error {
	if l.Time == "" {
		l.Time = now()
	}
	if l.Changes == nil {
		l.Changes = []string{}
	}
	b, e := json.Marshal(l.Changes)
	if e != nil {
		return e
	}
	_, e = s.db.Exec("INSERT INTO logs(time,kind,trigger,object,detail,success,changes) VALUES(?,?,?,?,?,?,?)", l.Time, l.Kind, l.Trigger, l.Object, l.Detail, l.Success, string(b))
	return e
}
