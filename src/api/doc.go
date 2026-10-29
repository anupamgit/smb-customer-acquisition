// Package main is the Application Service for Customer Acquisition Advisor
// (CAA).
//
// Component mapping (Step 3 architecture, preserved as the contract):
//   - config.go: Configuration and Secrets (server-side only)
//   - square.go: SMB Connector Client (Square alone, Step 4)
//   - normalize.go: Canonical Normalizer
//   - score.go: Customer Value Scorer (deterministic only)
//   - signals.go: Qloo Signal Builder
//   - qloo.go: Qloo Client
//   - llm.go + agent.go: LLM Client and Agent Reasoning
//   - rank.go: Opportunity Ranker (deterministic only)
//   - explain.go: Evidence and Explanation Builder
//   - store.go: Run Store (PostgreSQL in deployment; in-memory only when
//     DATABASE_URL is unset, for local use and tests)
//   - orchestrator.go: Analysis Orchestrator
//   - feedback.go: Feedback Recorder (optional, spec section 9)
//   - main.go: HTTP wiring for the Web Application interface
//
// No credential value is ever logged or sent to the browser. No customer
// PII is ever sent to Qloo or the LLM service: both receive derived,
// non-PII signals only.
package main
