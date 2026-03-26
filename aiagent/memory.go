package aiagent

import (
	"fmt"
	"strings"
	"time"
)

// initWorkingMemory 初始化工作记忆
func (a *Agent) initWorkingMemory() *WorkingMemory {
	if a.cfg.Memory == nil || !a.cfg.Memory.Enabled {
		return nil
	}
	return &WorkingMemory{
		KeyFindings:      make([]KeyFinding, 0),
		TestedHypotheses: make([]Hypothesis, 0),
		Evidence:         make([]Evidence, 0),
	}
}

// appendMemoryInstructions 在系统提示词中添加工作记忆说明
func (a *Agent) appendMemoryInstructions(systemPrompt string) string {
	memoryInstructions := `
## Working Memory

As you investigate, I will help you track key findings. After each tool observation, you will see a "Working Memory Summary" section that contains:

1. **Key Findings**: Important discoveries from tool results (metrics anomalies, error patterns, etc.)
2. **Hypotheses**: Your working theories about the root cause and their status (testing/confirmed/rejected)
3. **Evidence**: Supporting data you've collected

Use this working memory to:
- Avoid re-querying information you've already obtained
- Build upon previous findings
- Track which hypotheses have been tested
- Remember important values and patterns

When you identify important information in an observation, it will be automatically added to your working memory for future reference.
`
	return systemPrompt + memoryInstructions
}

// updateWorkingMemory 从 ReAct 步骤中提取关键信息并更新工作记忆
func (a *Agent) updateWorkingMemory(memory *WorkingMemory, step ReActStep) {
	if memory == nil {
		return
	}

	now := time.Now().Unix()

	// 1. 从 Thought 中提取假设
	hypothesis := a.extractHypothesis(step.Thought)
	if hypothesis != nil {
		a.addHypothesis(memory, hypothesis)
	}

	// 2. 从 Observation 中提取关键发现和证据
	if step.Observation != "" && !strings.HasPrefix(step.Observation, "Error:") {
		findings := a.extractKeyFindings(step.Action, step.Observation)
		for _, finding := range findings {
			finding.Timestamp = now
			a.addKeyFinding(memory, &finding)
		}

		evidence := a.extractEvidence(step.Action, step.Observation)
		for _, ev := range evidence {
			a.addEvidence(memory, &ev)
		}
	}
}

func (a *Agent) extractHypothesis(thought string) *Hypothesis {
	thoughtLower := strings.ToLower(thought)

	hypothesisKeywords := []string{
		"i suspect", "might be", "could be", "possibly", "hypothesis",
		"i think", "it seems", "appears to be", "likely", "probably",
		"可能是", "怀疑", "猜测", "似乎", "看起来",
	}

	for _, keyword := range hypothesisKeywords {
		if strings.Contains(thoughtLower, keyword) {
			return &Hypothesis{
				Description: thought,
				Status:      "testing",
			}
		}
	}

	return nil
}

func (a *Agent) extractKeyFindings(toolName, observation string) []KeyFinding {
	var findings []KeyFinding

	obsPreview := observation
	if len(obsPreview) > 2000 {
		obsPreview = obsPreview[:2000]
	}

	obsLower := strings.ToLower(obsPreview)

	anomalyPatterns := map[string]string{
		"high": "high", "low": "low", "error": "high", "exception": "high",
		"failed": "high", "timeout": "high", "spike": "high", "100%": "high",
		"0%": "medium", "critical": "high", "warning": "medium",
		"异常": "high", "错误": "high", "失败": "high", "超时": "high",
	}

	for pattern, relevance := range anomalyPatterns {
		if strings.Contains(obsLower, pattern) {
			idx := strings.Index(obsLower, pattern)
			start := idx - 100
			if start < 0 {
				start = 0
			}
			end := idx + len(pattern) + 100
			if end > len(obsPreview) {
				end = len(obsPreview)
			}

			context := strings.TrimSpace(obsPreview[start:end])
			if context != "" {
				findings = append(findings, KeyFinding{
					Content:   context,
					Source:    toolName,
					Relevance: relevance,
				})
			}
			break
		}
	}

	if len(findings) == 0 && len(observation) < 500 && len(observation) > 10 {
		findings = append(findings, KeyFinding{
			Content:   observation,
			Source:    toolName,
			Relevance: "low",
		})
	}

	return findings
}

