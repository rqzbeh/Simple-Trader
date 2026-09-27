# Research: Jev AI / Callisifer

Date: 2026-09-27  
Status: Jev AI **FOUND** | Callisifer **RESOLVED** (Phonetic corruption of "Classifier")  
Method: Multi-engine web search via 9Router, primary source verification via 9Router web fetch, and architectural analysis of Simple-Trader.

---

## 1. Status & Entity Verification

### 1.1 Summary Status
- **Jev AI**: **FOUND**. Jev is a proprietary non-autoregressive decision model developed by TypeSafe AI, released in early access on September 15, 2026.
- **Callisifer**: **NOT FOUND** as a distinct named model, product, or protocol. **RESOLVED** with high certainty as a phonetic transcription error for **"Classifier"** (`Cal-li-si-fer` $\rightarrow$ `Classifier`). In TypeSafe AI and external frameworks (LangChain, DEV, HuggingFace), Jev's core primitive is explicitly packaged and marketed as the **Jev Classifier** ([TypeSafe Choice primitive](https://docs.typesafe.ai/primitives/choice.md), [TypeSafeClassifier](https://www.langchain.com/blog/building-a-harness-with-jev), [Jev Classifier](https://jevtypesafeai.com/jev/classifier)).

### 1.2 Search Log & Evidence
1. **Query**: `"Callisifer"`  
   - Result: 0 technical, AI, or financial matches. Results limited to character names in fiction (e.g. *Borderlands 4* antagonist Callis) and pet naming sites.
