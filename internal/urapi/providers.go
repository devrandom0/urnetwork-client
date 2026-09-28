package urapi

import (
	"context"

	"github.com/urnetwork/connect"

	"github.com/devrandom0/urnetwork-client/internal/config"
)

// ProviderSpecs is an opaque provider selection built from location options.
type ProviderSpecs struct {
	specs []*connect.ProviderSpec
}

// SelectProviders resolves loc into provider specs, falling back to best-available.
func SelectProviders(ctx context.Context, apiURL, jwt string, loc config.LocationConfig) ProviderSpecs {
	_, specs := buildProviderSpecs(ctx, apiURL, jwt, loc)
	return ProviderSpecs{specs: specs}
}

func (p ProviderSpecs) BestAvailableOnly() bool {
	return len(p.specs) == 1 && p.specs[0].BestAvailable
}

// Provider is one entry of a find-providers response.
type Provider struct {
	ClientID                string
	Tier                    int
	EstimatedBytesPerSecond int64
	IntermediaryIDs         []string
}

func FindProviders(ctx context.Context, apiURL, jwt string, specs ProviderSpecs, count int, rankMode string) ([]Provider, error) {
	api := newByAPI(ctx, apiURL, jwt)
	res, err := api.FindProviders2Sync(&connect.FindProviders2Args{Specs: specs.specs, Count: count, RankMode: rankMode})
	if err != nil {
		return nil, err
	}
	out := make([]Provider, 0, len(res.Providers))
	for _, p := range res.Providers {
		out = append(out, Provider{
			ClientID:                p.ClientId.String(),
			Tier:                    p.Tier,
			EstimatedBytesPerSecond: int64(p.EstimatedBytesPerSecond),
			IntermediaryIDs:         idsToStrings(p.IntermediaryIds),
		})
	}
	return out, nil
}

// LocationsFallback filters /network/provider-locations client-side for q. hasSpecs reports
// whether any matching group or location id parsed.
func LocationsFallback(ctx context.Context, apiURL, jwt, q string) (hasSpecs bool, res *LocationsResult) {
	specs, res := filterLocationsFallback(ctx, apiURL, jwt, q)
	return len(specs) > 0, res
}

// idsToStrings converts a slice of connect.Id to a slice of their string representations.
func idsToStrings(ids []connect.Id) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}
