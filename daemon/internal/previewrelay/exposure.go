package previewrelay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
)

const (
	httpScheme  = "http"
	httpsScheme = "https"
)

func (s *Service) gatewayAddress(ctx context.Context, port int, origin string) (*url.URL, func(), error) {
	if isLoopbackOrigin(origin) {
		return &url.URL{Scheme: httpScheme, Host: net.JoinHostPort(clientHostname(origin), strconv.Itoa(port))}, nil, nil
	}
	if s.expose == nil {
		return nil, nil, errors.New("remote preview exposure is unavailable")
	}
	base, cleanup, err := s.expose(ctx, port, originScheme(origin) == httpsScheme)
	if err != nil {
		closeExposure(cleanup)
		return nil, nil, fmt.Errorf("expose preview gateway: %w", err)
	}
	parsed, err := parsePublicBase(base)
	if err != nil {
		closeExposure(cleanup)
		return nil, nil, fmt.Errorf("invalid preview public base: %w", err)
	}
	if originScheme(origin) == httpsScheme && parsed.Scheme != httpsScheme {
		closeExposure(cleanup)
		return nil, nil, errors.New("secure Marshal connections require an HTTPS preview")
	}
	// Keep HTTP gateways on the initiating hostname/IP, so pairing by Tailscale IP
	// remains same-site when the host's preferred name is MagicDNS.
	if parsed.Scheme == httpScheme {
		parsed.Host = net.JoinHostPort(clientHostname(origin), parsed.Port())
	}
	return parsed, cleanup, nil
}

func closeExposure(cleanup func()) {
	if cleanup != nil {
		cleanup()
	}
}
