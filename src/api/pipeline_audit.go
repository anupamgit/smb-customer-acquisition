package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"advisor/scenario"
)

type pipelineAuditor struct {
	enabled bool
	runID   string
	b       strings.Builder
}

func newPipelineAuditor(cfg config, runID string) *pipelineAuditor {
	return &pipelineAuditor{enabled: cfg.pipelineAuditEnabled, runID: runID}
}

func (a *pipelineAuditor) section(title string, lines ...string) {
	if a == nil || !a.enabled {
		return
	}
	writeSection(&a.b, title, lines...)
}

func (a *pipelineAuditor) writeRun(run *Run, businessType string, scenarioName string) {
	if a == nil || !a.enabled || run == nil {
		return
	}
	var out strings.Builder
	out.WriteString("# Pipeline audit\n\n")
	writeSection(&out, "Run context",
		fmt.Sprintf("Run ID: `%s`", run.ID),
		fmt.Sprintf("Status: %s", run.Status),
		fmt.Sprintf("Business type: %s", emptyDash(businessType)),
		fmt.Sprintf("Scenario: %s", emptyDash(scenarioName)),
		fmt.Sprintf("Written: %s", time.Now().UTC().Format(time.RFC3339)),
	)
	if run.Error != "" {
		writeSection(&out, "Failure", run.Error)
	}
	out.WriteString(a.b.String())
	dir, err := pipelineAuditDir()
	if err != nil {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, run.ID+".md")
	_ = os.WriteFile(path, []byte(out.String()), 0o644)
}

func writeSection(b *strings.Builder, title string, lines ...string) {
	b.WriteString("\n## ")
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func pipelineAuditDir() (string, error) {
	if dir, ok := scenario.FindDataDir(""); ok {
		return filepath.Join(filepath.Dir(dir), "outputs", "audit"), nil
	}
	return filepath.Join("outputs", "audit"), nil
}

func scenarioNameForConnector(c Connector) string {
	if sc, ok := c.(*scenarioConnector); ok {
		return sc.name
	}
	sn, ok := c.(interface{ ScenarioName() string })
	if !ok {
		return ""
	}
	return sn.ScenarioName()
}

func businessTypeForAudit(c Connector, cfg config) string {
	return businessTypeForRun(c, cfg)
}
