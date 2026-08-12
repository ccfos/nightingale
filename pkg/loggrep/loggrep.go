package loggrep

import (
	"bufio"
	"context"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const MaxLogLines = 5000

// DefaultTimeout bounds a single search. A log dir routinely reaches several
// GB (size based rotation keeps RotateNum files per level), and scanning all
// of it on a cold page cache outlives the read timeout of any reverse proxy
// in front of n9e. Rather than let the request die there, the search returns
// whatever it collected within the budget and marks the result truncated.
const DefaultTimeout = 10 * time.Second

// scanWindow bounds how much of a file is read in one pass. A file is read one
// window at a time from its end backwards, so a scan cut short by the deadline
// has already covered the newest part of it. Reading a large file forward would
// hand back its oldest lines instead, and the lines being looked for - the event
// that just fired, the trace that was just served - are the ones at the end.
// The window is also the granularity at which the deadline is checked, so it
// has to stay small enough to finish well inside the budget on a cold cache.
// A var rather than a const so the tests can shrink it instead of writing
// multi-MB fixtures to prove the windows tile the file correctly.
var scanWindow int64 = 16 << 20 // 16MB

// maxLineBytes caps how much of one line is kept for matching. Anything longer
// is matched on its head and the remainder is consumed and dropped - unlike a
// bufio.Scanner buffer overflow, which used to abandon the rest of the file
// without saying so. A var for the same reason as scanWindow.
var maxLineBytes = 1 << 20 // 1MB

const (
	// ReasonTimeout means the budget ran out before every candidate file was
	// scanned, so older matches may be missing.
	ReasonTimeout = "timeout"
	// ReasonLimit means more lines matched than Limit allows, so the oldest
	// matches were dropped.
	ReasonLimit = "limit"
)

// GrepOptions bounds a search. The zero value is valid and applies defaults.
type GrepOptions struct {
	// Deadline stops the scan. Zero means DefaultTimeout from the start.
	Deadline time.Time
	// Limit caps the returned lines, newest kept. Zero means MaxLogLines.
	Limit int
	// Since skips log files last written before it. This is what makes an
	// event lookup cheap: a file rotated away before the event existed cannot
	// mention it, so the big historical files never get opened.
	Since time.Time
}

// GrepResult carries the matched lines plus whether they are the complete set.
type GrepResult struct {
	Logs      []string `json:"logs"`
	Truncated bool     `json:"truncated"`
	Reason    string   `json:"reason,omitempty"`
}

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
	Logs      []string `json:"logs"`
	Instance  string   `json:"instance"`
	Truncated bool     `json:"truncated"`
	Reason    string   `json:"reason,omitempty"`
}

type PageData struct {
	Hash string
}

type AlertEvalPageData struct {
	RuleID string
}

type TraceLogsPageData struct {
	TraceID string
}

// GrepLogDir searches all log files in logDir, rotated ones included, for
// lines containing keyword and returns the newest matches.
func GrepLogDir(ctx context.Context, logDir, keyword string, opts GrepOptions) GrepResult {
	return grep(ctx, filepath.Join(logDir, "*.log*"), keyword, opts)
}

// GrepLatestLogFiles searches only the current (non-rotated) log files in
// logDir, i.e. files matching *.log without a suffix like .log.20240101.
func GrepLatestLogFiles(ctx context.Context, logDir, keyword string, opts GrepOptions) GrepResult {
	return grep(ctx, filepath.Join(logDir, "*.log"), keyword, opts)
}

// logFile is a scan candidate. group is the log file's name with any rotation
// suffix stripped ("INFO.log.000" -> "INFO.log"), which identifies the
// rotation chain the file belongs to.
type logFile struct {
	path    string
	group   string
	modTime time.Time
}

