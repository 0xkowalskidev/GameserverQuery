package protocol

import (
	"context"
)

// ValheimProtocol implements Valheim server queries using Source A2S protocol
// Valheim uses Source protocol but has different default port (2456)
type ValheimProtocol struct {
	source *SourceProtocol
}

func init() {
	registry.Register(&ValheimProtocol{source: &SourceProtocol{}})
}

func (v *ValheimProtocol) Name() string {
	return "valheim"
}

func (v *ValheimProtocol) DefaultPort() int {
	return 2456 // Valheim game port
}

func (v *ValheimProtocol) DefaultQueryPort() int {
	return 2457 // Valheim query port (game port + 1)
}

func (v *ValheimProtocol) Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error) {
	// Use Source protocol for the actual query
	info, err := v.source.Query(ctx, addr, opts)
	if err != nil {
		return info, err
	}

	return info, nil
}

