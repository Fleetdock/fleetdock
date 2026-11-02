// Package tableviewapp manages saved data-browser table views.
package tableviewapp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	tableviewdom "github.com/Fleetdock/fleetdock/backend/internal/domain/tableview"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// MaxStateBytes caps a view's stored state.
const MaxStateBytes = 64 << 10

// Service validates and stores views.
type Service struct {
	repo tableviewdom.Repository
}

// NewService wires the service.
func NewService(repo tableviewdom.Repository) *Service {
	return &Service{repo: repo}
}

// Input is what a create or update sets. Nil fields stay as they are on update.
type Input struct {
	Table  *string          `json:"table"`
	Name   *string          `json:"name"`
	State  *json.RawMessage `json:"state"`
	Shared *bool            `json:"shared"`
}

func parseID(raw, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, apperr.Invalid(field, "must be a valid UUID")
	}
	return id, nil
}

func cleanName(n string) (string, error) {
	n = strings.TrimSpace(n)
	if n == "" || utf8.RuneCountInString(n) > 100 {
		return "", apperr.Invalid("name", "the name is 1 to 100 characters")
	}
	return n, nil
}

func cleanTable(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" || len(t) > 300 {
		return "", apperr.Invalid("table", "the table is required")
	}
	return t, nil
}

func cleanState(s json.RawMessage) (json.RawMessage, error) {
	s = bytes.TrimSpace(s)
	if len(s) > MaxStateBytes {
		return nil, apperr.Invalid("state", "the view is too large")
	}
	var obj map[string]any
	if len(s) == 0 || json.Unmarshal(s, &obj) != nil || obj == nil {
		return nil, apperr.Invalid("state", "the view must be a JSON object")
	}
	return s, nil
}

// List returns the caller's views of a database (of one table when given) and the shared ones.
func (s *Service) List(ctx context.Context, databaseID, table string, userID uuid.UUID) ([]tableviewdom.View, error) {
	did, err := parseID(databaseID, "id")
	if err != nil {
		return nil, err
	}
	items, err := s.repo.List(ctx, did, strings.TrimSpace(table), userID)
	if items == nil {
		items = []tableviewdom.View{}
	}
	return items, err
}

// Create saves a new view owned by userID.
func (s *Service) Create(ctx context.Context, databaseID string, userID uuid.UUID, in Input) (*tableviewdom.View, error) {
	did, err := parseID(databaseID, "id")
	if err != nil {
		return nil, err
	}
	if in.Table == nil || in.Name == nil || in.State == nil {
		return nil, apperr.Invalid("state", "table, name and state are required")
	}
	v := &tableviewdom.View{DatabaseID: did, CreatedBy: &userID, Shared: in.Shared != nil && *in.Shared}
	if v.Table, err = cleanTable(*in.Table); err != nil {
		return nil, err
	}
	if v.Name, err = cleanName(*in.Name); err != nil {
		return nil, err
	}
	if v.State, err = cleanState(*in.State); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

// get loads a view the caller may see: their own, or a shared one.
func (s *Service) get(ctx context.Context, databaseID, viewID string, userID uuid.UUID) (*tableviewdom.View, error) {
	did, err := parseID(databaseID, "id")
	if err != nil {
		return nil, err
	}
	vid, err := parseID(viewID, "viewId")
	if err != nil {
		return nil, err
	}
	v, err := s.repo.Get(ctx, did, vid)
	if err != nil {
		return nil, err
	}
	if !v.Shared && !owns(v, userID) {
		return nil, apperr.NotFound("view not found")
	}
	return v, nil
}

func owns(v *tableviewdom.View, userID uuid.UUID) bool {
	return v.CreatedBy != nil && *v.CreatedBy == userID
}

// Update changes the caller's own view.
func (s *Service) Update(ctx context.Context, databaseID, viewID string, userID uuid.UUID, in Input) (*tableviewdom.View, error) {
	v, err := s.get(ctx, databaseID, viewID, userID)
	if err != nil {
		return nil, err
	}
	if !owns(v, userID) {
		return nil, apperr.Forbidden("only the person who saved this view can change it")
	}
	if in.Name != nil {
		if v.Name, err = cleanName(*in.Name); err != nil {
			return nil, err
		}
	}
	if in.State != nil {
		if v.State, err = cleanState(*in.State); err != nil {
			return nil, err
		}
	}
	if in.Shared != nil {
		v.Shared = *in.Shared
	}
	if err := s.repo.Update(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

// Delete removes a view: the caller's own, or a shared one when they may
// change the database (canWrite).
func (s *Service) Delete(ctx context.Context, databaseID, viewID string, userID uuid.UUID, canWrite bool) error {
	v, err := s.get(ctx, databaseID, viewID, userID)
	if err != nil {
		return err
	}
	if !owns(v, userID) && (!v.Shared || !canWrite) {
		return apperr.Forbidden("only the person who saved this view, or someone who can change the database, can delete it")
	}
	return s.repo.Delete(ctx, v.DatabaseID, v.ID)
}