// grep scans the files matching pattern newest first and returns the newest
// matches, bounded by both the deadline and the line limit. Scanning newest
// first is what makes a partial result useful: whatever the budget buys is the
// most recent activity, which is what these log pages are read for. That holds
// inside a file as well - see grepFile - so a result cut short by the deadline
// is missing older lines, never newer ones.
func grep(ctx context.Context, pattern, keyword string, opts GrepOptions) GrepResult {
	limit := opts.Limit
	if limit <= 0 {
		limit = MaxLogLines
	}

	deadline := opts.Deadline
	if deadline.IsZero() {
		deadline = time.Now().Add(DefaultTimeout)
	}
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	files := candidates(pattern, opts.Since)

	var logs []string
	var timedOut bool
	// unread marks that some candidate data was never looked at, whether the
	// deadline cut the scan short or a file could not be read to its start.
	// Either way the missing lines are the older ones.
	var unread bool

	// matched counts matches per rotation chain. Once a chain has yielded
	// limit lines, its older files are skipped: every line in them is older
	// than limit lines already held from the same chain, so none can survive
	// the final truncation. The count is per chain rather than global because
	// the level files (INFO.log, ERROR.log, ...) are concurrent streams, not
	// successive rotations - a global count would let a chatty INFO stream
	// hide the ERROR lines of the very event being investigated.
	matched := make(map[string]int, len(files))

	for _, f := range files {
		if matched[f.group] >= limit {
			continue
		}

		// checked per file as well as inside the scan: a dir made of many
		// small files would otherwise never reach the in-loop check
		if ctx.Err() != nil || time.Now().After(deadline) {
			timedOut = true
			break
		}

		lines, stopped, incomplete := grepFile(ctx, f.path, keyword, deadline, limit)
		logs = append(logs, lines...)
		matched[f.group] += len(lines)

		if incomplete {
			unread = true
		}

		// the deadline is gone for every remaining file too, so stop here;
		// a file that merely could not be read to its start does not say
		// anything about the others, so that one only sets the flag
		if stopped {
			timedOut = true
			break
		}
	}

	sort.Slice(logs, func(i, j int) bool {
		return logs[i] > logs[j]
	})

	result := GrepResult{Logs: logs}

	if len(logs) > limit {
		result.Logs = logs[:limit]
		result.Truncated = true
		result.Reason = ReasonLimit
	}

	// an unread remainder wins the reason: it means log data was never looked
	// at, which hides more than dropping the tail of an over-long result does
	if timedOut || unread {
		result.Truncated = true
		result.Reason = ReasonTimeout
	}

	return result
}

// candidates returns the files matching pattern, newest first, dropping those
// last written before since.
func candidates(pattern string, since time.Time) []logFile {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	files := make([]logFile, 0, len(paths))
	for _, path := range paths {
		st, err := os.Stat(path)
		if err != nil || st.IsDir() || st.Size() == 0 {
			continue
		}

		// mtime is the file's last write, so a file untouched since before
		// the cutoff holds nothing newer than it
		if !since.IsZero() && st.ModTime().Before(since) {
			continue
		}

		files = append(files, logFile{
			path:    path,
			group:   rotationGroup(path),
			modTime: st.ModTime(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].path < files[j].path
		}
		return files[i].modTime.After(files[j].modTime)
	})

	return files
}

// rotationGroup maps a log file onto its rotation chain by dropping whatever
// the rotation appended after ".log" ("INFO.log.000" and "INFO.log.2026072711"
// both belong to "INFO.log").
func rotationGroup(path string) string {
	name := filepath.Base(path)
	if i := strings.Index(name, ".log"); i >= 0 {
		return name[:i+len(".log")]
	}
	return name
}

// grepFile returns the lines of a file containing keyword, newest first. The
// file is read one scanWindow at a time from its end backwards, so the budget
// is spent on the most recent lines: a single size-rotated file can be hundreds
// of MB and blow the whole budget on its own, and reading it forward would hand
// back its oldest lines while the event being investigated sits at the end.
//
// stopped reports that the deadline (or the context) cut the scan short, which
// says nothing more can be scanned at all. incomplete reports that this file
// was not read back to its start for any reason - the deadline, or a read error
// - so older matches in it may be missing.
func grepFile(ctx context.Context, filePath, keyword string, deadline time.Time, limit int) (lines []string, stopped bool, incomplete bool) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, false, true
	}
	defer f.Close()

	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, false, true
	}

	// a non-positive window would leave the loop below never advancing
	window := scanWindow
	if window <= 0 {
		window = 16 << 20
	}

	for end := size; end > 0; {
		if ctx.Err() != nil || time.Now().After(deadline) {
			return lines, true, true
		}

		start := end - window
		if start < 0 {
			start = 0
		}

		found, err := grepWindow(f, start, end, keyword)
		lines = append(lines, found...)
		if err != nil {
			// a file rotated away mid scan, a bad sector: the matches already
			// collected stay usable, but the older part of the file is lost
			return lines, false, true
		}

		// one match past the limit already proves the result is truncated, and
		// every remaining match in this file is older than the ones in hand, so
		// none of them could survive the final truncation anyway
		if len(lines) > limit {
			return lines, false, false
		}

		end = start
	}

	return lines, false, false
}

