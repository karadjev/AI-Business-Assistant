package agent

import (
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/network"
	"net/http"
	"time"
)

func SafeClient(timeout time.Duration) *http.Client { return network.SafeClient(timeout) }
