package integrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// readConnection reads one connection's saved settings and its secrets from the keychain. A missing
// row or a missing keychain entry is not an error: both mean "nothing is stored", which is what a
// connection nobody has set up looks like. name is how the connection reads in an error message.
func readConnection[C, S any](ctx context.Context, s *Service, id, name string) (C, S, error) {
	var config C
	var secrets S
	var found bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetIntegration(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("read the %s connection's settings: %w", name, err)
		}
		found = true
		if row.ConfigJSON == "" {
			return nil
		}
		return json.Unmarshal([]byte(row.ConfigJSON), &config)
	})
	if err != nil || !found {
		var none C
		return none, secrets, err
	}
	raw, err := s.keys.Get(id)
	if err != nil && !errors.Is(err, security.ErrNoKey) {
		var none C
		return none, secrets, fmt.Errorf("read the %s connection's secrets from the keychain: %w", name, err)
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &secrets); err != nil {
			var none C
			var noSecrets S
			return none, noSecrets, fmt.Errorf("read the %s connection's secrets: %w", name, err)
		}
	}
	return config, secrets, nil
}
