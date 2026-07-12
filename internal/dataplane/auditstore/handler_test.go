package auditstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/cyacco/Sheeld/internal/dataplane/db/generated"
)

// fakeHandlerQueries records the ListAuditLogs params it was called with and
// returns a fixed set of rows. Only ListAuditLogs is exercised here.
type fakeHandlerQueries struct {
	lastParams generated.ListAuditLogsParams
	rows       []generated.AuditLog
	called     bool
}

func (f *fakeHandlerQueries) ListAuditLogs(_ context.Context, arg generated.ListAuditLogsParams) ([]generated.AuditLog, error) {
	f.called = true
	f.lastParams = arg
	return f.rows, nil
}
func (f *fakeHandlerQueries) AuditSummary(context.Context, generated.AuditSummaryParams) (generated.AuditSummaryRow, error) {
	return generated.AuditSummaryRow{}, nil
}
func (f *fakeHandlerQueries) AuditDailySeries(context.Context, generated.AuditDailySeriesParams) ([]generated.AuditDailySeriesRow, error) {
	return nil, nil
}
func (f *fakeHandlerQueries) AuditByModel(context.Context, generated.AuditByModelParams) ([]generated.AuditByModelRow, error) {
	return nil, nil
}
func (f *fakeHandlerQueries) AuditBySource(context.Context, generated.AuditBySourceParams) ([]generated.AuditBySourceRow, error) {
	return nil, nil
}

func doList(t *testing.T, fake *fakeHandlerQueries, query string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(fake)
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/audit-logs?"+query, nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)
	return rec
}

func TestList_RejectsBadParams(t *testing.T) {
	org := uuid.New().String()
	cases := []struct {
		name  string
		query string
	}{
		{"missing org", "limit=10"},
		{"bad status", "org_id=" + org + "&status=maybe"},
		{"unpaired cursor", "org_id=" + org + "&before=2026-07-11T00:00:00Z"},
		{"bad from", "org_id=" + org + "&from=yesterday"},
		{"bad source", "org_id=" + org + "&source_id=not-a-uuid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeHandlerQueries{}
			rec := doList(t, fake, c.query)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rec.Code)
			}
			if fake.called {
				t.Error("query should not have run on a validation failure")
			}
		})
	}
}

func TestList_ThreadsFiltersAndCursor(t *testing.T) {
	org := uuid.New()
	src := uuid.New()
	before := uuid.New()
	fake := &fakeHandlerQueries{}
	rec := doList(t, fake, "org_id="+org.String()+
		"&source_id="+src.String()+
		"&status=fail"+
		"&from=2026-07-01T00:00:00Z"+
		"&to=2026-07-11T00:00:00Z"+
		"&before=2026-07-10T12:00:00Z"+
		"&before_id="+before.String()+
		"&limit=25")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	p := fake.lastParams
	if p.OrganizationID != org {
		t.Errorf("org not threaded: %v", p.OrganizationID)
	}
	if !p.SourceID.Valid || uuid.UUID(p.SourceID.Bytes) != src {
		t.Errorf("source filter not threaded: %+v", p.SourceID)
	}
	if !p.Status.Valid || p.Status.String != "fail" {
		t.Errorf("status filter not threaded: %+v", p.Status)
	}
	if !p.FromTime.Valid || !p.ToTime.Valid {
		t.Errorf("time bounds not threaded: from=%v to=%v", p.FromTime, p.ToTime)
	}
	if !p.BeforeTime.Valid || !p.BeforeID.Valid || uuid.UUID(p.BeforeID.Bytes) != before {
		t.Errorf("cursor not threaded: t=%v id=%v", p.BeforeTime, p.BeforeID)
	}
	if p.LimitCount != 25 {
		t.Errorf("limit not threaded: %d", p.LimitCount)
	}
}

func TestList_DefaultsLimit(t *testing.T) {
	fake := &fakeHandlerQueries{}
	rec := doList(t, fake, "org_id="+uuid.New().String())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fake.lastParams.LimitCount != 50 {
		t.Errorf("expected default limit 50, got %d", fake.lastParams.LimitCount)
	}
	// No filters set → all nullable params absent.
	if fake.lastParams.SourceID.Valid || fake.lastParams.Status.Valid || fake.lastParams.BeforeTime.Valid {
		t.Errorf("expected no filters set: %+v", fake.lastParams)
	}
}
