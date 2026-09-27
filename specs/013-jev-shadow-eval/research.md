# Research: Classifier Prompt and Question Design (Jev & LLM)

**Feature**: Spec-013 (Jev Decision Model Shadow Evaluation)  
**Target Path**: `/home/redsnow/Simple-Trader/specs/013-jev-shadow-eval/research.md`  
**Date**: 2026-09-27  
**Scope**: High-accuracy question/prompt design techniques for TypeSafe Jev (System One) and OpenAI-compatible Chat LLMs evaluating identical futures trading tasks.

---

## 1. Jev Question Design Rules

TypeSafe Jev is a non-autoregressive decision model. It produces calibrated probability distributions and discrete choices without autoregressive text generation. The rules below govern Jev question and state construction.

### 1.1 State Structuring Rules
- **Use Structured JSON Objects for Multi-Part State**: For application state, structured JSON objects with explicit key names provide clear semantic boundaries. Strings are only appropriate for isolated text passages.  
  *Source*: [https://docs.typesafe.ai/concepts/state.md](https://docs.typesafe.ai/concepts/state.md)
- **Pre-Filter State to Prevent Context Rot**: Model accuracy degrades when state contains irrelevant data. Filter data in code prior to API calls; send only fields necessary for the evaluation.  
  *Source*: [https://docs.typesafe.ai/model-jaggedness/jev-1.13.md#large-state-full-of-irrelevant-detail](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md#large-state-full-of-irrelevant-detail)
- **Pre-Compute Mathematical Logic in Code**: Jev cannot perform arithmetic, counting, or numeric aggregation. Compute technical indicators, price spreads, percentage changes, and volume ratios in application code before injecting values into state.  
  *Source*: [https://docs.typesafe.ai/model-jaggedness/jev-1.13.md#math-and-numbers](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md#math-and-numbers)
- **Pre-Compute Temporal Durations in Code**: Jev treats dates and timestamps as plain text and cannot calculate elapsed time or determine window boundaries. Compute elapsed intervals in code (for example, `age_minutes`, `minutes_to_funding`) and supply these as discrete numbers or semantic buckets.  
  *Source*: [https://docs.typesafe.ai/model-jaggedness/jev-1.13.md#date-and-time-comparison](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md#date-and-time-comparison)
- **Text-Only Input Constraint**: State must contain text, numeric literals, booleans, or structured arrays/objects. Binary data, images, and raw media are not supported.  
  *Source*: [https://docs.typesafe.ai/concepts/state.md](https://docs.typesafe.ai/concepts/state.md)

### 1.2 Instruction Authoring Rules
- **One Snap Judgment per Question**: Design each question for a fast, intuitive decision that an expert can make in one second. Complex multi-step reasoning must be decomposed into separate questions.  
  *Source*: [https://docs.typesafe.ai/primitives.md#ask-for-one-snap-judgment-per-question](https://docs.typesafe.ai/primitives.md#ask-for-one-snap-judgment-per-question)
- **Explicit Field Referencing via Backticks**: Reference specific JSON paths within state using dot-and-index notation enclosed in backticks (for example, `` `market.rsi` `` or `` `news.headlines[0].title` ``). This focuses the model directly on relevant state components.  
  *Source*: [https://docs.typesafe.ai/primitives.md#reference-specific-fields](https://docs.typesafe.ai/primitives.md#reference-specific-fields)
- **Self-Contained Instructions**: Question IDs are internal routing keys and are not transmitted to the model during inference. Instructions must be fully self-contained.  
  *Source*: [https://docs.typesafe.ai/primitives.md#define-a-question](https://docs.typesafe.ai/primitives.md#define-a-question)
- **Positive Polarity on Noul Questions**: Phrase Noul questions such that a value near 1.0 indicates true/affirmative. Avoid inverted phrasing (for example, "Is the price not falling?") to prevent negative bias and downstream logical inversion.  
  *Source*: [https://docs.typesafe.ai/primitives/noul.md#writing-a-noul-question](https://docs.typesafe.ai/primitives/noul.md#writing-a-noul-question)
- **Direct Phrasing Without Indirection**: Avoid double negatives, hypothetical conditionals, or multi-hop logic. Jev reads instructions literally.  
  *Source*: [https://docs.typesafe.ai/model-jaggedness/jev-1.13.md#indirection](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md#indirection)
- **Structured Instruction Objects for Complex Tasks**: When an instruction requires evaluation parameters or candidate comparison, format `instructions` as a JSON object with explicit keys (such as `question`, `focus`, `inspect`).  
  *Source*: [https://docs.typesafe.ai/primitives/advanced.md#structured-instructions](https://docs.typesafe.ai/primitives/advanced.md#structured-instructions)

### 1.3 Criteria and Rubric Construction Rules
- **Differentiate Choice Options with Negative Boundaries**: For Choice questions, define not only what an option covers, but also what it excludes. Using structured JSON criteria with `what`, `not_for`, and `examples` sharpens decision boundaries.  
  *Source*: [https://docs.typesafe.ai/primitives/advanced.md#json-rubric-for-boundary-clarification](https://docs.typesafe.ai/primitives/advanced.md#json-rubric-for-boundary-clarification)
- **Describe Situations, Not Degrees, in Score Levels**: Write concrete descriptions of distinct operational situations for each Score level. Avoid abstract adjectives (such as "moderate") and bare numbers (`"0"`, `"1"`, `"2"`).  
  *Source*: [https://docs.typesafe.ai/primitives/score.md#writing-good-levels](https://docs.typesafe.ai/primitives/score.md#writing-good-levels)
- **Limit Score Questions to One Dimension**: A Score level description must measure a single attribute. If a description combines unrelated factors, the model cannot map inputs consistently and confidence decreases.  
  *Source*: [https://docs.typesafe.ai/primitives/score.md#writing-good-levels](https://docs.typesafe.ai/primitives/score.md#writing-good-levels)
- **Use Between 2 and 10 Distinct Score Levels**: Employ 3 to 5 levels for optimal calibration. Only add levels that have distinct, non-overlapping criteria.  
  *Source*: [https://docs.typesafe.ai/primitives/score.md#writing-good-levels](https://docs.typesafe.ai/primitives/score.md#writing-good-levels)
- **Define Explicit Noul Criteria for Subtle Boundaries**: While Noul criteria are optional, providing explicit `true` and `false` criterion definitions (with positive and negative examples) resolves ambiguity in borderline cases.  
  *Source*: [https://docs.typesafe.ai/primitives/advanced.md#structured-noul-criteria](https://docs.typesafe.ai/primitives/advanced.md#structured-noul-criteria)

### 1.4 Calibration and Confidence Usage Rules
- **Distinguish Probability from Confidence**: `probabilities` is the full probability distribution across choices or score levels. `confidence` is a normalized measure of distribution dispersion (1.0 indicates concentrated probability on one option; values near 0.0 indicate uniform uncertainty). Noul returns a single scalar probability without a separate confidence field.  
  *Source*: [https://docs.typesafe.ai/confidence.md](https://docs.typesafe.ai/confidence.md)
- **Apply Three-Tier Confidence Routing**:
  1. *High Confidence* ($\ge 0.80$): Act automatically.
  2. *Moderate Confidence* ($0.50 \le \text{confidence} < 0.80$): Secondary validation or conservative sizing.
  3. *Low Confidence* ($< 0.50$): Withhold execution or fallback to baseline risk-off behavior.  
  *Source*: [https://docs.typesafe.ai/confidence.md#three-paths-for-using-confidence-in-your-code](https://docs.typesafe.ai/confidence.md#three-paths-for-using-confidence-in-your-code)
- **Scale Confidence Thresholds with Financial Risk**: Set higher confidence floors for destructive or capital-committing actions (order entry) than for read-only evaluations (logging, clustering).  
  *Source*: [https://docs.typesafe.ai/confidence.md#thresholds-scale-with-risk](https://docs.typesafe.ai/confidence.md#thresholds-scale-with-risk)
- **Do Not Assume Structural Invariance Across Primitives**: A Choice question probability does not equal a Noul probability on identical text ($P_{\text{choice}}(\text{yes}) \neq P_{\text{noul}}(\text{yes})$), and $P_{\text{noul}}(\text{true}) \neq 1 - P_{\text{noul}}(\text{false})$. Calibrate thresholds independently for each primitive type.  
  *Source*: [https://docs.typesafe.ai/model-jaggedness/jev-1.13.md#common-sense-structural-invariants](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md#common-sense-structural-invariants)

### 1.5 Batching, Splitting, and Parallel Execution Rules
- **Batch Parallel Questions in a Single Request**: All questions evaluating the same state must be submitted in a single API call. Jev evaluates questions concurrently in one pass, reducing latency by up to 10x and cost by up to 12x compared to sequential calls.  
  *Source*: [https://docs.typesafe.ai/primitives.md#ask-multiple-questions-together](https://docs.typesafe.ai/primitives.md#ask-multiple-questions-together)
- **Implement Speculative Fan-Out**: Query all potentially relevant factors (entry bias, volatility hazard, news impact, severity) in a single request, and let backend business logic decide which fields to consume.  
  *Source*: [https://docs.typesafe.ai/patterns/fan-out.md](https://docs.typesafe.ai/patterns/fan-out.md)
- **Decompose Multi-Factor Judgments (Composite Scoring)**: Split complex trading decisions into distinct atomic questions (trend direction, volume confirmation, volatility risk). Weight and combine the resulting scores in application code.  
  *Source*: [https://docs.typesafe.ai/patterns/composite-scoring.md](https://docs.typesafe.ai/patterns/composite-scoring.md)
- **Reserve Sequential Calls Exclusively for Dependent State**: Only execute sequential requests when the input state of Question B strictly depends on the resolved value of Question A.  
  *Source*: [https://docs.typesafe.ai/primitives.md#when-one-question-depends-on-another](https://docs.typesafe.ai/primitives.md#when-one-question-depends-on-another)

---

## 2. LLM Classifier Prompt Rules

When utilizing an OpenAI-compatible Chat LLM (`/v1/chat/completions`) for classification, autoregressive decoding characteristics determine accuracy and schema reliability.

### 2.1 Constrained Decoding and JSON Schema Ordering
- **Declare Reasoning Fields Before Decision Labels**: In strict JSON schema constrained decoding (Finite State Machine guided decoding), property order determines generation order. An autoregressive transformer conditions its next token only on prior tokens in the prefix. If `label` precedes `reasoning`, the classification decision is sampled before any reasoning occurs, converting the explanation into post-hoc rationalization. Placing `evidence` and `reasoning` before `label` forces the model to attend to its reasoning scratchpad before sampling the decision.  
  *Source*: [https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985](https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985)
- **Enforce Strict Schema Parameters**: Set `strict: true` inside `response_format` of type `json_schema`. Every property declared in `properties` must be listed in `required`, and `additionalProperties: false` must be set on all object nodes.  
  *Source*: [https://developers.openai.com/api/docs/guides/structured-outputs](https://developers.openai.com/api/docs/guides/structured-outputs)
- **Represent Nullable Values Explicitly**: When an output field is optional, use a union schema (`"type": ["string", "null"]`) rather than omitting it from `required`. This avoids placeholder hallucinations while satisfying strict validation.  
  *Source*: [https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985](https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985)
- **Use Descriptive Schema Field Names as Semantic Anchors**: Forced schema keys (such as `negation_check` or `counter_arguments_considered`) are injected into the KV cache by the decoder state machine, providing free prompt guidance immediately prior to value generation.  
  *Source*: [https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985](https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985)
- **Design Enum Values to Differ at the First Token**: Select enum values whose initial tokens are distinct (for example, `LONG`, `SHORT`, `NO_TRADE` or `BULLISH`, `BEARISH`, `NEUTRAL`, `MIXED`). This ensures clear token separation and avoids probability mass sharing on shared prefixes.  
  *Source*: [https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985](https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985)

### 2.2 Rubric-Anchored Few-Shot Prompting
- **Balance Class Representation in Examples**: LLMs exhibit strong recency and frequency biases. Distribute few-shot examples equally across all valid classification classes. Ensure the final example does not consistently represent the same class.  
  *Source*: [https://www.prompthub.us/blog/the-few-shot-prompting-guide](https://www.prompthub.us/blog/the-few-shot-prompting-guide)
- **Anchor Rubrics with Concrete Positive and Negative Boundary Examples**: For every label, provide an explicit definition of matching conditions along with negative cases ("what this label is NOT"). Ground each level with realistic input excerpts.  
  *Source*: [https://towardsdatascience.com/llm-as-a-judge-a-practical-guide/](https://towardsdatascience.com/llm-as-a-judge-a-practical-guide/)
- **Use Native Chat Role Formatting for Examples**: Structure few-shot pairs as alternating `user` and `assistant` messages in the API message list rather than embedding them as a single concatenated prompt string.  
  *Source*: [https://huggingface.co/docs/transformers/main/en/tasks/prompting](https://huggingface.co/docs/transformers/main/en/tasks/prompting)

### 2.3 Handling Financial Slang and Negation
- **Provide an In-Prompt Domain Slang Lexicon**: Explicitly define financial and crypto-specific terminology (for example, "liquidation cascade", "short squeeze", "rug", "funding flip", "bear trap") in the system prompt to prevent general-domain misinterpretations.  
  *Source*: [https://arxiv.org/html/2310.13226](https://arxiv.org/html/2310.13226)
- **Mandate Explicit Negation Extraction**: LLMs frequently misclassify sentences containing grammatical negations (such as "denies rumors of insolvency", "rejection of appeal", "fails to break support"). Enforce a structured schema step where the model must identify and invert negated sentiments before selecting the final label.  
  *Source*: [https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985](https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985)
- **Enforce Position-Intent Vocabulary**: Restrict classification labels strictly to position intent (`LONG`, `SHORT`, `NO_TRADE`) rather than order side (`BUY`, `SELL`). In perpetual futures, "BUY" can represent a long entry or a short exit.  
  *Source*: Simple-Trader Spec-013 Architectural Constraint (`spec.md:17`)

---

## 3. Proposed Question Set (Jev Decision Model)

Below is the concrete `questions` JSON payload for TypeSafe Jev. It covers all required tasks:
1. `entry_direction`: Choice (`LONG`, `SHORT`, `NO_TRADE`)
2. `exit_now`: Noul (binary exit recommendation)
3. `news_impact`: Choice (`BULLISH`, `BEARISH`, `NEUTRAL`, `MIXED`)
4. `severity`: Score (4-level severity rubric)
5. `has_catalyst`: Noul (active event driver verification)

### 3.1 Sample Evaluated State
```json
{
  "symbol": "BTCUSDT",
  "position": {
    "has_open_position": true,
    "side": "LONG",
    "entry_price": 63200.0,
    "current_price": 62450.0,
    "unrealized_pnl_pct": -1.18,
    "holding_duration_minutes": 145,
    "stop_loss_price": 62100.0,
    "take_profit_price": 65500.0
  },
  "market": {
    "timeframe": "15m",
    "rsi_14": 38.2,
    "macd_histogram": -14.2,
    "ema_fast_above_slow": false,
    "volume_surge_ratio": 2.45,
    "orderbook_bid_ask_imbalance": -0.32,
    "funding_rate_annualized_pct": 8.5
  },
  "news_cluster": {
    "cluster_id": "cls-9812",
    "age_minutes": 12,
    "source_count": 4,
    "title": "SEC Rejects Emergency Appeal in Landmark Ripple Ruling",
    "snippet": "Federal court denies the SEC's latest emergency motion to stay the summary judgment. Markets observe support holding across major digital assets with no enforcement actions pending."
  }
}
```

### 3.2 Jev Questions Definition Payload
```json
{
  "model": "jev-latest",
  "questions": {
    "entry_direction": {
      "type": "choice",
      "instructions": {
        "question": "What is the recommended perpetual futures position intent for `symbol` given `market` and `news_cluster`?",
        "focus": "Evaluate directional alignment. Do not recommend an entry if market indicators contradict the news sentiment."
      },
      "criteria": {
        "LONG": {
          "what": "Bullish directional setup where indicators and news indicate upward price momentum",
          "not_for": "Bearish momentum, neutral consolidation, or high-risk conflicting data",
          "examples": ["RSI recovering from oversold with bullish news and volume expansion", "Breakout above resistance confirmed by positive catalyst"]
        },
        "SHORT": {
          "what": "Bearish directional setup where indicators and news indicate downward price momentum",
          "not_for": "Bullish momentum, neutral consolidation, or high-risk conflicting data",
          "examples": ["RSI breaking down below 40 with negative news and heavy volume", "Rejection at key resistance confirmed by regulatory enforcement"]
        },
        "NO_TRADE": {
          "what": "Ambiguous, conflicting, neutral, or highly volatile market conditions where capital should be preserved",
          "not_for": "Clear high-conviction directional trends with corroborating volume",
          "examples": ["Conflicting signals between news and technicals", "Low volume sideways drift", "Extreme spread or uncertain event risk"]
        }
      }
    },
    "exit_now": {
      "type": "noul",
      "instructions": {
        "question": "Should the open `position` in `symbol` be closed immediately based on `market` indicators and `news_cluster`?",
        "inspect": ["position", "market", "news_cluster"],
        "focus": "A true value means immediate market exit. Evaluate whether adverse momentum or catalyst threatens capital."
      },
      "criteria": {
        "true": {
          "what": "Adverse trend development, thesis invalidation, or strong negative catalyst threatening the open position",
          "examples": [
            "Long position held while price breaks below support on expanding selling volume",
            "Breaking negative regulatory or exploit news directly impacting the held asset"
          ]
        },
        "false": {
          "what": "Normal market fluctuations within established risk parameters; original trade thesis remains intact",
          "examples": [
            "Minor consolidation pullback while position remains well above stop loss",
            "Favorable momentum continuing in the direction of the trade"
          ]
        }
      }
    },
    "news_impact": {
      "type": "choice",
      "instructions": {
        "question": "What is the directional market impact of `news_cluster` on `symbol`?",
        "focus": "Classify the fundamental price implication. Account for double negatives and resolution of regulatory actions."
      },
      "criteria": {
        "BULLISH": {
          "what": "Events expected to increase buying demand, clear regulatory hurdles, or drive adoption",
          "not_for": "Routine updates, mixed commentary, or negative legal actions",
          "examples": ["ETF approval", "Dismissal of regulatory charges", "Institutional capital allocation"]
        },
        "BEARISH": {
          "what": "Events expected to cause liquidation, increase regulatory enforcement, or expose security exploits",
          "not_for": "Routine market commentary or dismissed allegations",
          "examples": ["Protocol exploit or hack", "Exchange insolvency rumors confirmed", "Hostile enforcement action"]
        },
        "NEUTRAL": {
          "what": "Informational announcements or routine corporate statements with negligible price impact",
          "not_for": "High-impact legal verdicts or unexpected macroeconomic developments",
          "examples": ["Scheduled maintenance notice", "Minor partnership with non-market entity"]
        },
        "MIXED": {
          "what": "Complex developments containing both strongly favorable and adverse components",
          "not_for": "Decisively positive or negative announcements",
          "examples": ["Bill passes with high tax rate but clear regulatory clarity", "Earnings beat accompanied by reduced forward guidance"]
        }
      }
    },
    "severity": {
      "type": "score",
      "instructions": {
        "question": "How severe is the potential market impact of `news_cluster`?",
        "focus": "Rate the structural magnitude and volatility potential of the event."
      },
      "criteria": [
        {
          "summary": "Routine: Negligible or minor market noise",
          "signals": ["Common social sentiment", "Minor technical update", "Unverified single-source rumor"]
        },
        {
          "summary": "Notable: Moderate volatility event affecting single asset",
          "signals": ["Significant exchange listing", "Ecosystem grant announcement", "Quarterly financial report"]
        },
        {
          "summary": "Significant: Substantial catalyst driving sector-wide price repricing",
          "signals": ["Major regulatory ruling", "Macro rate policy shift", "Top 20 asset exploit or depeg"]
        },
        {
          "summary": "Critical: Systemic market event triggering widespread liquidations",
          "signals": ["Tier 1 custodian or exchange collapse", "Global regulatory ban", "Systemic bridge exploit"]
        }
      ]
    },
    "has_catalyst": {
      "type": "noul",
      "instructions": {
        "question": "Does `news_cluster` describe an active, fresh catalyst that directly drives immediate price action?",
        "inspect": "news_cluster",
        "focus": "Distinguish active breaking developments from stale recap articles or general educational analysis."
      },
      "criteria": {
        "true": {
          "what": "A specific verifiable occurrence that happened recently and directly influences order book demand",
          "examples": ["Court ruling issued 15 minutes ago", "Emergency interest rate decision", "Live security incident"]
        },
        "false": {
          "what": "Generic commentary, retrospective market recaps, or educational articles lacking fresh impetus",
          "examples": ["Weekly crypto market summary", "Beginner guide to proof of stake", "Opinion editorial"]
        }
      }
    }
  }
}
```

---

## 4. Proposed LLM Prompt Pattern

For OpenAI-compatible Chat endpoints (`/v1/chat/completions`), this pattern implements strict structured outputs with causal scratchpad ordering and explicit domain guidance.

### 4.1 System Prompt
```text
You are an expert quantitative crypto futures risk classifier.
You evaluate market state, technical indicators, and news events for perpetual futures contracts.

CRITICAL OPERATIONAL RULES:
1. Futures Position Intent: Output only position intent ("LONG", "SHORT", "NO_TRADE"). Never emit order side ("BUY", "SELL").
2. Domain Slang Interpretation:
   - "Short squeeze": Rapid price spike forcing short liquidations -> Bullish continuation signal.
   - "Bear trap": Brief breakdown below support that immediately reverses upward -> Bullish reversal signal.
   - "Bull trap": Brief breakout above resistance that immediately fails downward -> Bearish reversal signal.
   - "Liquidation cascade": Accelerating momentum driven by forced liquidations in the direction of the break.
   - "God candle": Unusually large single-interval price expansion bar.
   - "Rug" / "Exploit": Unauthorized liquidity drain or contract exploit -> Extreme Bearish signal.
3. Negation Resolution:
   - Carefully resolve grammatical negations and legal terms.
   - Example: "SEC drops lawsuit" contains the negative word "drops", but the market event is BULLISH.
   - Example: "Rejects motion to stay" resolves to positive continuation of prior judgment.
4. Reasoning Order:
   - Always extract verbatim evidence into `evidence_quotes` first.
   - Identify and resolve any slang or negations in `slang_and_negations`.
   - Explicitly articulate counter-arguments in `counter_arguments_considered`.
   - Finally select the categorical `direction` or `impact` and assign a calibrated `confidence`.
```

### 4.2 Few-Shot Examples (Balanced User/Assistant Pairs)

#### Example 1: Bullish Legal Resolution with Negation (LONG)
*User*:
```json
{
  "symbol": "XRPUSDT",
  "position": {"has_open_position": false},
  "market": {"rsi_14": 58.4, "volume_surge_ratio": 3.1, "ema_fast_above_slow": true},
  "news_cluster": {
    "title": "Appeals Court Rejects Regulatory Motion to Overturn Landmark Ruling",
    "snippet": "Judges denied the SEC motion for interlocutory appeal, reaffirming non-security status."
  }
}
```
*Assistant*:
```json
{
  "evidence_quotes": [
    "Appeals Court Rejects Regulatory Motion",
    "denied the SEC motion for interlocutory appeal",
    "volume_surge_ratio: 3.1"
  ],
  "slang_and_negations": [
    "Rejects motion / denied appeal: Double negation confirming the prior ruling stands in favor of the asset. Highly bullish."
  ],
  "counter_arguments_considered": "Considered NO_TRADE due to prior regulatory uncertainty, but high volume surge with legal finality confirms momentum.",
  "entry_direction": "LONG",
  "exit_recommended": false,
  "news_impact": "BULLISH",
  "severity_level": "Significant",
  "has_active_catalyst": true,
  "confidence": 0.94
}
```

#### Example 2: Bearish Trap with Negative Momentum (SHORT)
*User*:
```json
{
  "symbol": "ETHUSDT",
  "position": {"has_open_position": false},
  "market": {"rsi_14": 41.2, "volume_surge_ratio": 2.8, "ema_fast_above_slow": false},
  "news_cluster": {
    "title": "Major Layer-2 Bridge Pauses Transactions Amid Exploit Rumors",
    "snippet": "Validators halted the bridge contract following an anomalous $40M outflow."
  }
}
```
*Assistant*:
```json
{
  "evidence_quotes": [
    "Bridge Pauses Transactions Amid Exploit Rumors",
    "anomalous $40M outflow",
    "rsi_14: 41.2",
    "ema_fast_above_slow: false"
  ],
  "slang_and_negations": [
    "Exploit: Protocol security breach. Bearish catalyst with high contagion risk."
  ],
  "counter_arguments_considered": "Considered whether outflow was planned migration, but bridge pause confirms unexpected security incident.",
  "entry_direction": "SHORT",
  "exit_recommended": false,
  "news_impact": "BEARISH",
  "severity_level": "Significant",
  "has_active_catalyst": true,
  "confidence": 0.91
}
```

#### Example 3: Conflicting Market State (NO_TRADE)
*User*:
```json
{
  "symbol": "BTCUSDT",
  "position": {"has_open_position": false},
  "market": {"rsi_14": 49.5, "volume_surge_ratio": 0.8, "ema_fast_above_slow": true},
  "news_cluster": {
    "title": "Whale Moves 1,000 BTC to Cold Storage, Miners Continue Hedging",
    "snippet": "Mixed on-chain activity observed over the weekend as volume remains below 30-day average."
  }
}
```
*Assistant*:
```json
{
  "evidence_quotes": [
    "Whale Moves 1,000 BTC to Cold Storage",
    "Miners Continue Hedging",
    "volume remains below 30-day average",
    "volume_surge_ratio: 0.8"
  ],
  "slang_and_negations": [
    "Cold storage transfer: Mild accumulation signal.",
    "Hedging: Downside risk protection."
  ],
  "counter_arguments_considered": "Considered LONG due to cold storage accumulation, but below-average volume and miner hedging show lack of market consensus.",
  "entry_direction": "NO_TRADE",
  "exit_recommended": false,
  "news_impact": "NEUTRAL",
  "severity_level": "Routine",
  "has_active_catalyst": false,
  "confidence": 0.86
}
```

### 4.3 JSON Schema Definition (`response_format`)
```json
{
  "type": "json_schema",
  "json_schema": {
    "name": "futures_market_evaluation",
    "strict": true,
    "schema": {
      "type": "object",
      "properties": {
        "evidence_quotes": {
          "type": "array",
          "items": { "type": "string" },
          "description": "Verbatim observed facts, indicator values, or quote snippets from state"
        },
        "slang_and_negations": {
          "type": "array",
          "items": { "type": "string" },
          "description": "Explicit identification and resolved meaning of any crypto slang, traps, or grammatical negations"
        },
        "counter_arguments_considered": {
          "type": "string",
          "description": "Consideration of alternative interpretations and why they were rejected"
        },
        "entry_direction": {
          "type": "string",
          "enum": ["LONG", "SHORT", "NO_TRADE"],
          "description": "Recommended perpetual futures position intent"
        },
        "exit_recommended": {
          "type": "boolean",
          "description": "True if an open position should be closed immediately"
        },
        "news_impact": {
          "type": "string",
          "enum": ["BULLISH", "BEARISH", "NEUTRAL", "MIXED"],
          "description": "Categorical directional market impact of the news"
        },
        "severity_level": {
          "type": "string",
          "enum": ["Routine", "Notable", "Significant", "Critical"],
          "description": "Market impact severity rating"
        },
        "has_active_catalyst": {
          "type": "boolean",
          "description": "True if an active, fresh event driver is present"
        },
        "confidence": {
          "type": "number",
          "description": "Calibrated subjective certainty between 0.0 and 1.0"
        }
      },
      "required": [
        "evidence_quotes",
        "slang_and_negations",
        "counter_arguments_considered",
        "entry_direction",
        "exit_recommended",
        "news_impact",
        "severity_level",
        "has_active_catalyst",
        "confidence"
      ],
      "additionalProperties": false
    }
  }
}
```

---

## 5. Decisions, Rationale, and Alternatives

| Topic | Decision Made | Rationale | Alternatives Considered & Rejected |
|---|---|---|---|
| **Question Batching in Jev** | Submit all five judgments (`entry_direction`, `exit_now`, `news_impact`, `severity`, `has_catalyst`) in a single parallel Jev request. | Jev evaluates questions concurrently in one forward pass. Single-request execution reduces network overhead, lowers latency by ~10x, and reduces token charges by ~12x. | *Sequential calls*: Evaluates each question independently. Rejected due to latency penalty ($5 \times 150\text{ms} = 750\text{ms}$) and redundant state token billing. |
| **Reasoning Placement in LLM Schema** | Place `evidence_quotes`, `slang_and_negations`, and `counter_arguments_considered` before decision labels in JSON schema. | Autoregressive models condition subsequent tokens on preceding tokens. Constrained FSM decoding forces field order; declaring labels first results in post-hoc rationalization without causal reasoning. | *Label first, reasoning last*: More readable JSON hierarchy. Rejected because the decision is made with zero conditioning on the analysis, reducing accuracy on complex tasks. |
| **Criteria Granularity in Jev** | Use structured object criteria (`what`, `not_for`, `examples`) for Choice and Noul rather than plain strings. | TypeSafe documentation proves that defining negative boundaries (`not_for`) and positive examples prevents misclassification on edge cases. | *Single-sentence criteria strings*: Simpler payload. Rejected because boundary ambiguity between `NO_TRADE` and directional bias causes probability dispersion. |
| **Severity Metric Representation** | Use 4-level discrete Score primitive (`Routine`, `Notable`, `Significant`, `Critical`) with descriptive situational criteria. | Jev documentation explicitly prohibits numeric interpolation or bare number criteria (`["0", "1", "2"]`). Situation-based levels provide calibrated probability weights. | *Continuous floating-point score*: Ask LLM/Jev for a 0.0–1.0 float. Rejected because Jev cannot generate arbitrary numbers and LLM float outputs suffer from severe clustering bias. |
| **Vocabulary Standardization** | Enforce position intent vocabulary (`LONG`, `SHORT`, `NO_TRADE`) across both Jev and LLM interfaces. | Perpetual futures trade bidirectionally. Order sides (`BUY`/`SELL`) are ambiguous because buying occurs on long entries and short exits. | *Order sides (`BUY`, `SELL`, `HOLD`)*: Standard spot market convention. Rejected per Spec-013 constraint to prevent execution layer inversion bugs. |
| **Handling Negations & Slang** | Provide explicit domain lexicon in LLM system prompt and require an explicit `slang_and_negations` extraction field. | Crypto news frequently uses double negatives ("denies fraud rumors") and inverted sentiment terms ("god candle", "short squeeze"). Forcing explicit extraction resolves polarity prior to label selection. | *Implicit classification*: Rely on base model pre-training. Rejected due to documented failure rates of LLMs on financial negation. |

---

## 6. Sources

1. **TypeSafe Documentation Index**: [https://docs.typesafe.ai/llms.txt](https://docs.typesafe.ai/llms.txt)
2. **TypeSafe System One Architecture**: [https://docs.typesafe.ai/concepts/how-to-build-with-system-one.md](https://docs.typesafe.ai/concepts/how-to-build-with-system-one.md)
3. **TypeSafe State Design**: [https://docs.typesafe.ai/concepts/state.md](https://docs.typesafe.ai/concepts/state.md)
4. **TypeSafe Primitives Guide**: [https://docs.typesafe.ai/primitives.md](https://docs.typesafe.ai/primitives.md)
5. **TypeSafe Choice Primitive**: [https://docs.typesafe.ai/primitives/choice.md](https://docs.typesafe.ai/primitives/choice.md)
6. **TypeSafe Noul Primitive**: [https://docs.typesafe.ai/primitives/noul.md](https://docs.typesafe.ai/primitives/noul.md)
7. **TypeSafe Score Primitive**: [https://docs.typesafe.ai/primitives/score.md](https://docs.typesafe.ai/primitives/score.md)
8. **TypeSafe Advanced Structure**: [https://docs.typesafe.ai/primitives/advanced.md](https://docs.typesafe.ai/primitives/advanced.md)
9. **TypeSafe Confidence & Calibration**: [https://docs.typesafe.ai/confidence.md](https://docs.typesafe.ai/confidence.md)
10. **TypeSafe Jev 1.13 Jaggedness & Failure Modes**: [https://docs.typesafe.ai/model-jaggedness/jev-1.13.md](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md)
11. **TypeSafe Speculative Fan-Out Pattern**: [https://docs.typesafe.ai/patterns/fan-out.md](https://docs.typesafe.ai/patterns/fan-out.md)
12. **TypeSafe Composite Scoring Pattern**: [https://docs.typesafe.ai/patterns/composite-scoring.md](https://docs.typesafe.ai/patterns/composite-scoring.md)
13. **TypeSafe Trading Function Calling Cookbook**: [https://docs.typesafe.ai/cookbooks/function_calling.md](https://docs.typesafe.ai/cookbooks/function_calling.md)
14. **TypeSafe System One API Reference**: [https://docs.typesafe.ai/api.md](https://docs.typesafe.ai/api.md)
15. **OpenAI Structured Outputs Guide**: [https://developers.openai.com/api/docs/guides/structured-outputs](https://developers.openai.com/api/docs/guides/structured-outputs)
16. **Why JSON Schema Field Order Breaks Structured Output Accuracy**: [https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985](https://dev.to/ji_ai/why-json-schema-field-order-breaks-structured-output-accuracy-2985)
17. **LLM Structured Outputs: Schema Validation for Real Pipelines**: [https://collinwilkins.com/articles/structured-output](https://collinwilkins.com/articles/structured-output)
18. **LLM-as-a-Judge Rubric Design Guide**: [https://towardsdatascience.com/llm-as-a-judge-a-practical-guide/](https://towardsdatascience.com/llm-as-a-judge-a-practical-guide/)
19. **Prompt Engineering Guide - Few-Shot Prompting**: [https://www.promptingguide.ai/techniques/fewshot](https://www.promptingguide.ai/techniques/fewshot)
20. **Enhancing Zero-Shot Crypto Sentiment with LLMs & Prompt Engineering**: [https://arxiv.org/html/2310.13226](https://arxiv.org/html/2310.13226)
