package costcontrol

import (
	"context"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"testing"
)

func TestFixedPointEstimate(t *testing.T) {
	for _, c := range []struct {
		in, out      int
		ri, ro, want int64
	}{{1, 0, 1, 1, 1}, {1000, 500, 1000000, 2000000, 2000}, {0, 0, 1, 1, 0}, {96000, 96000, 1000000000000, 1000000000000, 192000000000}} {
		n, e := Estimate(c.in, c.out, c.ri, c.ro)
		if e != nil || n != c.want {
			t.Fatal("estimate", n, e, c)
		}
	}
	for _, c := range []struct {
		in, out int
		ri, ro  int64
	}{{-1, 1, 1, 1}, {1, -1, 1, 1}, {1, 1, 0, 1}, {1, 1, 1000000000001, 1}, {96001, 1, 1, 1}} {
		if _, e := Estimate(c.in, c.out, c.ri, c.ro); e == nil {
			t.Fatal("invalid input accepted")
		}
	}
}
func TestZeroBudgetRefusesBeforeDatabase(t *testing.T) {
	cfg := config.Agent{LiveEnabled: true, Live: config.Live{ExternalAuthorized: true, BaseURL: "https://provider.example/v1", AllowedBases: []string{"https://provider.example/v1"}, InputRate: 1000000, OutputRate: 1000000, PlatformMicroRUB: 0}}
	service := Service{Config: cfg}
	if _, e := service.Reserve(context.Background(), catalog.Tenant{}, "", 1, 1, 1); e == nil {
		t.Fatal("zero budget admitted live request")
	}
}
