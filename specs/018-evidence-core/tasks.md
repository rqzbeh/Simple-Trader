# Tasks: spec-018

- [X] T state.go: extend StateObject/BuildState — headlines, catalysts, sentiment (only if present), buckets (supertrend/trend/rsi_zone/volume_spike/confluence_band/regime/natr), price/change/session, hydrated gates, trading_policy; all omitempty
- [X] T ai/types.go: DecisionRequest carries Gates map; signal_handlers fills pre-AI gate outcomes + verifies sentiment/catalysts already flow
- [X] T signals.go: EntryQuestions → direction (Choice LONG/SHORT structured rubrics, backtick refs, no numeric clauses) + edge (Noul criteria) ; drop entry mega-question
- [X] T signals.go: judgeEntryCore reads direction+edge, ValidateChoice{LONG,SHORT}, noul validation, composition FR-503, composed confidence
- [X] T signals.go: trust argmax FR-504 (escalated vs escalated_jev_kept), [DECISION] logs both confidences
- [X] T bucket helpers (rsi_zone/trend/confluence_band/volume_spike) as pure funcs + unit tests
- [X] T state tests: real headline text/supertrend/gates present in marshalled JSON; absent fields omitted (no fabrication)
- [X] T composition tests: edge>=0.5 → BUY/SELL, edge<0.5 → NO_TRADE; conf = min / 1-edge; malformed → schema error
- [X] T trust tests: lower-esc keeps Jev (escalated_jev_kept), higher-esc wins, tie keeps Jev
- [X] T update mock Jev server: answer direction+edge docs-shaped; fix all affected tests
- [X] T shadow primary id already 'direction' — verify + shadow test update
- [X] T gates G28-G31 added; suite + -race + docker build green
- [ ] T013 CI matrix green; secret scan; push
- [ ] T014 deploy VPS (pull-loop); verify [DECISION] lines, no FATAL, health
- [ ] T015 converge: evidence appended, gates reverify ALL MET, report with live evidence
