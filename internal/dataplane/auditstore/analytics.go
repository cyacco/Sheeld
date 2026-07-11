package auditstore

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/dataplane/db/generated"
	"github.com/cyacco/Sheeld/internal/shared/response"
)

// AnalyticsResponse is the aggregated usage payload for the dashboard.
type AnalyticsResponse struct {
	Days     int              `json:"days"`
	Summary  AnalyticsSummary `json:"summary"`
	Daily    []DailyPoint     `json:"daily"`
	ByModel  []ModelUsage     `json:"by_model"`
	BySource []SourceUsage    `json:"by_source"`
}

type AnalyticsSummary struct {
	TotalRequests    int64 `json:"total_requests"`
	Passed           int64 `json:"passed"`
	Rejected         int64 `json:"rejected"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type DailyPoint struct {
	Day         string `json:"day"` // YYYY-MM-DD
	Requests    int64  `json:"requests"`
	TotalTokens int64  `json:"total_tokens"`
}

type ModelUsage struct {
	Model       string `json:"model"`
	Requests    int64  `json:"requests"`
	TotalTokens int64  `json:"total_tokens"`
}

type SourceUsage struct {
	SourceID    uuid.UUID `json:"source_id"`
	Requests    int64     `json:"requests"`
	Rejected    int64     `json:"rejected"`
	TotalTokens int64     `json:"total_tokens"`
}

// Analytics handles GET /v1/internal/analytics?org_id=&days=.
func (h *Handler) Analytics(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(r.URL.Query().Get("org_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid or missing org_id")
		return
	}

	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	since := time.Now().AddDate(0, 0, -days)
	ctx := r.Context()

	summary, err := h.queries.AuditSummary(ctx, generated.AuditSummaryParams{
		OrganizationID: orgID,
		CreatedAt:      since,
	})
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to load summary")
		return
	}

	dailyRows, err := h.queries.AuditDailySeries(ctx, generated.AuditDailySeriesParams{
		OrganizationID: orgID,
		CreatedAt:      since,
	})
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to load daily series")
		return
	}

	modelRows, err := h.queries.AuditByModel(ctx, generated.AuditByModelParams{
		OrganizationID: orgID,
		CreatedAt:      since,
	})
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to load model breakdown")
		return
	}

	sourceRows, err := h.queries.AuditBySource(ctx, generated.AuditBySourceParams{
		OrganizationID: orgID,
		CreatedAt:      since,
	})
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to load source breakdown")
		return
	}

	resp := AnalyticsResponse{
		Days: days,
		Summary: AnalyticsSummary{
			TotalRequests:    summary.TotalRequests,
			Passed:           summary.Passed,
			Rejected:         summary.TotalRequests - summary.Passed,
			PromptTokens:     summary.PromptTokens,
			CompletionTokens: summary.CompletionTokens,
			TotalTokens:      summary.TotalTokens,
		},
		Daily:    make([]DailyPoint, 0, len(dailyRows)),
		ByModel:  make([]ModelUsage, 0, len(modelRows)),
		BySource: make([]SourceUsage, 0, len(sourceRows)),
	}
	for _, d := range dailyRows {
		resp.Daily = append(resp.Daily, DailyPoint{
			Day:         d.Day.Format("2006-01-02"),
			Requests:    d.Requests,
			TotalTokens: d.TotalTokens,
		})
	}
	for _, m := range modelRows {
		resp.ByModel = append(resp.ByModel, ModelUsage{
			Model:       m.Model.String,
			Requests:    m.Requests,
			TotalTokens: m.TotalTokens,
		})
	}
	for _, s := range sourceRows {
		resp.BySource = append(resp.BySource, SourceUsage{
			SourceID:    s.SourceID,
			Requests:    s.Requests,
			Rejected:    s.Rejected,
			TotalTokens: s.TotalTokens,
		})
	}

	response.JSON(w, http.StatusOK, resp)
}
