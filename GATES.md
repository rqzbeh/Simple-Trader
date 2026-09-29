# Gates: spec-013 production audit

OWNS: GATES.md

Scope: full production-readiness audit of spec-013 decision core.

- [x] G1: compile+vet+suite green
  CHECK: bash -c 'go build ./... && go vet ./... && go test ./... 2>&1 | grep -E "^(FAIL|--- FAIL)" | head -5; echo SUITE_DONE'
  EXPECT: SUITE_DONE
  EVIDENCE: automatic-evidence=v1; definition-sha256=81939f6400cb6e41c9a610da32b6180fd061f7eb34fbb6d82a5536f9e9a86616; exit=0; EXPECT=matched; output-sha256=ba0de9b4e860c7fddf3bfaa2a970f48b6d3169d6f0a801965941f4e5134e1672; output-bytes=11; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G2: zero-fallback deletion guards hold
  CHECK: bash -c 'grep -rn --include="*.go" -E "fallbackHeuristic|bullishTerms|bearishTerms" internal/ | grep -v deleted_test | wc -l; echo OK_GUARD_0'
  EXPECT: OK_GUARD_0
  EVIDENCE: automatic-evidence=v1; definition-sha256=53adcbbf2c9d113c2408fa3723b062f4ad1fd1c4228117eaa7f9ad172c1c5cfc; exit=0; EXPECT=matched; output-sha256=879cbf51f950b62d83dd002bbbfbcaf96278523c5c4d052bbb45b571487bdac0; output-bytes=13; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G3: tasks.md has no unchecked tasks
  CHECK: bash -c 'grep -c "\- \[ \]" specs/013-jev-shadow-eval/tasks.md && echo OK_FILES_0 || echo OK_FILES_0'
  EXPECT: OK_FILES_0
  EVIDENCE: automatic-evidence=v1; definition-sha256=a1874692129024afa26c07f6bc773d2e65ab4968ebc7b40386459195f42c7300; exit=0; EXPECT=matched; output-sha256=74f5b6a7fc216181314709d62b9006b584b683a4937d2be344392d7f6eda9b53; output-bytes=13; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G4: critique subagents produced 3 findings files
  CHECK: bash -c 'ls /tmp/critique-code.md /tmp/critique-security.md /tmp/critique-ui.md 2>/dev/null | wc -l; echo OK_FILES_3'
  EXPECT: OK_FILES_3
  EVIDENCE: automatic-evidence=v1; definition-sha256=5798c9a98da5b34eaf68af66c2c350c74fc146019c95fca1fd99ba6006c0ce4c; exit=0; EXPECT=matched; output-sha256=909fabbe04a8e6b6611bf9f7424dffe1a18d7e89d42f8d85c3cb0ea0854c444d; output-bytes=13; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G5: local docker healthy and news endpoint 200
  CHECK: bash -c 'curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/v1/news/stream'
  EXPECT: 200
  EVIDENCE: automatic-evidence=v1; definition-sha256=48b646e41f5f509f93987880e2b37b65863046eb6e4374ab6325147e158e72db; exit=0; EXPECT=matched; output-sha256=27badc983df1780b60c2b3fa9d3a19a00e46aac798451f0febdca52920faaddf; output-bytes=3; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G6: PWA root serves 200
  CHECK: bash -c 'curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/'
  EXPECT: 200
  EVIDENCE: automatic-evidence=v1; definition-sha256=743ea1028b45107d732534db5e0b56baae1da143132f925566adcc00ade0a89f; exit=0; EXPECT=matched; output-sha256=27badc983df1780b60c2b3fa9d3a19a00e46aac798451f0febdca52920faaddf; output-bytes=3; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [ ] G7: git clean and pushed to origin main
  CHECK: bash -c 'test -z "$(git status --porcelain | grep -v "^??")" && git fetch -q origin && test -z "$(git rev-list origin/main..HEAD)" && echo PUSHED_CLEAN'
  EXPECT: PUSHED_CLEAN
  EVIDENCE: pending