// grepWindow returns the lines containing keyword that begin inside
// [start, end). A line beginning before start belongs to the next window down
// and is skipped here; a line beginning inside the window but running past end
// is read whole. Together those two rules make the windows tile the file
// exactly - no line is dropped at a boundary and none is returned twice.
func grepWindow(f *os.File, start, end int64, keyword string) ([]string, error) {
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}

	r := bufio.NewReaderSize(f, 256*1024)
	off := start

	if start > 0 {
		// the line straddling start is the previous window's business
		n, err := readLine(r, io.Discard)
		off += n
		if err != nil {
			return nil, ignoreEOF(err)
		}
	}

	var lines []string
	var sb strings.Builder

	for off <= end {
		sb.Reset()
		n, err := readLine(r, &sb)
		off += n

		if n > 0 {
			if line := sb.String(); strings.Contains(line, keyword) {
				lines = append(lines, line)
			}
		}

		if err != nil {
			return lines, ignoreEOF(err)
		}
	}

	return lines, nil
}

// readLine consumes up to and including the next newline, writing the line
// without its trailing newline to out, and returns how many bytes it consumed.
// Only the first maxLineBytes of a line are written; the rest is consumed and
// dropped, so an over-long line costs that one line rather than the remainder
// of the file.
func readLine(r *bufio.Reader, out io.Writer) (int64, error) {
	var consumed int64
	var written int

	for {
		chunk, err := r.ReadSlice('\n')
		consumed += int64(len(chunk))

		if err == nil || err == io.EOF {
			chunk = trimEOL(chunk)
		}

		if room := maxLineBytes - written; room > 0 && len(chunk) > 0 {
			if len(chunk) > room {
				chunk = chunk[:room]
			}
			n, werr := out.Write(chunk)
			written += n
			if werr != nil {
				return consumed, werr
			}
		}

		// ReadSlice fills its buffer and returns without the delimiter when a
		// line is longer than the buffer; keep going until the line ends
		if err == bufio.ErrBufferFull {
			continue
		}

		return consumed, err
	}
}

func trimEOL(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
		if n = len(b); n > 0 && b[n-1] == '\r' {
			b = b[:n-1]
		}
	}
	return b
}

