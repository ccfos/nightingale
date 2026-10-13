package provider

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
)

// Use only Adaptive Card 1.2 elements, supported by Teams Workflows on desktop
// and mobile. The editable content stays a separate Markdown block.
type teamsElement struct {
	Type                string         `json:"type"`
	Text                string         `json:"text,omitempty"`
	Size                string         `json:"size,omitempty"`
	Weight              string         `json:"weight,omitempty"`
	Color               string         `json:"color,omitempty"`
	Wrap                bool           `json:"wrap,omitempty"`
	IsSubtle            bool           `json:"isSubtle,omitempty"`
	Spacing             string         `json:"spacing,omitempty"`
	Separator           bool           `json:"separator,omitempty"`
	Style               string         `json:"style,omitempty"`
	Bleed               bool           `json:"bleed,omitempty"`
	Width               string         `json:"width,omitempty"`
	HorizontalAlignment string         `json:"horizontalAlignment,omitempty"`
	Items               []teamsElement `json:"items,omitempty"`
	Columns             []teamsElement `json:"columns,omitempty"`
}

type teamsAction struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type teamsAdaptiveCard struct {
	Schema  string            `json:"$schema"`
	Type    string            `json:"type"`
	Version string            `json:"version"`
	MSTeams map[string]string `json:"msteams"`
	Body    []teamsElement    `json:"body"`
	Actions []teamsAction     `json:"actions,omitempty"`
}

func buildTeamsCard(req *NotifyRequest) teamsAdaptiveCard {
	var event *models.AlertCurEvent
	if len(req.Events) > 0 {
		event = req.Events[0]
	}
	status, color, style := "Notification", "Accent", "emphasis"
	title := tplString(req.TplContent, "title")
	if event != nil {
		status, color, style = "Triggered", "Attention", "attention"
		if event.IsRecovered {
			status, color, style = "Recovered", "Good", "good"
		} else if event.Severity > 1 {
			color, style = "Warning", "warning"
		}
		status = fmt.Sprintf("S%d · %s", event.Severity, status)
		if title == "" {
			title = event.RuleName
		}
	}
	if title == "" {
		title = "Nightingale alert"
	}
	title, _ = truncateInRunes(strings.Join(strings.Fields(title), " "), 256)
	statusText := teamsText("●  "+status, "Small", "Bolder", color)
	statusText.HorizontalAlignment = "Right"
	brand := teamsText("Nightingale", "Small", "Bolder", "Default")
	brand.IsSubtle = true
	card := teamsAdaptiveCard{
		Schema: "http://adaptivecards.io/schemas/adaptive-card.json", Type: "AdaptiveCard", Version: "1.2",
		MSTeams: map[string]string{"width": "Full"},
		Body: []teamsElement{{Type: "Container", Style: style, Bleed: true, Items: []teamsElement{
			{Type: "ColumnSet", Columns: []teamsElement{
				{Type: "Column", Width: "stretch", Items: []teamsElement{brand}},
				{Type: "Column", Width: "auto", Items: []teamsElement{statusText}},
			}},
			teamsText(teamsLiteral(title), "Medium", "Bolder", "Default"),
		}}},
	}
	if event != nil {
		columns := []teamsElement{}
		if event.TargetIdent != "" {
			columns = append(columns, teamsSummary("Target", event.TargetIdent, "Default", "Medium"))
		}
		if event.IsRecovered {
			if event.FirstTriggerTime > 0 && event.LastEvalTime >= event.FirstTriggerTime {
				duration := tplx.HumanizeDurationFloat64(float64(event.LastEvalTime - event.FirstTriggerTime))
				columns = append(columns, teamsSummary("Duration", duration, "Good", "Large"))
			}
		} else if event.TriggerValue != "" {
			columns = append(columns, teamsSummary("Trigger value", event.TriggerValue, color, "Large"))
		}
		if len(columns) > 0 {
			card.Body = append(card.Body, teamsElement{Type: "ColumnSet", Spacing: "Medium", Columns: columns})
		}
	}
	content, _ := truncateInRunes(tplString(req.TplContent, "content"), 4000)
	if strings.TrimSpace(content) != "" {
		block := teamsText(content, "Default", "Default", "Default")
		block.Spacing = "Medium"
		block.Separator = true
		card.Body = append(card.Body, block)
	}
	if event != nil {
		stamp, label := event.FirstTriggerTime, "Started"
		if stamp <= 0 {
			stamp = event.TriggerTime
		}
		if event.IsRecovered {
			stamp, label = event.LastEvalTime, "Recovered"
		}
		if stamp > 0 {
			// Teams localizes DATE/TIME to the viewer's timezone. This avoids confusing
			// a timestamp formatted in the alert engine's timezone with local time.
			iso := time.Unix(stamp, 0).UTC().Format(time.RFC3339)
			footer := teamsText(fmt.Sprintf("%s · {{DATE(%s, SHORT)}} {{TIME(%s)}}", label, iso, iso), "Small", "Default", "Default")
			footer.IsSubtle = true
			footer.Spacing = "Medium"
			card.Body = append(card.Body, footer)
		}
		card.Actions = teamsActions(req.SiteUrl, event)
	}
	return card
}

func teamsText(text, size, weight, color string) teamsElement {
	return teamsElement{Type: "TextBlock", Text: text, Size: size, Weight: weight, Color: color, Wrap: true, Spacing: "Small"}
}

func teamsSummary(label, value, color, size string) teamsElement {
	caption := teamsText(label, "Small", "Default", "Default")
	caption.IsSubtle = true
	value, _ = truncateInRunes(strings.Join(strings.Fields(value), " "), 256)
	return teamsElement{Type: "Column", Width: "stretch", Items: []teamsElement{caption, teamsText(teamsLiteral(value), size, "Bolder", color)}}
}

// Keep rule names, target identifiers and metric values literal in Markdown.
// User-authored content is deliberately left as Markdown.
var teamsMarkdownEscaper = strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "`", "\\`", "[", `\[`, "]", `\]`)

func teamsLiteral(s string) string { return teamsMarkdownEscaper.Replace(s) }

func teamsActions(siteURL string, event *models.AlertCurEvent) []teamsAction {
	base := strings.TrimRight(strings.TrimSpace(siteURL), "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || event.Id <= 0 {
		return nil
	}
	id := strconv.FormatInt(event.Id, 10)
	actions := []teamsAction{{Type: "Action.OpenUrl", Title: "View alert", URL: base + "/share/alert-his-events/" + id}}
	if !event.IsRecovered {
		actions = append(actions, teamsAction{Type: "Action.OpenUrl", Title: "Silence 1h", URL: base + "/alert-mutes/add?__event_id=" + id})
	}
	if event.Cate == "prometheus" {
		actions = append(actions, teamsAction{Type: "Action.OpenUrl", Title: "View graph", URL: base + "/metric/explorer?__event_id=" + id + "&mode=graph"})
	}
	return actions
}
