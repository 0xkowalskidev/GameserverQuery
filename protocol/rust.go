package protocol

import (
	"context"
)

// RustProtocol implements Rust server queries using Source A2S protocol
// Rust uses Source protocol but has different default port (28015)
type RustProtocol struct {
	source *SourceProtocol
}

func init() {
	registry.Register(&RustProtocol{source: &SourceProtocol{}})
}

func (r *RustProtocol) Name() string {
	return "rust"
}

func (r *RustProtocol) DefaultPort() int {
	return 28015 // Rust game port, not 27015
}

func (r *RustProtocol) DefaultQueryPort() int {
	return 28015 // Rust query port is the same as game port
}

func (r *RustProtocol) Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error) {
	// Use Source protocol for the actual query
	info, err := r.source.Query(ctx, addr, opts)
	if err != nil {
		return info, err
	}
	
	// Game field will be determined by central game detector
	// No need to override here
	
	return info, nil
}