func (a *Agent) extractEvidence(toolName, observation string) []Evidence {
	var evidenceList []Evidence

	evidenceType := "other"
	toolNameLower := strings.ToLower(toolName)

	if strings.Contains(toolNameLower, "metric") || strings.Contains(toolNameLower, "prometheus") {
		evidenceType = "metric"
	} else if strings.Contains(toolNameLower, "log") || strings.Contains(toolNameLower, "loki") {
		evidenceType = "log"
	} else if strings.Contains(toolNameLower, "trace") || strings.Contains(toolNameLower, "jaeger") {
		evidenceType = "trace"
	} else if strings.Contains(toolNameLower, "config") || strings.Contains(toolNameLower, "cmdb") {
		evidenceType = "config"
	}

	content := observation
	if len(content) > 1000 {
		content = content[:1000] + "... (truncated)"
	}

	if content != "" && !strings.HasPrefix(content, "Error:") {
		evidenceList = append(evidenceList, Evidence{
			Type:    evidenceType,
			Content: content,
			Source:  toolName,
		})
	}

	return evidenceList
}

func (a *Agent) addKeyFinding(memory *WorkingMemory, finding *KeyFinding) {
	for _, existing := range memory.KeyFindings {
		if existing.Content == finding.Content {
			return
		}
	}

	maxFindings := a.cfg.Memory.MaxFindings
	if maxFindings <= 0 {
		maxFindings = DefaultMaxFindings
	}

	if len(memory.KeyFindings) >= maxFindings {
		for i, existing := range memory.KeyFindings {
			if existing.Relevance == "low" {
				memory.KeyFindings = append(memory.KeyFindings[:i], memory.KeyFindings[i+1:]...)
				break
			}
		}
		if len(memory.KeyFindings) >= maxFindings {
			memory.KeyFindings = memory.KeyFindings[1:]
		}
	}

	memory.KeyFindings = append(memory.KeyFindings, *finding)
}

func (a *Agent) addHypothesis(memory *WorkingMemory, hypothesis *Hypothesis) {
	maxHypotheses := a.cfg.Memory.MaxHypotheses
	if maxHypotheses <= 0 {
		maxHypotheses = DefaultMaxHypotheses
	}

	if len(memory.TestedHypotheses) >= maxHypotheses {
		for i, existing := range memory.TestedHypotheses {
			if existing.Status == "rejected" {
				memory.TestedHypotheses = append(memory.TestedHypotheses[:i], memory.TestedHypotheses[i+1:]...)
				break
			}
		}
		if len(memory.TestedHypotheses) >= maxHypotheses {
			memory.TestedHypotheses = memory.TestedHypotheses[1:]
		}
	}

	memory.TestedHypotheses = append(memory.TestedHypotheses, *hypothesis)
}

func (a *Agent) addEvidence(memory *WorkingMemory, evidence *Evidence) {
	maxEvidence := a.cfg.Memory.MaxEvidence
	if maxEvidence <= 0 {
		maxEvidence = DefaultMaxEvidence
	}

	if len(memory.Evidence) >= maxEvidence {
		memory.Evidence = memory.Evidence[1:]
	}

	memory.Evidence = append(memory.Evidence, *evidence)
}

func (a *Agent) formatWorkingMemorySummary(memory *WorkingMemory) string {
	if memory == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Working Memory Summary\n\n")

	if len(memory.KeyFindings) > 0 {
		sb.WriteString("### Key Findings\n")
		for i, finding := range memory.KeyFindings {
			relevanceIcon := "📋"
			if finding.Relevance == "high" {
				relevanceIcon = "🔴"
			} else if finding.Relevance == "medium" {
				relevanceIcon = "🟡"
			}
			content := finding.Content
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("%d. %s [%s] %s\n", i+1, relevanceIcon, finding.Source, content))
		}
		sb.WriteString("\n")
	}

	if len(memory.TestedHypotheses) > 0 {
		sb.WriteString("### Hypotheses\n")
		for i, hyp := range memory.TestedHypotheses {
			statusIcon := "🔍"
			if hyp.Status == "confirmed" {
				statusIcon = "✅"
			} else if hyp.Status == "rejected" {
				statusIcon = "❌"
			}
			desc := hyp.Description
			if len(desc) > 150 {
				desc = desc[:150] + "..."
			}
			sb.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, statusIcon, desc))
		}
		sb.WriteString("\n")
	}

	if len(memory.Evidence) > 0 {
		sb.WriteString(fmt.Sprintf("### Evidence Collected: %d items\n", len(memory.Evidence)))
		typeCounts := make(map[string]int)
		for _, ev := range memory.Evidence {
			typeCounts[ev.Type]++
		}
		for evType, count := range typeCounts {
			sb.WriteString(fmt.Sprintf("- %s: %d\n", evType, count))
		}
	}

	return sb.String()
}
