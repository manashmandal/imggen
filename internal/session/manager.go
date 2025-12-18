package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNoSession       = errors.New("no active session")
	ErrNoIteration     = errors.New("no current iteration")
	ErrNothingToUndo   = errors.New("nothing to undo")
	ErrAtFirstImage    = errors.New("already at first image")
	ErrSessionNotFound = errors.New("session not found")
)

type Manager struct {
	store        *Store
	current      *Session
	currentIter  *Iteration
	defaultModel string
}

func NewManager(store *Store, defaultModel string) *Manager {
	if defaultModel == "" {
		defaultModel = "gpt-image-1"
	}
	return &Manager{
		store:        store,
		defaultModel: defaultModel,
	}
}

func (m *Manager) Current() *Session {
	return m.current
}

func (m *Manager) CurrentIteration() *Iteration {
	return m.currentIter
}

func (m *Manager) HasSession() bool {
	return m.current != nil
}

func (m *Manager) HasIteration() bool {
	return m.currentIter != nil
}

func (m *Manager) StartNew(ctx context.Context, name string) (*Session, error) {
	sess := &Session{
		ID:        uuid.New().String(),
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Model:     m.defaultModel,
	}

	if err := m.store.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	if _, err := EnsureImageDir(sess.ID); err != nil {
		return nil, fmt.Errorf("failed to create image directory: %w", err)
	}

	m.current = sess
	m.currentIter = nil
	return sess, nil
}

func (m *Manager) Load(ctx context.Context, id string) error {
	sess, err := m.store.GetSession(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSessionNotFound, err)
	}

	m.current = sess

	if sess.CurrentIterationID != "" {
		iter, err := m.store.GetIteration(ctx, sess.CurrentIterationID)
		if err != nil {
			return fmt.Errorf("failed to load current iteration: %w", err)
		}
		m.currentIter = iter
	} else {
		m.currentIter = nil
	}

	return nil
}

func (m *Manager) EnsureSession(ctx context.Context) error {
	if m.current == nil {
		_, err := m.StartNew(ctx, "")
		return err
	}
	return nil
}

func (m *Manager) AddIteration(ctx context.Context, iter *Iteration) error {
	if err := m.EnsureSession(ctx); err != nil {
		return err
	}

	iter.ID = uuid.New().String()
	iter.SessionID = m.current.ID
	iter.Timestamp = time.Now()

	if m.currentIter != nil {
		iter.ParentID = m.currentIter.ID
	}

	if err := m.store.CreateIteration(ctx, iter); err != nil {
		return fmt.Errorf("failed to create iteration: %w", err)
	}

	m.current.CurrentIterationID = iter.ID
	m.current.UpdatedAt = time.Now()
	if err := m.store.UpdateSession(ctx, m.current); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	m.currentIter = iter
	return nil
}

func (m *Manager) Undo(ctx context.Context) (*Iteration, error) {
	if m.currentIter == nil {
		return nil, ErrNoIteration
	}

	if m.currentIter.ParentID == "" {
		return nil, ErrAtFirstImage
	}

	parent, err := m.store.GetIteration(ctx, m.currentIter.ParentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent iteration: %w", err)
	}

	m.current.CurrentIterationID = parent.ID
	m.current.UpdatedAt = time.Now()
	if err := m.store.UpdateSession(ctx, m.current); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	m.currentIter = parent
	return parent, nil
}

func (m *Manager) History(ctx context.Context) ([]*Iteration, error) {
	if m.current == nil {
		return nil, nil
	}
	return m.store.ListIterations(ctx, m.current.ID)
}

func (m *Manager) ListSessions(ctx context.Context) ([]*Session, error) {
	return m.store.ListSessions(ctx)
}

func (m *Manager) DeleteSession(ctx context.Context, id string) error {
	if m.current != nil && m.current.ID == id {
		m.current = nil
		m.currentIter = nil
	}
	return m.store.DeleteSession(ctx, id)
}

func (m *Manager) RenameSession(ctx context.Context, name string) error {
	if m.current == nil {
		return ErrNoSession
	}
	m.current.Name = name
	m.current.UpdatedAt = time.Now()
	return m.store.UpdateSession(ctx, m.current)
}

func (m *Manager) SetModel(model string) {
	m.defaultModel = model
	if m.current != nil {
		m.current.Model = model
	}
}

func (m *Manager) GetModel() string {
	if m.current != nil {
		return m.current.Model
	}
	return m.defaultModel
}

func (m *Manager) ImagePath() string {
	if m.current == nil {
		return ""
	}
	dir, _ := ImageDir(m.current.ID)
	return filepath.Join(dir, uuid.New().String())
}

func (m *Manager) CurrentImagePath() string {
	if m.currentIter == nil {
		return ""
	}
	return m.currentIter.ImagePath
}

func (m *Manager) IterationCount(ctx context.Context) (int, error) {
	if m.current == nil {
		return 0, nil
	}
	return m.store.CountIterations(ctx, m.current.ID)
}

func (m *Manager) LogCost(ctx context.Context, entry *CostEntry) error {
	return m.store.LogCost(ctx, entry)
}

func (m *Manager) GetCostByDateRange(ctx context.Context, start, end time.Time) (*CostSummary, error) {
	return m.store.GetCostByDateRange(ctx, start, end)
}

func (m *Manager) GetCostByProvider(ctx context.Context) ([]ProviderCostSummary, error) {
	return m.store.GetCostByProvider(ctx)
}

func (m *Manager) GetTotalCost(ctx context.Context) (*CostSummary, error) {
	return m.store.GetTotalCost(ctx)
}

func (m *Manager) GetSessionCost(ctx context.Context) (*CostSummary, error) {
	if m.current == nil {
		return &CostSummary{}, nil
	}
	return m.store.GetSessionCost(ctx, m.current.ID)
}
