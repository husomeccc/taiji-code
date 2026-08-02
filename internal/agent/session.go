package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"taiji-code/types"
	"time"
)

// Session represents a saved conversation session
type Session struct {
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	WorkDir   string          `json:"work_dir"`
	Model     string          `json:"model"`
	Messages  []types.Message `json:"messages"`
	Summary   string          `json:"summary,omitempty"`
	Usage     types.Usage     `json:"usage"`
}

// SessionManager handles session persistence
type SessionManager struct {
	SessionDir string
}

// NewSessionManager creates a session manager
func NewSessionManager(workDir string) *SessionManager {
	dir := filepath.Join(workDir, ".taiji", "sessions")
	os.MkdirAll(dir, 0755)
	return &SessionManager{SessionDir: dir}
}

// Save saves the current session
func (sm *SessionManager) Save(session *Session) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	session.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化会话失败: %w", err)
	}

	filename := session.ID + ".json"
	path := filepath.Join(sm.SessionDir, filename)

	return os.WriteFile(path, data, 0644)
}

// Load loads a session by ID
func (sm *SessionManager) Load(id string) (*Session, error) {
	path := filepath.Join(sm.SessionDir, id+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取会话失败: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("解析会话失败: %w", err)
	}

	return &session, nil
}

// List returns all saved sessions (most recent first)
func (sm *SessionManager) List() ([]Session, error) {
	entries, err := os.ReadDir(sm.SessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []Session
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sm.SessionDir, entry.Name()))
		if err != nil {
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	// Sort by updated time (most recent first)
	for i := 0; i < len(sessions)-1; i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].UpdatedAt.After(sessions[i].UpdatedAt) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	return sessions, nil
}

// Delete removes a session
func (sm *SessionManager) Delete(id string) error {
	path := filepath.Join(sm.SessionDir, id+".json")
	return os.Remove(path)
}

// GenerateID creates a session ID from timestamp
func GenerateID() string {
	return fmt.Sprintf("session_%s", time.Now().Format("20060102_150405"))
}

// FormatSessionList formats sessions for display
func FormatSessionList(sessions []Session) string {
	if len(sessions) == 0 {
		return "没有保存的会话"
	}

	var result string
	for i, s := range sessions {
		msgCount := len(s.Messages)
		summary := s.Summary
		if summary == "" && msgCount > 0 {
			// Use first user message as summary
			for _, msg := range s.Messages {
				if msg.Role == types.RoleUser {
					summary = msg.Content
					if len(summary) > 50 {
						summary = summary[:50] + "..."
					}
					break
				}
			}
		}
		if summary == "" {
			summary = "(空会话)"
		}

		result += fmt.Sprintf("  %d. [%s] %s (%d条消息)\n",
			i+1,
			s.UpdatedAt.Format("01-02 15:04"),
			summary,
			msgCount,
		)
	}

	return result
}
