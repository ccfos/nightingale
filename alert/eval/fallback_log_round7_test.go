package eval

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/evallog"
	"github.com/stretchr/testify/require"
)

func TestLogEvalRecordFallback_IncludesOutcomeAndTruncatedQueryDetails(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	original := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = original
		reader.Close()
		writer.Close()
	})
	record := &evallog.EvalRecord{
		RuleId: 7, DatasourceId: 8, DurationMs: 12, Error: "evaluation failed",
		AnomalyTotal: 3, RecoverTotal: 1, Fired: 2, Pending: 1, Muted: 4, DropByPipeline: 5, Inhibited: 6,
		Queries: []evallog.QueryRecord{
			{Ref: "A", SeriesTotal: 3, DurationMs: 4, Query: strings.Repeat("q", 300), Error: "query failed"},
			{Ref: "B", SeriesTotal: 1, DurationMs: 2, Query: "up"},
		},
	}

	LogEvalRecordFallback(record)

	os.Stdout = original
	require.NoError(t, writer.Close())
	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	line := string(output)
	require.Contains(t, line, "alert_eval_7 datasource_8 evallog record dropped")
	require.Contains(t, line, "cost=12ms error=evaluation failed")
	require.Contains(t, line, "anomaly=3 recover=1 fired=2 pending=1 muted=4 dropped=5 inhibited=6")
	require.Contains(t, line, "A series=3 cost=4ms error=query failed query="+strings.Repeat("q", 256)+"...(truncated)")
	require.Contains(t, line, "; B series=1 cost=2ms query=up")
	require.NotContains(t, line, strings.Repeat("q", 257))
}
