package loggrep

import (
	"bufio"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const MaxLogLines = 5000

var hashPattern = regexp.MustCompile(`^[a-f0-9]{32,64}$`)
var idPattern = regexp.MustCompile(`^[1-9]\d*$`)
var traceIdPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// IsValidHash checks whether s looks like a valid MD5/SHA hex hash.
func IsValidHash(s string) bool {
	return hashPattern.MatchString(s)
}

// IsValidRuleID checks whether s looks like a valid positive integer rule ID.
func IsValidRuleID(s string) bool {
	return idPattern.MatchString(s)
}

// IsValidTraceID checks whether s looks like a valid trace ID (alphanumeric, hyphens, underscores).
func IsValidTraceID(s string) bool {
	return traceIdPattern.MatchString(s)
}

type EventDetailResp struct {
	Logs     []string `json:"logs"`
	Instance string   `json:"instance"`
}

type PageData struct {
	Hash     string
	Instance string
	Logs     []string
	Total    int
}

type AlertEvalPageData struct {
	RuleID   string
	Instance string
	Logs     []string
	Total    int
}

type TraceLogsPageData struct {
	TraceID  string
	Instance string
	Logs     []string
	Total    int
}

// GrepLogDir searches all log files in logDir for lines containing keyword,
// sorts them by timestamp descending, and truncates to MaxLogLines.
func GrepLogDir(logDir string, keyword string) ([]string, error) {
	logFiles, err := filepath.Glob(filepath.Join(logDir, "*.log*"))
	if err != nil {
		return nil, err
	}

	var logs []string
	for _, logFile := range logFiles {
		lines, err := GrepFile(logFile, keyword)
		if err != nil {
			continue
		}
		logs = append(logs, lines...)
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i] > logs[j]
	})

	if len(logs) > MaxLogLines {
		logs = logs[:MaxLogLines]
	}

	return logs, nil
}

// GrepLatestLogFiles searches only the current (non-rotated) log files in logDir
// (i.e. files matching *.log without any additional suffix like .log.20240101).
func GrepLatestLogFiles(logDir string, keyword string) ([]string, error) {
	logFiles, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		return nil, err
	}

	var logs []string
	for _, logFile := range logFiles {
		lines, err := GrepFile(logFile, keyword)
		if err != nil {
			continue
		}
		logs = append(logs, lines...)
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i] > logs[j]
	})

	if len(logs) > MaxLogLines {
		logs = logs[:MaxLogLines]
	}

	return logs, nil
}

