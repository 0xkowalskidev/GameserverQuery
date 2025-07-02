package protocol

import (
	"context"
)

// ArkProtocol implements ARK: Survival Evolved server queries using Source A2S protocol
// ARK uses port 7777 for gameplay but 27015 for Steam queries
type ArkProtocol struct {
	source *SourceProtocol
}

func init() {
	registry.Register(&ArkProtocol{source: &SourceProtocol{}})
}

func (a *ArkProtocol) Name() string {
	return "ark-survival-evolved"
}

func (a *ArkProtocol) DefaultPort() int {
	return 7777 // ARK game port where players connect
}

func (a *ArkProtocol) DefaultQueryPort() int {
	return 27015 // ARK query port for Steam queries
}

func (a *ArkProtocol) Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error) {
	// Use Source protocol for the actual query
	info, err := a.source.Query(ctx, addr, opts)
	if err != nil {
		return info, err
	}
	
	// Game field will be determined by central game detector
	// No need to override here
	
	return info, nil
}