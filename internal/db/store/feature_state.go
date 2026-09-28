package store

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

type FeatureStateStore struct {
	db  *DB
	log *zap.Logger
}

func NewFeatureStateStore(db *DB, log *zap.Logger) *FeatureStateStore {
	return &FeatureStateStore{db: db, log: log}
}

func (s *FeatureStateStore) List(ctx context.Context, orgID, kind string) ([]map[string]any, error) {
	rows, err := s.db.Pool.Query(ctx, `
		SELECT payload
		FROM feature_state
		WHERE org_id=$1 AND kind=$2
		ORDER BY updated_at DESC`, orgID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		item, err := scanFeaturePayload(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *FeatureStateStore) Get(ctx context.Context, orgID, kind, id string) (map[string]any, error) {
	item, err := scanFeaturePayload(func(dest ...any) error {
		return s.db.Pool.QueryRow(ctx, `
			SELECT payload
			FROM feature_state
			WHERE org_id=$1 AND kind=$2 AND id=$3`, orgID, kind, id).Scan(dest...)
	})
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *FeatureStateStore) Upsert(ctx context.Context, orgID, kind, id string, payload map[string]any) (map[string]any, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["id"] = id
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal feature payload: %w", err)
	}

	item, err := scanFeaturePayload(func(dest ...any) error {
		return s.db.Pool.QueryRow(ctx, `
			INSERT INTO feature_state (org_id, kind, id, payload)
			VALUES ($1, $2, $3, $4::jsonb)
			ON CONFLICT (org_id, kind, id)
			DO UPDATE SET payload=EXCLUDED.payload, updated_at=NOW()
			RETURNING payload`, orgID, kind, id, string(body)).Scan(dest...)
	})
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *FeatureStateStore) Delete(ctx context.Context, orgID, kind, id string) error {
	tag, err := s.db.Pool.Exec(ctx, `
		DELETE FROM feature_state
		WHERE org_id=$1 AND kind=$2 AND id=$3`, orgID, kind, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("feature state not found")
	}
	return nil
}

func scanFeaturePayload(scan func(dest ...any) error) (map[string]any, error) {
	var raw []byte
	if err := scan(&raw); err != nil {
		return nil, err
	}
	item := map[string]any{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	return item, nil
}