// GrepFile scans a file line by line and returns lines containing the keyword.
func GrepFile(filePath string, keyword string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, keyword) {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

// RenderHTML writes the event detail HTML page to w.
func RenderHTML(w io.Writer, data PageData) error {
	return htmlTpl.Execute(w, data)
}

// RenderAlertEvalHTML writes the alert eval detail HTML page to w.
func RenderAlertEvalHTML(w io.Writer, data AlertEvalPageData) error {
	return alertEvalHtmlTpl.Execute(w, data)
}

// RenderTraceLogsHTML writes the trace logs HTML page to w.
func RenderTraceLogsHTML(w io.Writer, data TraceLogsPageData) error {
	return traceLogsHtmlTpl.Execute(w, data)
}

var htmlTpl = template.Must(template.New("event-detail").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Event Detail - {{.Hash}}</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
    background: #f0f2f5; color: #333;
  }
  .header {
    background: #fff; border-bottom: 1px solid #ebebeb;
    padding: 16px 24px; position: sticky; top: 0; z-index: 10;
    box-shadow: 0 1px 4px rgba(0,0,0,0.06);
  }
  .header-top { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }
  .header h1 { font-size: 18px; font-weight: 600; color: #333; }
  .header h1 span { color: #6C53B1; font-family: "SFMono-Regular", Consolas, monospace; font-size: 15px; }
  .badges { display: flex; gap: 8px; flex-wrap: wrap; }
  .badge {
    display: inline-flex; align-items: center; gap: 4px;
    padding: 2px 10px; border-radius: 12px; font-size: 12px; font-weight: 500;
  }
  .badge-instance { background: #f0ecf7; color: #6C53B1; }
  .badge-count { background: #f5f5f5; color: #666; }
  .toolbar {
    margin-top: 12px; display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
  }
  .search-box {
    flex: 1; min-width: 200px; position: relative;
  }
  .search-box input {
    width: 100%; padding: 6px 12px 6px 32px;
    background: #fff; border: 1px solid #d9d9d9; border-radius: 6px;
    color: #333; font-size: 13px; outline: none;
  }
  .search-box input:focus { border-color: #6C53B1; box-shadow: 0 0 0 2px rgba(108,83,177,0.2); }
  .search-box svg {
    position: absolute; left: 8px; top: 50%; transform: translateY(-50%);
    width: 16px; height: 16px; fill: #bfbfbf;
  }
  .filter-btns button {
    padding: 4px 12px; border-radius: 6px; border: 1px solid #d9d9d9;
    background: #fff; color: #666; font-size: 12px; cursor: pointer;
  }
  .filter-btns button:hover { border-color: #6C53B1; color: #6C53B1; }
  .filter-btns button.active { background: #f0ecf7; border-color: #6C53B1; color: #6C53B1; }
  .log-container { padding: 8px 0; background: #fff; margin: 12px; border-radius: 8px; border: 1px solid #ebebeb; }
  .log-line {
    display: flex; font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
    font-size: 12px; line-height: 20px; padding: 0 24px;
    border-bottom: 1px solid transparent;
  }
  .log-line:hover { background: #fafafa; border-color: #ebebeb; }
  .line-no {
    min-width: 48px; text-align: right; color: #bfbfbf;
    padding-right: 16px; user-select: none; flex-shrink: 0;
  }
  .line-content { white-space: pre-wrap; word-break: break-all; flex: 1; color: #333; }
  .line-content .ts { color: #1890ff; }
  .line-content .lv-DEBUG { color: #8c8c8c; }
  .line-content .lv-INFO { color: #1890ff; }
  .line-content .lv-WARNING { color: #fa8c16; }
  .line-content .lv-WARNINGF { color: #fa8c16; }
  .line-content .lv-ERROR { color: #f5222d; }
  .line-content .lv-ERRORF { color: #f5222d; }
  .line-content .hl { background: #fff7e6; color: #d46b08; border-radius: 2px; padding: 0 2px; }
  .hidden { display: none !important; }
  .empty-state {
    text-align: center; padding: 64px 24px; color: #bfbfbf; font-size: 14px;
  }
  .match-count { font-size: 12px; color: #999; white-space: nowrap; }
</style>
</head>
<body>
<div class="header">
  <div class="header-top">
    <h1>Event Detail &mdash; <span>{{.Hash}}</span></h1>
    <div class="badges">
      <span class="badge badge-instance">&#9881; {{.Instance}}</span>
      <span class="badge badge-count" id="countBadge">{{.Total}} lines</span>
    </div>
  </div>
  <div class="toolbar">
    <div class="search-box">
      <svg viewBox="0 0 16 16"><path d="M11.5 7a4.5 4.5 0 1 1-9 0 4.5 4.5 0 0 1 9 0Zm-.82 4.74a6 6 0 1 1 1.06-1.06l3.04 3.04a.75.75 0 1 1-1.06 1.06l-3.04-3.04Z"/></svg>
      <input type="text" id="searchInput" placeholder="Filter logs..." autocomplete="off">
    </div>
    <div class="filter-btns" id="levelBtns">
      <button data-level="all" class="active">All</button>
      <button data-level="ERROR">Error</button>
      <button data-level="WARNING">Warn</button>
      <button data-level="INFO">Info</button>
      <button data-level="DEBUG">Debug</button>
    </div>
    <span class="match-count" id="matchCount"></span>
  </div>
</div>
<div class="log-container" id="logContainer">
{{- if eq .Total 0}}
  <div class="empty-state">No log lines found for this event hash.</div>
{{- else}}
  {{- range $i, $line := .Logs}}
  <div class="log-line" data-idx="{{$i}}"><span class="line-no">{{$i}}</span><span class="line-content">{{$line}}</span></div>
  {{- end}}
{{- end}}
</div>

<script>
(function() {
  var hash = {{.Hash}};
  var lines = document.querySelectorAll('.log-line');
  var searchInput = document.getElementById('searchInput');
  var levelBtns = document.getElementById('levelBtns').querySelectorAll('button');
  var matchCount = document.getElementById('matchCount');
  var countBadge = document.getElementById('countBadge');
  var activeLevel = 'all';
  var LEVELS = ['DEBUG','INFO','WARNING','WARNINGF','ERROR','ERRORF'];
  var LEVEL_RE = /\b(DEBUG|INFO|WARNING|WARNINGF|ERROR|ERRORF)\b/;
  var TS_RE = /^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}[.\d]*)/;

  // colorize on load
  lines.forEach(function(el) {
    var content = el.querySelector('.line-content');
    var text = content.textContent;
    var html = escapeHtml(text);

    // highlight timestamp
    html = html.replace(TS_RE, '<span class="ts">$1</span>');

    // highlight level
    html = html.replace(LEVEL_RE, function(m) { return '<span class="lv-'+m+'">'+m+'</span>'; });

    // highlight hash
    if (hash) {
      html = html.split(escapeHtml(hash)).join('<span class="hl">'+escapeHtml(hash)+'</span>');
    }

    content.innerHTML = html;
    el.dataset.level = detectLevel(text);
  });

  // search filter
  var debounceTimer;
  searchInput.addEventListener('input', function() {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(applyFilters, 150);
  });

  // level filter
  levelBtns.forEach(function(btn) {
    btn.addEventListener('click', function() {
      levelBtns.forEach(function(b) { b.classList.remove('active'); });
      btn.classList.add('active');
      activeLevel = btn.dataset.level;
      applyFilters();
    });
  });

  function applyFilters() {
    var q = searchInput.value.toLowerCase();
    var visible = 0;
    lines.forEach(function(el) {
      var text = el.querySelector('.line-content').textContent.toLowerCase();
      var levelOk = activeLevel === 'all' || matchLevel(el.dataset.level, activeLevel);
      var searchOk = !q || text.indexOf(q) !== -1;
      if (levelOk && searchOk) {
        el.classList.remove('hidden');
        visible++;
      } else {
        el.classList.add('hidden');
      }
    });
    matchCount.textContent = q || activeLevel !== 'all' ? visible + ' / ' + lines.length + ' shown' : '';
  }

  function matchLevel(lineLevel, filter) {
    if (filter === 'ERROR') return lineLevel === 'ERROR' || lineLevel === 'ERRORF';
    if (filter === 'WARNING') return lineLevel === 'WARNING' || lineLevel === 'WARNINGF';
    return lineLevel === filter;
  }

  function detectLevel(text) {
    var m = text.match(LEVEL_RE);
    return m ? m[1] : '';
  }

  function escapeHtml(s) {
    var d = document.createElement('div');
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }
})();
</script>
</body>
</html>
`))

var traceLogsHtmlTpl = template.Must(template.New("trace-logs").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Trace Logs - {{.TraceID}}</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
    background: #f0f2f5; color: #333;
  }
  .header {
    background: #fff; border-bottom: 1px solid #ebebeb;
    padding: 16px 24px; position: sticky; top: 0; z-index: 10;
    box-shadow: 0 1px 4px rgba(0,0,0,0.06);
  }
  .header-top { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }
  .header h1 { font-size: 18px; font-weight: 600; color: #333; }
  .header h1 span { color: #6C53B1; font-family: "SFMono-Regular", Consolas, monospace; font-size: 15px; }
  .badges { display: flex; gap: 8px; flex-wrap: wrap; }
  .badge {
    display: inline-flex; align-items: center; gap: 4px;
    padding: 2px 10px; border-radius: 12px; font-size: 12px; font-weight: 500;
  }
  .badge-instance { background: #f0ecf7; color: #6C53B1; }
  .badge-count { background: #f5f5f5; color: #666; }
  .toolbar {
    margin-top: 12px; display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
  }
  .search-box {
    flex: 1; min-width: 200px; position: relative;
  }
  .search-box input {
    width: 100%; padding: 6px 12px 6px 32px;
    background: #fff; border: 1px solid #d9d9d9; border-radius: 6px;
    color: #333; font-size: 13px; outline: none;
  }
  .search-box input:focus { border-color: #6C53B1; box-shadow: 0 0 0 2px rgba(108,83,177,0.2); }
  .search-box svg {
    position: absolute; left: 8px; top: 50%; transform: translateY(-50%);
    width: 16px; height: 16px; fill: #bfbfbf;
  }
  .filter-btns button {
    padding: 4px 12px; border-radius: 6px; border: 1px solid #d9d9d9;
    background: #fff; color: #666; font-size: 12px; cursor: pointer;
  }
  .filter-btns button:hover { border-color: #6C53B1; color: #6C53B1; }
  .filter-btns button.active { background: #f0ecf7; border-color: #6C53B1; color: #6C53B1; }
  .log-container { padding: 8px 0; background: #fff; margin: 12px; border-radius: 8px; border: 1px solid #ebebeb; }
  .log-line {
    display: flex; font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
    font-size: 12px; line-height: 20px; padding: 0 24px;
    border-bottom: 1px solid transparent;
  }
  .log-line:hover { background: #fafafa; border-color: #ebebeb; }
  .line-no {
    min-width: 48px; text-align: right; color: #bfbfbf;
    padding-right: 16px; user-select: none; flex-shrink: 0;
  }
  .line-content { white-space: pre-wrap; word-break: break-all; flex: 1; color: #333; }
  .line-content .ts { color: #1890ff; }
  .line-content .lv-DEBUG { color: #8c8c8c; }
  .line-content .lv-INFO { color: #1890ff; }
  .line-content .lv-WARNING { color: #fa8c16; }
  .line-content .lv-WARNINGF { color: #fa8c16; }
  .line-content .lv-ERROR { color: #f5222d; }
  .line-content .lv-ERRORF { color: #f5222d; }
  .line-content .hl { background: #fff7e6; color: #d46b08; border-radius: 2px; padding: 0 2px; }
  .hidden { display: none !important; }
  .empty-state {
    text-align: center; padding: 64px 24px; color: #bfbfbf; font-size: 14px;
  }
  .match-count { font-size: 12px; color: #999; white-space: nowrap; }
</style>
</head>
<body>
<div class="header">
  <div class="header-top">
    <h1>Trace Logs &mdash; <span>{{.TraceID}}</span></h1>
    <div class="badges">
      <span class="badge badge-instance">&#9881; {{.Instance}}</span>
      <span class="badge badge-count" id="countBadge">{{.Total}} lines</span>
    </div>
  </div>
  <div class="toolbar">
    <div class="search-box">
      <svg viewBox="0 0 16 16"><path d="M11.5 7a4.5 4.5 0 1 1-9 0 4.5 4.5 0 0 1 9 0Zm-.82 4.74a6 6 0 1 1 1.06-1.06l3.04 3.04a.75.75 0 1 1-1.06 1.06l-3.04-3.04Z"/></svg>
      <input type="text" id="searchInput" placeholder="Filter logs..." autocomplete="off">
    </div>
    <div class="filter-btns" id="levelBtns">
      <button data-level="all" class="active">All</button>
      <button data-level="ERROR">Error</button>
      <button data-level="WARNING">Warn</button>
      <button data-level="INFO">Info</button>
      <button data-level="DEBUG">Debug</button>
    </div>
    <span class="match-count" id="matchCount"></span>
  </div>
</div>
<div class="log-container" id="logContainer">
{{- if eq .Total 0}}
  <div class="empty-state">No log lines found for trace ID {{.TraceID}}.</div>
{{- else}}
  {{- range $i, $line := .Logs}}
  <div class="log-line" data-idx="{{$i}}"><span class="line-no">{{$i}}</span><span class="line-content">{{$line}}</span></div>
  {{- end}}
{{- end}}
</div>

<script>
(function() {
  var keyword = "trace_id=" + {{.TraceID}};
  var lines = document.querySelectorAll('.log-line');
  var searchInput = document.getElementById('searchInput');
  var levelBtns = document.getElementById('levelBtns').querySelectorAll('button');
  var matchCount = document.getElementById('matchCount');
  var countBadge = document.getElementById('countBadge');
  var activeLevel = 'all';
  var LEVELS = ['DEBUG','INFO','WARNING','WARNINGF','ERROR','ERRORF'];
  var LEVEL_RE = /\b(DEBUG|INFO|WARNING|WARNINGF|ERROR|ERRORF)\b/;
  var TS_RE = /^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}[.\d]*)/;

  lines.forEach(function(el) {
    var content = el.querySelector('.line-content');
    var text = content.textContent;
    var html = escapeHtml(text);
    html = html.replace(TS_RE, '<span class="ts">$1</span>');
    html = html.replace(LEVEL_RE, function(m) { return '<span class="lv-'+m+'">'+m+'</span>'; });
    if (keyword) {
      html = html.split(escapeHtml(keyword)).join('<span class="hl">'+escapeHtml(keyword)+'</span>');
    }
    content.innerHTML = html;
    el.dataset.level = detectLevel(text);
  });

  var debounceTimer;
  searchInput.addEventListener('input', function() {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(applyFilters, 150);
  });

  levelBtns.forEach(function(btn) {
    btn.addEventListener('click', function() {
      levelBtns.forEach(function(b) { b.classList.remove('active'); });
      btn.classList.add('active');
      activeLevel = btn.dataset.level;
      applyFilters();
    });
  });

  function applyFilters() {
    var q = searchInput.value.toLowerCase();
    var visible = 0;
    lines.forEach(function(el) {
      var text = el.querySelector('.line-content').textContent.toLowerCase();
      var levelOk = activeLevel === 'all' || matchLevel(el.dataset.level, activeLevel);
      var searchOk = !q || text.indexOf(q) !== -1;
      if (levelOk && searchOk) {
        el.classList.remove('hidden');
        visible++;
      } else {
        el.classList.add('hidden');
      }
    });
    matchCount.textContent = q || activeLevel !== 'all' ? visible + ' / ' + lines.length + ' shown' : '';
  }

  function matchLevel(lineLevel, filter) {
    if (filter === 'ERROR') return lineLevel === 'ERROR' || lineLevel === 'ERRORF';
    if (filter === 'WARNING') return lineLevel === 'WARNING' || lineLevel === 'WARNINGF';
    return lineLevel === filter;
  }

  function detectLevel(text) {
    var m = text.match(LEVEL_RE);
    return m ? m[1] : '';
  }

  function escapeHtml(s) {
    var d = document.createElement('div');
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }
})();
</script>
</body>
</html>
`))

var alertEvalHtmlTpl = template.Must(template.New("alert-eval-detail").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Alert Eval Detail - Rule {{.RuleID}}</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
    background: #f0f2f5; color: #333;
  }
  .header {
    background: #fff; border-bottom: 1px solid #ebebeb;
    padding: 16px 24px; position: sticky; top: 0; z-index: 10;
    box-shadow: 0 1px 4px rgba(0,0,0,0.06);
  }
  .header-top { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }
  .header h1 { font-size: 18px; font-weight: 600; color: #333; }
  .header h1 span { color: #6C53B1; font-family: "SFMono-Regular", Consolas, monospace; font-size: 15px; }
  .badges { display: flex; gap: 8px; flex-wrap: wrap; }
  .badge {
    display: inline-flex; align-items: center; gap: 4px;
    padding: 2px 10px; border-radius: 12px; font-size: 12px; font-weight: 500;
  }
  .badge-instance { background: #f0ecf7; color: #6C53B1; }
  .badge-count { background: #f5f5f5; color: #666; }
  .toolbar {
    margin-top: 12px; display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
  }
  .search-box {
    flex: 1; min-width: 200px; position: relative;
  }
  .search-box input {
    width: 100%; padding: 6px 12px 6px 32px;
    background: #fff; border: 1px solid #d9d9d9; border-radius: 6px;
    color: #333; font-size: 13px; outline: none;
  }
  .search-box input:focus { border-color: #6C53B1; box-shadow: 0 0 0 2px rgba(108,83,177,0.2); }
  .search-box svg {
    position: absolute; left: 8px; top: 50%; transform: translateY(-50%);
    width: 16px; height: 16px; fill: #bfbfbf;
  }
  .filter-btns button {
    padding: 4px 12px; border-radius: 6px; border: 1px solid #d9d9d9;
    background: #fff; color: #666; font-size: 12px; cursor: pointer;
  }
  .filter-btns button:hover { border-color: #6C53B1; color: #6C53B1; }
  .filter-btns button.active { background: #f0ecf7; border-color: #6C53B1; color: #6C53B1; }
  .log-container { padding: 8px 0; background: #fff; margin: 12px; border-radius: 8px; border: 1px solid #ebebeb; }
  .log-line {
    display: flex; font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
    font-size: 12px; line-height: 20px; padding: 0 24px;
    border-bottom: 1px solid transparent;
  }
  .log-line:hover { background: #fafafa; border-color: #ebebeb; }
  .line-no {
    min-width: 48px; text-align: right; color: #bfbfbf;
    padding-right: 16px; user-select: none; flex-shrink: 0;
  }
  .line-content { white-space: pre-wrap; word-break: break-all; flex: 1; color: #333; }
  .line-content .ts { color: #1890ff; }
  .line-content .lv-DEBUG { color: #8c8c8c; }
  .line-content .lv-INFO { color: #1890ff; }
  .line-content .lv-WARNING { color: #fa8c16; }
  .line-content .lv-WARNINGF { color: #fa8c16; }
  .line-content .lv-ERROR { color: #f5222d; }
  .line-content .lv-ERRORF { color: #f5222d; }
  .line-content .hl { background: #fff7e6; color: #d46b08; border-radius: 2px; padding: 0 2px; }
  .line-content a.event-hash { color: #6C53B1; text-decoration: underline; text-decoration-style: dotted; }
  .line-content a.event-hash:hover { color: #531dab; text-decoration-style: solid; }
  .hidden { display: none !important; }
  .empty-state {
    text-align: center; padding: 64px 24px; color: #bfbfbf; font-size: 14px;
  }
  .match-count { font-size: 12px; color: #999; white-space: nowrap; }
</style>
</head>
<body>
<div class="header">
  <div class="header-top">
    <h1>Alert Eval Detail &mdash; Rule <span>{{.RuleID}}</span></h1>
    <div class="badges">
      <span class="badge badge-instance">&#9881; {{.Instance}}</span>
      <span class="badge badge-count" id="countBadge">{{.Total}} lines</span>
    </div>
  </div>
  <div class="toolbar">
    <div class="search-box">
      <svg viewBox="0 0 16 16"><path d="M11.5 7a4.5 4.5 0 1 1-9 0 4.5 4.5 0 0 1 9 0Zm-.82 4.74a6 6 0 1 1 1.06-1.06l3.04 3.04a.75.75 0 1 1-1.06 1.06l-3.04-3.04Z"/></svg>
      <input type="text" id="searchInput" placeholder="Filter logs..." autocomplete="off">
    </div>
    <div class="filter-btns" id="levelBtns">
      <button data-level="all" class="active">All</button>
      <button data-level="ERROR">Error</button>
      <button data-level="WARNING">Warn</button>
      <button data-level="INFO">Info</button>
      <button data-level="DEBUG">Debug</button>
    </div>
    <span class="match-count" id="matchCount"></span>
  </div>
</div>
<div class="log-container" id="logContainer">
{{- if eq .Total 0}}
  <div class="empty-state">No log lines found for alert rule {{.RuleID}}.</div>
{{- else}}
  {{- range $i, $line := .Logs}}
  <div class="log-line" data-idx="{{$i}}"><span class="line-no">{{$i}}</span><span class="line-content">{{$line}}</span></div>
  {{- end}}
{{- end}}
</div>

<script>
(function() {
  var keyword = "alert_eval_" + {{.RuleID}};
  var lines = document.querySelectorAll('.log-line');
  var searchInput = document.getElementById('searchInput');
  var levelBtns = document.getElementById('levelBtns').querySelectorAll('button');
  var matchCount = document.getElementById('matchCount');
  var countBadge = document.getElementById('countBadge');
  var activeLevel = 'all';
  var LEVELS = ['DEBUG','INFO','WARNING','WARNINGF','ERROR','ERRORF'];
  var LEVEL_RE = /\b(DEBUG|INFO|WARNING|WARNINGF|ERROR|ERRORF)\b/;
  var TS_RE = /^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}[.\d]*)/;
  var HASH_RE = /\b([0-9a-f]{32})\b/g;

  // colorize on load
  lines.forEach(function(el) {
    var content = el.querySelector('.line-content');
    var text = content.textContent;
    var html = escapeHtml(text);

    // highlight timestamp
    html = html.replace(TS_RE, '<span class="ts">$1</span>');

    // highlight level
    html = html.replace(LEVEL_RE, function(m) { return '<span class="lv-'+m+'">'+m+'</span>'; });

    // highlight keyword
    if (keyword) {
      html = html.split(escapeHtml(keyword)).join('<span class="hl">'+escapeHtml(keyword)+'</span>');
    }

    // linkify event hash
    html = html.replace(HASH_RE, function(m) { return '<a class="event-hash" href="../event-detail/'+m+'" target="_blank">'+m+'</a>'; });

    content.innerHTML = html;
    el.dataset.level = detectLevel(text);
  });

  // search filter
  var debounceTimer;
  searchInput.addEventListener('input', function() {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(applyFilters, 150);
  });

  // level filter
  levelBtns.forEach(function(btn) {
    btn.addEventListener('click', function() {
      levelBtns.forEach(function(b) { b.classList.remove('active'); });
      btn.classList.add('active');
      activeLevel = btn.dataset.level;
      applyFilters();
    });
  });

  function applyFilters() {
    var q = searchInput.value.toLowerCase();
    var visible = 0;
    lines.forEach(function(el) {
      var text = el.querySelector('.line-content').textContent.toLowerCase();
      var levelOk = activeLevel === 'all' || matchLevel(el.dataset.level, activeLevel);
      var searchOk = !q || text.indexOf(q) !== -1;
      if (levelOk && searchOk) {
        el.classList.remove('hidden');
        visible++;
      } else {
        el.classList.add('hidden');
      }
    });
    matchCount.textContent = q || activeLevel !== 'all' ? visible + ' / ' + lines.length + ' shown' : '';
  }

  function matchLevel(lineLevel, filter) {
    if (filter === 'ERROR') return lineLevel === 'ERROR' || lineLevel === 'ERRORF';
    if (filter === 'WARNING') return lineLevel === 'WARNING' || lineLevel === 'WARNINGF';
    return lineLevel === filter;
  }

  function detectLevel(text) {
    var m = text.match(LEVEL_RE);
    return m ? m[1] : '';
  }

  function escapeHtml(s) {
    var d = document.createElement('div');
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }
})();
</script>
</body>
</html>
`))