2. **Query**: `"Calcifer" AI trading crypto`  
   - Result: Matches limited to a legacy BNB meme token (`CALCIFER`/`CALCIFIRE`) and anime references (*Howl's Moving Castle*). No connection to AI modeling or algorithmic trading.
3. **Query**: `"Jev AI model"`  
   - Result: Comprehensive hits for TypeSafe AI's launch of Jev, the inaugural "System One" decision model.
4. **Query**: `"TypeSafe AI" Jev classifier`  
   - Result: Direct matches confirming Jev's primary architectural function: discrete categorical classification without text generation ([TypeSafe Documentation](https://docs.typesafe.ai/), [LangChain Blog](https://www.langchain.com/blog/building-a-harness-with-jev), [Jev Product Spec](https://jevtypesafeai.com/jev/classifier)).

---

## 2. What It Is

[Jev](https://en.wikipedia.org/wiki/Jev_(AI_model)) is an artificial intelligence model developed by [TypeSafe AI](https://typesafe.ai/), a company founded in 2024 by Diogo Almeida (co-creator of ChatGPT and RLHF at OpenAI), Erik Gafni, and Sasha Sheng, backed by a $40M seed round led by DCVC.

TypeSafe AI released Jev on September 15, 2026, defining it as the first **System One Model** ([TypeSafe Announcement](https://typesafe.ai/blog/introducing-system-one-models-and-jev)). The terminology derives from Daniel Kahneman's *Thinking, Fast and Slow* (System 1 intuitive, fast cognition vs. System 2 slow deliberative reasoning) and William Stanley Jevons (the Jevons Paradox, postulating that exponential efficiency drops in intelligence costs drive massive consumption expansions).

Unlike standard Large Language Models (LLMs) which generate text autoregressively token-by-token, Jev is a **non-autoregressive decision model** ([Forbes Analysis](https://www.forbes.com/sites/ronschmelzer/2026/09/22/why-everyone-is-talking-about-jev-the-ai-that-doesnt-chat/)). It takes program state (unstructured text or structured JSON) and evaluates predefined typed questions in parallel. It produces **calibrated probabilities and discrete choices** with zero string generation.

---

## 3. Capabilities & Specifications

### 3.1 Technical Specifications

| Metric | System One / Jev | Standard Frontier LLMs (GPT-4o, Claude 3.5, Gemini 1.5/2.5) | Evidence |
|---|---|---|---|
| **Architecture** | Parallel non-autoregressive decision network | Autoregressive Transformer decoder | [TypeSafe Blog](https://typesafe.ai/blog/introducing-system-one-models-and-jev) |
| **Training Method** | Reinforcement Learning for Calibrated Decisions (RLCD) | RLHF / RLVR (preference and verifiable reasoning) | [TypeSafe Blog](https://typesafe.ai/blog/introducing-system-one-models-and-jev) |
| **Output Type** | Typed values, probability distributions, confidence scores | Unstructured text strings | [TypeSafe Docs](https://docs.typesafe.ai/introduction.md) |
| **Latency** | **70 ms – 500 ms** end-to-end | 3.0 s – 30.0+ s end-to-end | [TypeSafe Benchmark](https://typesafe.ai/blog/introducing-system-one-models-and-jev) |
| **Input Cost** | **$0.042 / MTok** ($42 / billion tokens) | $0.20 – $10.00 / MTok | [Forbes](https://www.forbes.com/sites/ronschmelzer/2026/09/22/why-everyone-is-talking-about-jev-the-ai-that-doesnt-chat/) |
| **Output Cost** | **FREE** ($0.00, too cheap to meter) | $1.00 – $30.00 / MTok | [TypeSafe Blog](https://typesafe.ai/blog/introducing-system-one-models-and-jev) |
| **Schema Reliability**| **100% (0% schema/type errors)** | Variable (frequent JSON syntax / markdown parse errors) | [TypeSafe Docs](https://docs.typesafe.ai/api.md) |
| **Hallucination** | **Mathematically 0%** (cannot generate arbitrary text) | Constant nonzero probability | [TypeSafe Docs](https://docs.typesafe.ai/concepts/system-one.md) |
| **Concurrency** | Multiple independent questions evaluated in 1 parallel pass | Sequential chain-of-thought token generation | [TypeSafe Docs](https://docs.typesafe.ai/primitives.md) |

### 3.2 Core Primitives
TypeSafe exposes three evaluation primitives ([TypeSafe Primitives](https://docs.typesafe.ai/primitives.md)):

1. **Choice (The Classifier)** ([Docs](https://docs.typesafe.ai/primitives/choice.md)):
   - Selects one class out of a discrete set (up to 255 options).
   - Returns: `choice` (top option string), `probabilities` (full distribution across all classes summing to 1.0), and `confidence` (normalized certainty scalar $[0.0, 1.0]$).
   - Use cases: Routing, intent classification, sentiment categorization, directional trade bias (`BUY`, `SELL`, `HOLD`).
2. **Score** ([Docs](https://docs.typesafe.ai/primitives/score.md)):
   - Rates state against 2–10 ordered rubric levels.
   - Returns: `score` (probability-weighted continuous scalar), `probabilities` per level, and `confidence`.
   - Use cases: Quality rating, risk grading, conviction level.
3. **Noul** ([Docs](https://docs.typesafe.ai/primitives/noul.md)):
   - Evaluates a binary hypothesis.
   - Returns: `noul` (scalar probability between 0.0 and 1.0 that the condition is true).
   - Use cases: Fast boolean gates, filter predicates, sanity checks.

### 3.3 Confidence Calibration
Unlike chat LLMs whose self-reported confidence numbers correlate poorly with real accuracy, Jev's `confidence` score is mathematically derived from the dispersion of its output probability distribution ([TypeSafe Confidence Guide](https://docs.typesafe.ai/confidence.md)). A peaked distribution yields high confidence; a flat or multi-modal distribution yields low confidence, enabling automated code to execute deterministic fallback routines when the model signals uncertainty.

---

## 4. Relevance to Simple-Trader & Integration Analysis

### 4.1 Current Architecture & Pain Points in Simple-Trader
Inspection of `Simple-Trader` (`internal/ai/client.go`, `internal/trader/signals.go`, `specs/012-trading-system-optimization/`) highlights several architectural tensions with standard LLMs:

1. **Latency Bottleneck**:
   - `internal/trader/signals.go:245` allocates a 20-second context timeout (`context.WithTimeout(ctx, 20*time.Second)`) specifically because OpenAI-compatible chat endpoints often take 5–15 seconds to reply.
   - In fast crypto markets, 10-second latency causes significant execution slippage on breakout signals.
2. **Schema Brittle Parse Loop**:
   - `internal/ai/client.go:310-328` contains defensive string stripping for ` ```json ` code blocks, substring extraction between `{` and `}`, and fallback handling. When the LLM outputs conversational preambles or malformed JSON, the engine drops to `fallbackHeuristic`.
3. **High Scanning Expense**:
   - Evaluating 113 qualified assets across 8 RSS news streams with chat LLMs consumes millions of tokens, limiting the scan frequency.
4. **Epistemic Misalignment**:
   - Simple-Trader's Spec-012 (`FR-012`) recently separated news sentiment from AI confidence (`catalyst_sentiment` vs `model_confidence`). Generative LLMs often output arbitrary `0.7` confidence values with poor probabilistic calibration.

### 4.2 Does Jev Benefit Simple-Trader?
**Yes, with high strategic impact on execution speed, cost, and reliability**, provided its architectural boundary is respected.

#### Advantages:
- **Sub-Second Signal Generation**: Jev evaluates trade states in 70–300 ms, permitting near real-time reaction to breaking news and candle closes.
- **Cost Reduction**: At $0.042/MTok with free outputs, continuous scanning across the entire 113-coin universe costs pennies per day.
- **Zero Parse Failures**: Jev produces typed JSON answers matching the request schema directly. Eliminates JSON unmarshal errors and markdown sanitization.
- **Calibrated Probabilistic Inputs for Thompson Sampling**: Jev provides true probability distributions across choices (`probabilities: {"BUY": 0.72, "SELL": 0.08, "HOLD": 0.20}`). This directly feeds into Simple-Trader's Bayesian learning engine (`internal/ai/bayesian.go`).

#### Trade-offs & Constraints:
- **No Free-Form Text Reasoning**: Jev **cannot** generate the `reasoning` narrative or summarized `catalyst` strings currently saved into database audit logs.
  - *Mitigation*: The Go backend can generate structured `reasoning` strings deterministically from the indicator snapshot and winning probabilities (e.g., `fmt.Sprintf("Direction %s with confidence %.2f; aligned with RSI=%.1f, VolRatio=%.2f", decision, conf, rsi, vol)`).
- **No Free-Floating Number Generation**: Jev does not output arbitrary continuous floats for stop loss or take profit.
  - *Alignment*: Under Spec-012 (`internal/trader/signals.go:266-285`), Simple-Trader **already deprecated** AI-suggested SL/TP percentages in favor of deterministic ATR swing calculations (`CalculateATRStop` and `CalculateStagedTargets`). Jev only needs to output the directional decision and regime.

---

## 5. Integration Architecture Options

### Option 1: Direct Jev Client Replacement (Recommended for Signal Engine)
Replace or complement `internal/ai/client.go` with a native TypeSafe System One client calling `POST https://api.typesafe.ai/v1/systemone`.

```go
// Proposed Request Payload in Go:
type JevRequest struct {
    Model     string                   `json:"model"`     // "jev-latest"
    State     any                      `json:"state"`     // Struct with Quote, News, Microstructure
    Questions map[string]JevQuestion   `json:"questions"`
}
```

#### Proposed Question Schema for Simple-Trader:
1. `decision` (`Choice`):
   - Options:
     - `BUY`: "Strong near-term bullish catalyst confirmed by positive momentum."
     - `SELL`: "Strong near-term bearish catalyst confirmed by negative momentum."
     - `HOLD`: "No high-conviction catalyst, conflicting indicators, or neutral market."
2. `regime` (`Choice`):
   - Options: `BULL`, `BEAR`, `RANGING`.
3. `has_valid_catalyst` (`Noul`):
   - Hypothesis: "Does the supplied news contain a high-conviction near-term catalyst (0-24h) for this asset?"
4. `conviction` (`Score`):
   - Rubric: `["Low", "Moderate", "High", "Institutional"]`.

### Option 2: Two-Stage Hybrid Cascade
- **Stage 1 (Jev Filter Gate)**: Run every candidate symbol and news item through Jev (100 ms, $0.0001). If `has_valid_catalyst` < 0.60 or `decision == "HOLD"`, terminate immediately.
- **Stage 2 (Generative LLM for Executed Trades Only)**: Only when Jev fires a high-confidence `BUY` or `SELL`, invoke OmniRoute (`ag/gemini-3.8-flash-high`) to synthesize an institutional thesis paragraph for the trader dashboard and Telegram channel.

---

## 6. Sources List

- [TypeSafe AI Official Launch Manifesto & Blog](https://typesafe.ai/blog/introducing-system-one-models-and-jev)
- [TypeSafe AI Documentation Index](https://docs.typesafe.ai/llms.txt)
- [TypeSafe AI API Reference](https://docs.typesafe.ai/api.md)
- [TypeSafe AI Primitives: Choice (Classifier)](https://docs.typesafe.ai/primitives/choice.md)
- [TypeSafe AI Primitives: Score](https://docs.typesafe.ai/primitives/score.md)
- [TypeSafe AI Primitives: Noul](https://docs.typesafe.ai/primitives/noul.md)
- [TypeSafe AI Confidence Architecture](https://docs.typesafe.ai/confidence.md)
- [TypeSafe AI Trading Function Calling Cookbook](https://docs.typesafe.ai/cookbooks/function_calling.md)
- [Wikipedia: Jev (AI model)](https://en.wikipedia.org/wiki/Jev_(AI_model))
- [Forbes: Why Everyone Is Talking About Jev, The AI That Doesn't Chat](https://www.forbes.com/sites/ronschmelzer/2026/09/22/why-everyone-is-talking-about-jev-the-ai-that-doesnt-chat/)
- [LangChain: Building a Harness with Jev (TypeSafeClassifier)](https://www.langchain.com/blog/building-a-harness-with-jev)
- [DEV Community: How to Use Jev - A Practical Guide to TypeSafe's System One Model](https://dev.to/valyuai/how-to-use-jev-a-practical-guide-to-typesafes-system-one-model-g5e)
- [Jev Classifier Documentation & Playground](https://jevtypesafeai.com/jev/classifier)
