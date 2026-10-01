# Tasks: spec-022

- [X] T001 frontend getHealthStatus: STANDBY/IDLE/DEGRADED/HEALTHY matrix (unit test)
- [X] T002 backend warmup probe for gateway + Jev on boot (async, bounded)
- [X] T003 signal path: store-nil/insert-fail = explicit error before any Telegram dispatch (regression test: no telegram when insert fails)
- [X] T004 gates G42-G45 appended (pre-code, unlazy)
- [ ] T005 suite + race + docker build + compose full boot
- [ ] T006 Playwright: System Stats cards non-DOWN on cold boot (both engines)
- [ ] T007 CI → deploy VPS → live verify (stats truth, DB-connected, no false DOWN)
- [ ] T008 converge + gates reverify ALL MET