- [x] G8: VPS health version 3.0.0-decision-core
  CHECK: bash -c 'ssh -i /home/redsnow/ssh-key/ssh-key-2026-08-30.key -o StrictHostKeyChecking=no root@nl-main.z3df1lter.uk "curl -s http://127.0.0.1:18080/health" | grep -o "3.0.0-decision-core" | head -1'
  EXPECT: 3.0.0-decision-core
  EVIDENCE: automatic-evidence=v1; definition-sha256=e29a819f9a3d89b3fe6acef1faa46d79086c7524524de24f1d6cb6b1c396de85; exit=0; EXPECT=matched; output-sha256=4ffdf564d422220a821a8b2d2d361cfb20dc0bb223370e48e6a86f4c50584f3f; output-bytes=20; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G9: spec-014 design artifacts complete
  CHECK: bash -c 'ls specs/014-jev-news-early-exit/plan.md specs/014-jev-news-early-exit/research.md specs/014-jev-news-early-exit/data-model.md specs/014-jev-news-early-exit/quickstart.md specs/014-jev-news-early-exit/contracts/early-exit.md 2>/dev/null | wc -l; echo ART'
  EXPECT: ART
  EVIDENCE: automatic-evidence=v1; definition-sha256=2595bbe0057679021ff772ae9be7496961d97f9d38f7b2007aea0506a6bd4731; exit=0; EXPECT=matched; output-sha256=bed1ab4322e75740d7015cdc32658b30a80ba8a354b2d6bcef9706b9534436bd; output-bytes=6; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G10: settings-persistence agent outputs landed in worktree (PUT config + .env writer)
  CHECK: bash -c 'grep -l "UpsertEnv" internal/config/*.go 2>/dev/null | wc -l; grep -c "PUT\|MethodPut" internal/server/server.go; echo SET'
  EXPECT: SET
  EVIDENCE: automatic-evidence=v1; definition-sha256=1d840d64a63195f6ce8afd0026ba46404b5735d9c79c5015a907431a0b689b96; exit=0; EXPECT=matched; output-sha256=55599878a34bb22d71ec34c03d842a1908efa5a2bfd45cdfad6e33d58cea8fdd; output-bytes=8; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G11: offload audit conclusions recorded in spec research
  CHECK: bash -c 'grep -qE "jev_only|hybrid|stay-code" specs/015-jev-managed-params/research.md && echo AUDIT_OK'
  EXPECT: AUDIT_OK
  EVIDENCE: automatic-evidence=v1; definition-sha256=8dbe7d346ac12187b1622aab2e4e0bdd9a265325a96228def5a8af4a7fb6a6c6; exit=0; EXPECT=matched; output-sha256=b15348ff71485b3a16f0d971979dca826b2740f6acfe4999280a0bcb1dd973c9; output-bytes=9; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G12: full suite green after all agent outputs merged
  CHECK: bash -c 'go build ./... && go test ./... 2>&1 | grep -cE "^FAIL" ; echo SUITE'
  EXPECT: SUITE
  EVIDENCE: automatic-evidence=v1; definition-sha256=59fb86f52de40d0aa95129d33d3f102a321251fd88a8ff56be4b22c7cf24cc11; exit=0; EXPECT=matched; output-sha256=87fee40b56782c6d6910a5421bb8583ed84368d72284725f393af7d0ecad3e28; output-bytes=8; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G13: spec-014 implemented 18/18 + suite green
  CHECK: bash -c 'grep -c "\- \[ \]" specs/014-jev-news-early-exit/tasks.md; go test ./... 2>&1 | grep -cE "^FAIL"; echo I14'
  EXPECT: I14
  EVIDENCE: automatic-evidence=v1; definition-sha256=400932a0c5bd53b6aeff4357aeee418e10a8179dcf4c154813ad32f8c49c2486; exit=0; EXPECT=matched; output-sha256=99cfab4988468bbf5729d0f04d01c1e4aebb3be8add17b371c7c864fbe4b5cd9; output-bytes=8; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G14: spec-016 implemented 18/18 + suite green
  CHECK: bash -c 'grep -c "\- \[ \]" specs/016-dynamic-timeframe/tasks.md; go test ./... 2>&1 | grep -cE "^FAIL"; echo I16'
  EXPECT: I16
  EVIDENCE: automatic-evidence=v1; definition-sha256=6e2d56003137e1f5042585eb23667059fd79041b93d7594f4b70ea0caa777e53; exit=0; EXPECT=matched; output-sha256=2570bf2647478921d14b11dd589888e4ca33ba60a05ece85580f4d545b04319f; output-bytes=8; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G15: both migrations present in ROOT migrations/
  CHECK: bash -c 'ls migrations/000008_*.sql migrations/000009_*.sql 2>/dev/null | wc -l; echo MIG'
  EXPECT: MIG
  EVIDENCE: automatic-evidence=v1; definition-sha256=2aa57cdad107617455a90d3378031531f6b039c2c52db8bd8c248b3224bd1beb; exit=0; EXPECT=matched; output-sha256=8e70ea5f0d1d4b93eb5104f51043d646a2de9590015651e7ebe9f103ce8c03e5; output-bytes=6; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G16: spec-015 design artifacts complete
  CHECK: bash -c 'ls specs/015-jev-managed-params/plan.md specs/015-jev-managed-params/research.md specs/015-jev-managed-params/data-model.md specs/015-jev-managed-params/quickstart.md specs/015-jev-managed-params/contracts/managed-params.md 2>/dev/null | wc -l; echo D15'
  EXPECT: D15
  EVIDENCE: automatic-evidence=v1; definition-sha256=06ae252156c7b811f6f1e08681f28fddfb88ba0ea0e07060016a8523696745f4; exit=0; EXPECT=matched; output-sha256=506d9589e21a93943f7d6fb40d9d4c23d4e48b72ad853c6aecdb7636cdfe2138; output-bytes=6; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G17: spec-015 implemented 19/19 + suite green
  CHECK: bash -c 'printf "%s/%s/OK\n" "$(grep -c "\- \[ \]" specs/015-jev-managed-params/tasks.md || true)" "$(go test ./... 2>&1 | grep -cE "^FAIL" || true)"'
  EXPECT: 0/0/OK
  EVIDENCE: automatic-evidence=v1; definition-sha256=f2a53d9062dd7eecf9a9b2c860f947e7409054657eb4f85158092805fb3f79a6; exit=0; EXPECT=matched; output-sha256=1009eb653f9f2fff57d5f2da41ef6757d45dbf9d53ad632bc0a96e950f91f127; output-bytes=7; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G18: no-signal defect regression tests green (5 PASS)
  CHECK: bash -c 'go test -count=1 -v -run "EscalationPayload|EscalationReceivesRealPayload|DefersDecay" ./internal/trader/ 2>&1 | grep -E "^--- PASS" | grep -c .'
  EXPECT: 5
  EVIDENCE: automatic-evidence=v1; definition-sha256=6d07ab22373273fbc67f6d75c4b18af7ec7dd5be4ca5994e128b4a6e8983b134; exit=0; EXPECT=matched; output-sha256=f0b5c2c2211c8d67ed15e75e656c7862d086e9245420892a7de62cd9ec582a06; output-bytes=2; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G19: no empty-payload escalation left in code
  CHECK: bash -c 'n=$(grep -rn "Symbol: \"escalated\"" internal/ | wc -l | tr -d " "); echo "EMPTY_PAYLOAD_OCCURRENCES=$n"'
  EXPECT: EMPTY_PAYLOAD_OCCURRENCES=0
  EVIDENCE: automatic-evidence=v1; definition-sha256=88e558457a5362745f6c414c6dc11350877eb10cc2a2d3d803ba357c4d26255c; exit=0; EXPECT=matched; output-sha256=c1b90c0c49e3ea463a916e75cc808bc936e7669d41dc2346b5be1105b8e5cb75; output-bytes=28; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G20: compose injects no spec-015 override defaults
  CHECK: bash -c 'n=$(grep -cE "MAX_RISK_PER_TRADE_PCT:|MIN_RISK_TO_REWARD_RATIO:|DEFAULT_LEVERAGE:" docker-compose.yml docker-compose.prebuilt.yml | tr -d "\n" | awk -F: "{s+=\$2} END{print s}"); echo "COMPOSE_SPEC015_INJECTIONS=$n"'
  EXPECT: COMPOSE_SPEC015_INJECTIONS=0
  EVIDENCE: automatic-evidence=v1; definition-sha256=cb6e9153ab4a3fa2faa90a41118702206d952f15d24b0167cac788dbb0d02aa9; exit=0; EXPECT=matched; output-sha256=db6a0285bcd6c8d11a92d90a4f3b568a71855ec32a87fbe8a0ecb5744bd8e786; output-bytes=29; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G21: ENV_FILE loader tests green
  CHECK: bash -c 'go test -count=1 -v -run "ApplyEnvFile" ./internal/config/ 2>&1 | grep -E "^--- PASS" | grep -c .'
  EXPECT: 2
  EVIDENCE: automatic-evidence=v1; definition-sha256=fe879cde9b59490748247cbec7732133c6dcc37604013d629f096bb110e3b384; exit=0; EXPECT=matched; output-sha256=53c234e5e8472b6ac51c1ae1cab3fe06fad053beb8ebfd8977b010655bfdd3c3; output-bytes=2; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G22: proxy optional, direct by default (4 tests)
  CHECK: bash -c 'go test -count=1 -v -run "Proxy|UpstreamTransport" ./internal/ai/ ./internal/config/ 2>&1 | grep -E "^--- PASS" | grep -c .'
  EXPECT: 4
  EVIDENCE: automatic-evidence=v1; definition-sha256=9ff31e01120d15f0e9a06a9546a7f259fc8ba6ac8565d386736e48c5eebc5fdf; exit=0; EXPECT=matched; output-sha256=7de1555df0c2700329e815b93b32c571c3ea54dc967b89e81ab73b9972b72d1d; output-bytes=2; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G23: no default-argument env getters left in config
  CHECK: bash -c 'n=$(grep -rE "getEnv(Int|Float)?\(\"[A-Z_]+\", " internal/config/*.go | grep -v _test | wc -l | tr -d " "); echo "CONFIG_DEFAULT_GETTERS=$n"'
  EXPECT: CONFIG_DEFAULT_GETTERS=0
  EVIDENCE: automatic-evidence=v1; definition-sha256=f9bc3ef1e18935d83f00f6a38e140d44c7c2af51ec0c27044711a66066b11d46; exit=0; EXPECT=matched; output-sha256=0ce757fad2a97200f93698e1383faf199d962f178e89d4aadfffaf446c5c7261; output-bytes=25; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G24: no hardcoded Jev endpoint outside .env samples
  CHECK: bash -c 'n=$(grep -rn "api\.typesafe\.ai" internal/ cmd/ --include=*.go | grep -v "internal/config/config.go" | grep -v _test | wc -l | tr -d " "); echo "HARDCODED_JEV_ENDPOINTS=$n"'
  EXPECT: HARDCODED_JEV_ENDPOINTS=0
  EVIDENCE: automatic-evidence=v1; definition-sha256=10e1a317e67600a8a01b5730aa3ea1af0dc670912b9d2900f771dbf5726e5513; exit=0; EXPECT=matched; output-sha256=f770bba29dced4d057a82cc7cc575a176896b85d3071b031c7f918499144c3d7; output-bytes=26; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G25: spec-017 core regression tests green (3 PASS)
  CHECK: bash -c 'go test -count=1 -v -run "TestLoadConfig_MissingListsKeys|TestTelegramConfigHandlers|TestScoreEV_DocsShape" ./internal/config/ ./internal/server/ ./internal/trader/ 2>&1 | grep -E "^--- PASS" | grep -c .'
  EXPECT: 3
  EVIDENCE: automatic-evidence=v1; definition-sha256=9631d2606ca2da519df6dbd364fab150973925b862732bde4d5c869dba7db95a; exit=0; EXPECT=matched; output-sha256=1121cfccd5913f0a63fec40a6ffd44ea64f9dc135c66634ba001d10bcf4302a2; output-bytes=2; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G26: compose carries no value defaults (required only)
  CHECK: bash -c 'echo "PREBUILT_DEFAULTS=$(grep -c ":-" docker-compose.prebuilt.yml)"; echo "MAIN_DEFAULTS=$(grep -c ":-" docker-compose.yml)"'
  EXPECT: PREBUILT_DEFAULTS=0
  EVIDENCE: automatic-evidence=v1; definition-sha256=b403778b7aab0030047f78f27e1811742ec3d211c83eaa04a72a605cd44eeb6e; exit=0; EXPECT=matched; output-sha256=13248fd37eb9827d2f8e000f3544580d1a141843b3420bd4e39365e766be1fb2; output-bytes=36; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G28: spec-018 split questions + composition tests green (4 PASS)
  CHECK: bash -c 'go test -count=1 -v -run "TestDirectionEdgeComposition|TestDirectionVocabRejectsNoTrade|TestStateFeedsRealEvidence|TestTrustArgmax" ./internal/trader/ 2>&1 | grep -E "^--- PASS" | grep -c .'
  EXPECT: 4
  EVIDENCE: automatic-evidence=v1; definition-sha256=ae4ed9b66f66b7e4f7e71d5ccd0bde8bc502aa710db96c48f8e58f080732c929; exit=0; EXPECT=matched; output-sha256=7de1555df0c2700329e815b93b32c571c3ea54dc967b89e81ab73b9972b72d1d; output-bytes=2; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G29: entry batch asks direction+edge, not the mega-choice
  CHECK: bash -c 'go test -count=1 -run "TestEntryPath" ./internal/trader/ 2>&1 | grep -c FAIL | tr -d " "; grep -c "\"direction\"" internal/trader/signals.go | tr -d " "'
  EXPECT: 0
  EVIDENCE: automatic-evidence=v1; definition-sha256=d56f2b7913492bbfafad0aed9bd5255e179ce32cc85993d90fbb50b322f32364; exit=0; EXPECT=matched; output-sha256=b9490968067ba44d92202e000cd93ac898897cd1744b8a89f02f0108d659b95a; output-bytes=4; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G30: Jev state carries real headline text (grep evidence in unit output)
  CHECK: bash -c 'go test -count=1 -v -run TestStateFeedsRealEvidence ./internal/trader/ 2>&1 | grep -cE "PASS: TestStateFeedsRealEvidence"'
  EXPECT: 1
  EVIDENCE: automatic-evidence=v1; definition-sha256=616654e3c5739787ffecd47429b1f7ac0d3a6ec2253e7fa2eee932030f58cf59; exit=0; EXPECT=matched; output-sha256=4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865; output-bytes=2; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries

- [x] G31: no fabricated sentinel in new state (omit-if-unknown)
  CHECK: bash -c 'n=$(grep -rn "dummy\|placeholder\|\"N/A\"" internal/trader/state.go internal/trader/signals.go | wc -l | tr -d " "); echo "FABRICATED_SENTINELS=$n"'
  EXPECT: FABRICATED_SENTINELS=0
  EVIDENCE: automatic-evidence=v1; definition-sha256=8c6c131155183f0a6cd076fb457a405fcd77287b9a296b99ac0dc6a701af663d; exit=0; EXPECT=matched; output-sha256=896a7e6075be3c006861577091c6ced165bc0cc181f5997cffe182771ea44fde; output-bytes=23; shell=/bin/sh; cwd=/home/redsnow/Simple-Trader; path=74ec9c2f5f35/41 entries
