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

// IsValidHash checks whether s looks like a valid MD5/SHA hex hash.
func IsValidHash(s string) bool {
	return hashPattern.MatchString(s)
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
    background: #0d1117; color: #c9d1d9;
  }
  .header {
    background: #161b22; border-bottom: 1px solid #30363d;
    padding: 16px 24px; position: sticky; top: 0; z-index: 10;
  }
  .header-top { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }
  .header h1 { font-size: 18px; font-weight: 600; color: #f0f6fc; }
  .header h1 span { color: #7ee787; font-family: "SFMono-Regular", Consolas, monospace; font-size: 15px; }
  .badges { display: flex; gap: 8px; flex-wrap: wrap; }
  .badge {
    display: inline-flex; align-items: center; gap: 4px;
    padding: 2px 10px; border-radius: 12px; font-size: 12px; font-weight: 500;
  }
  .badge-instance { background: #1f3a5f; color: #79c0ff; }
  .badge-count { background: #272e37; color: #8b949e; }
  .toolbar {
    margin-top: 12px; display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
  }
  .search-box {
    flex: 1; min-width: 200px; position: relative;
  }
  .search-box input {
    width: 100%; padding: 6px 12px 6px 32px;
    background: #0d1117; border: 1px solid #30363d; border-radius: 6px;
    color: #c9d1d9; font-size: 13px; outline: none;
  }
  .search-box input:focus { border-color: #58a6ff; box-shadow: 0 0 0 2px rgba(56,139,253,0.3); }
  .search-box svg {
    position: absolute; left: 8px; top: 50%; transform: translateY(-50%);
    width: 16px; height: 16px; fill: #484f58;
  }
  .filter-btns button {
    padding: 4px 12px; border-radius: 6px; border: 1px solid #30363d;
    background: transparent; color: #8b949e; font-size: 12px; cursor: pointer;
  }
  .filter-btns button:hover { border-color: #58a6ff; color: #58a6ff; }
  .filter-btns button.active { background: #1f3a5f; border-color: #58a6ff; color: #79c0ff; }
  .log-container { padding: 8px 0; }
  .log-line {
    display: flex; font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
    font-size: 12px; line-height: 20px; padding: 0 24px;
    border-bottom: 1px solid transparent;
  }
  .log-line:hover { background: #161b22; border-color: #30363d; }
  .line-no {
    min-width: 48px; text-align: right; color: #484f58;
    padding-right: 16px; user-select: none; flex-shrink: 0;
  }
  .line-content { white-space: pre-wrap; word-break: break-all; flex: 1; }
  .line-content .ts { color: #7ee787; }
  .line-content .lv-DEBUG { color: #8b949e; }
  .line-content .lv-INFO { color: #79c0ff; }
  .line-content .lv-WARNING { color: #d29922; }
  .line-content .lv-WARNINGF { color: #d29922; }
  .line-content .lv-ERROR { color: #f85149; }
  .line-content .lv-ERRORF { color: #f85149; }
  .line-content .hl { background: #533d08; color: #f0c239; border-radius: 2px; padding: 0 1px; }
  .hidden { display: none !important; }
  .empty-state {
    text-align: center; padding: 64px 24px; color: #484f58; font-size: 14px;
  }
  .match-count { font-size: 12px; color: #8b949e; white-space: nowrap; }
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
