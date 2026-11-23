package agentruntime

import (
	"context"
	"encoding/json"
	"time"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/specialists"
)

func (a *attempt) readGrowth(ctx context.Context, s af.Scope) (specialists.ReadResult, error) {
	var raw []byte
	if err := a.service.Pool.QueryRow(ctx, "SELECT platform.agent_growth_projection($1,$2)", a.job.ID, a.lease).Scan(&raw); err != nil {
		return specialists.ReadResult{}, err
	}
	var data map[string]int64
	if err := json.Unmarshal(raw, &data); err != nil || len(data) != 3 {
		return specialists.ReadResult{}, af.ErrDenied
	}
	names := []string{"units", "variants", "categories"}
	unit := "count"
	if a.job.Action == "review_inventory" {
		names = []string{"in_stock", "out_of_stock", "unknown"}
		unit = "variants"
	}
	metrics := make([]specialists.Metric, 0, len(names))
	for _, name := range names {
		v, ok := data[name]
		if !ok || v < 0 {
			return specialists.ReadResult{}, af.ErrDenied
		}
		metrics = append(metrics, specialists.Metric{Name: name, Value: v, Unit: unit})
	}
	now := time.Now()
	return specialists.ReadResult{Scope: s, Revision: a.job.ExecutionID, ObservedAt: now, ValidUntil: now.Add(time.Minute), Metrics: metrics}, nil
}

type coordinatorChild struct {
	Agent  string          `json:"agent_id"`
	Status string          `json:"status"`
	Result json.RawMessage `json:"result"`
}

type coordinatorReport struct {
	Plan, Children  json.RawMessage
	State           string   `json:"state"`
	Recommendations []string `json:"recommendations,omitempty"`
}

func coordinateResult(action string, plan, raw json.RawMessage) (coordinatorReport, error) {
	var children []coordinatorChild
	if err := json.Unmarshal(raw, &children); err != nil {
		return coordinatorReport{}, err
	}
	if len(children) == 0 {
		return coordinatorReport{}, af.ErrDenied
	}
	completed := 0
	for _, child := range children {
		if child.Status == "completed" && len(child.Result) != 0 && string(child.Result) != "null" {
			completed++
		}
	}
	report := coordinatorReport{Plan: plan, Children: raw, State: "completed"}
	switch {
	case completed == 0:
		report.State = "failed"
	case completed != len(children):
		report.State = "partial_failure"
	}
	if action == "coordinate_growth" {
		report.Recommendations = growthRecommendations(children)
	}
	return report, nil
}

func growthRecommendations(children []coordinatorChild) []string {
	// Decode only completed metric children. Marketing has its own typed result.
	var out []string
	var currentCount, previousCount *int64
	var units *int64
	incomplete := false
	for _, child := range children {
		if child.Status != "completed" {
			incomplete = true
			continue
		}
		if child.Agent != "analytics" && child.Agent != "sales" && child.Agent != "product_inventory" {
			continue
		}
		var results []specialists.ReadResult
		if json.Unmarshal(child.Result, &results) != nil || len(results) != 1 {
			incomplete = true
			continue
		}
		for _, m := range results[0].Metrics {
			switch child.Agent {
			case "analytics":
				switch m.Name {
				case "completed_orders_created_last_30d":
					currentCount = &m.Value
				case "completed_orders_created_previous_30d":
					previousCount = &m.Value
				}
				continue
			case "sales":
				if m.Name == "units" {
					units = &m.Value
				}
				continue
			}
			switch {
			case m.Name == "unknown" && m.Value > 0:
				out = append(out, "Сверьте варианты с неподтверждённым остатком перед продвижением.")
			case m.Name == "out_of_stock" && m.Value > 0:
				out = append(out, "Проверьте возможность пополнения вариантов с нулевым остатком.")
			case m.Name == "in_stock" && m.Value > 0:
				out = append(out, "При подготовке предложений используйте только позиции с подтверждённым наличием.")
			}
		}
	}
	if currentCount != nil && previousCount != nil && *currentCount < *previousCount {
		out = append(out, "Завершённых заказов за последние 30 дней стало меньше. Проверьте причины до запуска предложений.")
	}
	if currentCount != nil && units != nil && *currentCount > 0 && *units == 0 {
		out = append(out, "В завершённых заказах есть расхождение между числом заказов и строками товара. Сверьте исходные данные.")
	}
	if incomplete {
		out = append(out, "Часть подзадач не завершена. Учитывайте только результаты со статусом «Завершена».")
	}
	return out
}
