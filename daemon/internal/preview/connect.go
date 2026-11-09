package preview

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/previewrelay"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// SetExposure configures private access to preview gateways from paired devices.
func (s *Service) SetExposure(expose previewrelay.Expose) { s.relay.SetExposure(expose) }

// Connect opens an already-running project server without starting another process.
func (s *Service) Connect(ctx context.Context, cardID string, req protocol.PreviewConnectRequest, authorized func(context.Context) bool) (protocol.PreviewConnection, error) {
	if _, err := s.projects.Card(ctx, cardID); err != nil {
		return protocol.PreviewConnection{}, err
	}
	connection, err := s.relay.Open(ctx, cardID, req.URL, req.Origin, authorized)
	if err != nil {
		return protocol.PreviewConnection{}, protocol.Refused(err.Error())
	}
	return protocol.PreviewConnection{URL: connection.URL, SourceURL: connection.SourceURL, ExpiresAt: protocol.NewTimestamp(connection.ExpiresAt)}, nil
}