func ignoreEOF(err error) error {
	if err == io.EOF {
		return nil
	}
	return err
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

// buildLogPageTpl assembles a log viewer page template from the shared shell.
// The page itself is a data-free shell served without authentication: its JS
// fetches "<page url>/logs" carrying the login JWT that the Nightingale UI
// keeps in localStorage (same origin), and the backend enforces permissions
// on that logs endpoint.
func buildLogPageTpl(name, title, heading, pageVars string) *template.Template {
	page := strings.NewReplacer(
		"__TITLE__", title,
		"__HEADING__", heading,
		"__PAGE_VARS__", pageVars,
	).Replace(logPageTplText)
	return template.Must(template.New(name).Parse(page))
}

var htmlTpl = buildLogPageTpl("event-detail",
	`Event Detail - {{.Hash}}`,
	`Event Detail &mdash; <span>{{.Hash}}</span>`,
	`var highlight = {{.Hash}};
  var linkifyHashes = false;`)

var alertEvalHtmlTpl = buildLogPageTpl("alert-eval-detail",
	`Alert Eval Detail - Rule {{.RuleID}}`,
	`Alert Eval Detail &mdash; Rule <span>{{.RuleID}}</span>`,
	`var highlight = "alert_eval_" + {{.RuleID}};
  var linkifyHashes = true;`)

var traceLogsHtmlTpl = buildLogPageTpl("trace-logs",
	`Trace Logs - {{.TraceID}}`,
	`Trace Logs &mdash; <span>{{.TraceID}}</span>`,
	`var highlight = "trace_id=" + {{.TraceID}};
  var linkifyHashes = false;`)

const logPageTplText = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>__TITLE__</title>
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
  .notice {
    margin: 12px 12px 0; padding: 8px 12px; border-radius: 6px;
    background: #fffbe6; border: 1px solid #ffe58f; color: #ad6800; font-size: 12px;
  }
</style>
</head>
<body>
<div class="header">
  <div class="header-top">
    <h1>__HEADING__</h1>
    <div class="badges">
      <span class="badge badge-instance" id="instanceBadge">&#9881; -</span>
      <span class="badge badge-count" id="countBadge">loading</span>
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
<div class="notice hidden" id="notice"></div>
<div class="log-container" id="logContainer">
  <div class="empty-state">Loading logs...</div>
</div>

<script>
(function() {
  __PAGE_VARS__
  var searchInput = document.getElementById('searchInput');
  var levelBtns = document.getElementById('levelBtns').querySelectorAll('button');
  var matchCount = document.getElementById('matchCount');
  var countBadge = document.getElementById('countBadge');
  var instanceBadge = document.getElementById('instanceBadge');
  var container = document.getElementById('logContainer');
  var activeLevel = 'all';
  var lines = [];
  var LEVEL_RE = /\b(DEBUG|INFO|WARNING|WARNINGF|ERROR|ERRORF)\b/;
  var TS_RE = /^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}[.\d]*)/;
  var HASH_RE = /\b([0-9a-f]{32})\b/g;

  // the logs endpoint requires a login: reuse the JWT the Nightingale UI
  // keeps in localStorage (same origin), sent as the Authorization header
  fetch(location.pathname + '/logs', {
    headers: { 'Authorization': 'Bearer ' + (localStorage.getItem('access_token') || '') }
  }).then(function(resp) {
    // auth middleware errors come back as plain text, handler errors as JSON
    return resp.text().then(function(text) {
      var body = null;
      try { body = JSON.parse(text); } catch (e) {}
      return { status: resp.status, body: body, text: text };
    });
  }).then(function(r) {
    var err = (r.body && r.body.err) || (r.body ? '' : r.text);
    if (r.status !== 200 || err) {
      showStatus(statusMsg(r.status, err || 'HTTP ' + r.status));
      return;
    }
    var dat = (r.body && r.body.dat) || {};
    if (dat.truncated) { showNotice(dat.reason); }
    renderLines(dat.logs || [], dat.instance || '-');
  }).catch(function(e) {
    showStatus('failed to load logs: ' + e);
  });

  function showNotice(reason) {
    var notice = document.getElementById('notice');
    notice.textContent = reason === 'limit'
      ? 'Partial result: more lines matched than can be shown. Only the most recent ones are listed.'
      : 'Partial result: the search hit its time budget and stopped. The most recent logs are listed; older ones were not read.';
    notice.classList.remove('hidden');
  }

  function statusMsg(status, err) {
    if (status === 401) return 'unauthorized: please sign in to Nightingale in this browser, then reload this page';
    if (status === 403) return 'forbidden: you have no permission on the busi group these logs belong to';
    return err;
  }

  function showStatus(msg) {
    countBadge.textContent = '0 lines';
    container.innerHTML = '<div class="empty-state"></div>';
    container.firstChild.textContent = msg;
  }

  function renderLines(logs, instance) {
    instanceBadge.textContent = '⚙ ' + instance;
    countBadge.textContent = logs.length + ' lines';

    if (!logs.length) {
      container.innerHTML = '<div class="empty-state">No log lines found.</div>';
      return;
    }

    var parts = [];
    for (var i = 0; i < logs.length; i++) {
      var text = logs[i];
      var html = escapeHtml(text);

      // highlight timestamp
      html = html.replace(TS_RE, '<span class="ts">$1</span>');

      // highlight level
      html = html.replace(LEVEL_RE, function(m) { return '<span class="lv-'+m+'">'+m+'</span>'; });

      // highlight keyword
      if (highlight) {
        html = html.split(escapeHtml(highlight)).join('<span class="hl">'+escapeHtml(highlight)+'</span>');
      }

      // linkify event hash (alert-eval-detail page only)
      if (linkifyHashes) {
        html = html.replace(HASH_RE, function(m) { return '<a class="event-hash" href="../event-detail/'+m+'" target="_blank">'+m+'</a>'; });
      }

      parts.push('<div class="log-line" data-level="'+detectLevel(text)+'"><span class="line-no">'+i+'</span><span class="line-content">'+html+'</span></div>');
    }
    container.innerHTML = parts.join('');
    lines = container.querySelectorAll('.log-line');
  }

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
`
