# incubating/factory — Structure, Schemas, and Seed Cards

## Authoritative Design Source

Use `docs/ms-factory-impl-guide.md` (NOT the older `incubating-factory-plan.md`
which uses outdated terms like `failure`, `perf_feature`, `algo_feature` and 8
lifecycle states).

---

## 1. Directory Structure

```text
incubating/factory/
├── README.md
├── docs/
│   └── overview.md
├── schemas/
│   ├── known_failure.schema.yaml
│   ├── operator.schema.yaml
│   ├── trick.schema.yaml
│   ├── model.schema.yaml
│   └── report.schema.yaml
├── cards/
│   ├── known_failures/
│   ├── operators/
│   ├── tricks/
│   └── models/
├── reports/
│   └── .gitkeep
├── manifests/
│   ├── index.yaml
│   └── pack.yaml
└── packs/
    └── stable/
        └── v0.1/
```

No `vocab/` folder yet. No `lifecycle/` or `confidence/` folders — those are
metadata fields inside cards.

---

## 2. Card Taxonomy (4 kinds)

| kind | directory | what it holds |
|------|-----------|---------------|
| `known_failure` | `cards/known_failures/` | Diagnosed failure patterns with detection rules and fixes |
| `operator` | `cards/operators/` | Operator support/status per platform, fallbacks, variants |
| `trick` | `cards/tricks/` | Both perf optimizations AND algo techniques (differentiated by `category`) |
| `model` | `cards/models/` | Model training profiles with expected metrics and baselines |

Key design decision: **`trick` is the unified term** for both performance
optimizations (fused-adam, flash-attn, gradient-ckpt, etc.) and algorithm
techniques (MHC, LoRA+, GaLore, DPO, etc.). They are differentiated by the
`category` field, not by separate kinds.

---

## 3. Common Metadata (all cards share this)

Every card YAML file has these top-level fields:

```yaml
kind: <operator | known_failure | trick | model>
id: <kebab-case-unique-id>
lifecycle:
  state: <draft | stable | archived>    # only 3 states
source:
  kind: <bootstrap | execution | manual> # how the card was created
confidence:
  level: <bootstrap | observed | verified> # evidence strength
```

- `lifecycle.state`: `draft` -> `stable` -> `archived`. No other states.
- `source.kind`: `bootstrap` for cards seeded from demo.go data. `execution`
  for future cards from real runs. `manual` for human-authored.
- `confidence.level`: `bootstrap` for seeded cards (no real evidence yet).
  `observed` for cards backed by real run data. `verified` for cards with
  multiple confirming runs.

---

## 4. Schema Designs Per Kind

### 4.1 known_failure.schema.yaml

```yaml
required: [kind, id, lifecycle, source, confidence, severity, description, detection]
properties:
  kind: { const: known_failure }
  id: { type: string }
  lifecycle:
    state: { enum: [draft, stable, archived] }
  source:
    kind: { enum: [bootstrap, execution, manual] }
  confidence:
    level: { enum: [bootstrap, observed, verified] }
  severity: { enum: [low, medium, high, critical] }
  tags: { type: array, items: string }           # free-form search hints
  affects_operators: { type: array, items: string }  # operator card ids
  affects_platforms: { type: array, items: string }  # e.g. ["ascend-910b"]
  detection:
    pattern: { type: string }       # regex or keyword for log matching
    metric: { type: string }        # optional metric name
    threshold: { type: number }     # optional threshold
  description: { type: string }    # markdown: root cause + symptom (LLM-readable prose)
  fix:
    summary: { type: string }       # one-line fix description
    diff: { type: string }          # optional inline diff
    operator_id: { type: string }   # optional link to operator card
```

### 4.2 operator.schema.yaml

```yaml
required: [kind, id, lifecycle, source, confidence, name, category]
properties:
  kind: { const: operator }
  id: { type: string }
  name: { type: string }                # human-readable name
  category: { enum: [attention, optimizer, activation, norm, comm, memory] }
  lifecycle:
    state: { enum: [draft, stable, archived] }
  source:
    kind: { enum: [bootstrap, execution, manual] }
  confidence:
    level: { enum: [bootstrap, observed, verified] }
  platforms:                             # map of platform support
    type: object
    additionalProperties:                # key = "<framework>-<version>+<device>"
      status: { enum: [supported, not_supported, experimental] }
      profiled: { type: boolean }
      latency_ms: { type: number }       # optional
      constraints: { type: array, items: string }  # optional
  fallback: { type: string }            # optional operator id
  optimized_variant: { type: string }   # optional operator id
  description: { type: string }         # LLM-readable prose
```

### 4.3 trick.schema.yaml

```yaml
required: [kind, id, lifecycle, source, confidence, name, category]
properties:
  kind: { const: trick }
  id: { type: string }
  name: { type: string }
  category:
    enum: [compute, memory, communication, compilation,    # perf categories
           loss, attention, optimizer, regularization, scaling]  # algo categories
  lifecycle:
    state: { enum: [draft, stable, archived] }
  source:
    kind: { enum: [bootstrap, execution, manual] }
  confidence:
    level: { enum: [bootstrap, observed, verified] }
  description: { type: string }          # what it does, LLM-readable
  expected_gain: { type: string }        # e.g. "+10% throughput" or "+1.5 pts accuracy"
  platforms: { type: array, items: string }  # applicable platforms
  compatible_methods: { type: array, items: string }  # e.g. ["lora", "full"]
  config_diff: { type: string }          # optional inline config change
  code_diff: { type: string }            # optional inline code change
  dependencies: { type: array, items: string }  # optional trick ids
```

### 4.4 model.schema.yaml

```yaml
required: [kind, id, lifecycle, source, confidence, model, method, platform]
properties:
  kind: { const: model }
  id: { type: string }
  model: { type: string }               # e.g. "qwen3-7b"
  method: { type: string }              # e.g. "lora"
  platform: { type: string }            # e.g. "ascend-910b"
  framework: { type: string }           # e.g. "torch-2.7", "mindspore-2.5"
  lifecycle:
    state: { enum: [draft, stable, archived] }
  source:
    kind: { enum: [bootstrap, execution, manual] }
  confidence:
    level: { enum: [bootstrap, observed, verified] }
  config: { type: object }              # training config snapshot
  expected:
    final_loss: { type: number }
    throughput: { type: string }         # range, e.g. "510-520 tok/s"
    eval_acc: { type: object }           # benchmark -> range
    training_time: { type: string }
  baselines:
    type: array
    items:
      benchmark: { type: string }
      pretrain_acc: { type: number }
      posttrain_acc: { type: number }
      tolerance: { type: number }
  known_issues: { type: array, items: string }  # failure card ids
  verified_tricks: { type: object }      # trick id -> observed metrics
```

### 4.5 report.schema.yaml

```yaml
required: [kind, id, lifecycle, source, confidence, context, observation]
properties:
  kind: { const: report }
  id: { type: string }                  # e.g. "2026-03-18-qwen3-ascend-failure-001"
  lifecycle:
    state: { enum: [draft, stable, archived] }
  source:
    kind: { enum: [bootstrap, execution, manual] }
  confidence:
    level: { enum: [bootstrap, observed, verified] }
  submitted_by: { type: string }
  submitted_at: { type: string, format: date-time }
  context:
    model: { type: string }
    method: { type: string }
    platform: { type: string }
    framework: { type: string }
    device_count: { type: integer }
  observation:
    type: { enum: [failure, accuracy, performance, success] }
    summary: { type: string }
    error_log: { type: string }
  metrics:
    steps_completed: { type: integer }
    final_loss: { type: number }
    throughput: { type: number }
    eval_acc: { type: object }
  config_snapshot: { type: object }
```

Reports are v0.1 scaffolding only (just schema + `.gitkeep`). No seeded reports.

---

## 5. Seed Cards from demo.go

All seeded cards use `source.kind: bootstrap` and `confidence.level: bootstrap`.

### 5.1 Operators (6 cards)

| id | name | category | key data from demo.go |
|----|------|----------|-----------------------|
| `dsa` | DSA | attention | not_supported on torch-2.7+ascend-910b |
| `adam` | Adam | optimizer | 400ms latency on ascend, optimized_variant: fused-adam |
| `fused-adam` | Fused Adam | optimizer | ~250ms on ascend, optimized variant of adam |
| `softmax` | Softmax | activation | fp16 drift issue on ascend |
| `flash-attention` | Flash Attention | attention | requires CANN >= 8.0.RC3, fallback: sdpa |
| `sdpa` | SDPA | attention | fallback for flash-attention |

### 5.2 Known Failures (3 cards)

| id | severity | detection pattern | from demo function |
|----|----------|-------------------|--------------------|
| `dsa-torch27-ascend` | critical | `DSA operator.*not implemented` | `AnalyzeFailure()` — DSA op not in torch 2.7 |
| `fp16-softmax-drift` | high | `accuracy drift.*16.8 pts` | `AnalyzeSingleLaneDrift()` — fp16 softmax causes 16.8pt accuracy drop |
| `cann-flash-attn-version` | high | `FlashAttentionScore.*not found` | `RunNPUAnalysis()` — FlashAttention needs CANN >= 8.0.RC3 |

### 5.3 Tricks (18 cards total)

**Perf tricks** (9 cards, from `RunSingleLanePerfFeature`):

| id | category | expected_gain |
|----|----------|---------------|
| `fused-adam` | compute | +10% throughput (518->571 tok/s) |
| `flash-attn-v2` | compute | faster forward pass |
| `gradient-ckpt` | memory | reduced memory usage |
| `bf16-mixed` | compute | better precision/perf tradeoff |
| `graph-mode` | compilation | graph engine optimization |
| `comm-overlap` | communication | overlap allreduce with backward |
| `zero-offload` | memory | CPU offload for optimizer states |
| `sequence-parallel` | communication | split sequence across TP group |
| `selective-recompute` | memory | checkpoint only attention layers |

**Algo tricks** (9 cards, from `RunSingleLaneAlgoFeature`):

| id | category | expected_gain |
|----|----------|---------------|
| `mhc` | loss | +1.5 pts accuracy (72.1->73.6%) |
| `lora-plus` | optimizer | improved convergence via differential LR |
| `galore` | optimizer | reduced optimizer memory via low-rank projection |
| `dpo` | loss | direct preference optimization |
| `rope-scaling` | scaling | 4x context length extension |
| `moe-routing` | scaling | mixture-of-experts with top-k routing |
| `flash-attn` | attention | fused IO-aware attention kernel |
| `sparse-attn` | attention | block-sparse attention, 0.75 sparsity |
| `ddpm-noise` | loss | denoising diffusion noise scheduling |

Each trick card includes `config_diff` and/or `code_diff` extracted from the
demo diff strings.

### 5.4 Models (1 card)

| id | model | method | platform |
|----|-------|--------|----------|
| `qwen3-7b-lora-ascend-910b` | qwen3-7b | lora | ascend-910b |

Expected metrics (from demo step data):

- `final_loss`: 0.831 (base), 0.831 (perfFixed), 0.718 (trickApplied)
- `throughput`: "510-520 tok/s" (base), "565-572 tok/s" (with fused-adam)
- `eval_acc.ceval-valid`: 72.1% (baseline), 73.6% (with MHC)
- `known_issues`: ["dsa-torch27-ascend", "fp16-softmax-drift"]
- `verified_tricks`: fused-adam (571 tok/s), mhc (+1.5 pts)

---

## 6. Manifests

**`manifests/index.yaml`**:

```yaml
channels:
  stable:
    latest: "v0.1"
    url: "packs/stable/v0.1/"
```

**`manifests/pack.yaml`**:

```yaml
version: "v0.1"
channel: stable
created_at: "2026-03-18"
card_count:
  operators: 6
  known_failures: 3
  tricks: 18
  models: 1
cards:
  - { id: dsa, kind: operator, path: operators/dsa.yaml }
  - { id: adam, kind: operator, path: operators/adam.yaml }
  - { id: fused-adam, kind: operator, path: operators/fused-adam.yaml }
  - { id: softmax, kind: operator, path: operators/softmax.yaml }
  - { id: flash-attention, kind: operator, path: operators/flash-attention.yaml }
  - { id: sdpa, kind: operator, path: operators/sdpa.yaml }
  - { id: dsa-torch27-ascend, kind: known_failure, path: known_failures/dsa-torch27-ascend.yaml }
  - { id: fp16-softmax-drift, kind: known_failure, path: known_failures/fp16-softmax-drift.yaml }
  - { id: cann-flash-attn-version, kind: known_failure, path: known_failures/cann-flash-attn-version.yaml }
  - { id: fused-adam, kind: trick, path: tricks/fused-adam.yaml }
  - { id: flash-attn-v2, kind: trick, path: tricks/flash-attn-v2.yaml }
  - { id: gradient-ckpt, kind: trick, path: tricks/gradient-ckpt.yaml }
  - { id: bf16-mixed, kind: trick, path: tricks/bf16-mixed.yaml }
  - { id: graph-mode, kind: trick, path: tricks/graph-mode.yaml }
  - { id: comm-overlap, kind: trick, path: tricks/comm-overlap.yaml }
  - { id: zero-offload, kind: trick, path: tricks/zero-offload.yaml }
  - { id: sequence-parallel, kind: trick, path: tricks/sequence-parallel.yaml }
  - { id: selective-recompute, kind: trick, path: tricks/selective-recompute.yaml }
  - { id: mhc, kind: trick, path: tricks/mhc.yaml }
  - { id: lora-plus, kind: trick, path: tricks/lora-plus.yaml }
  - { id: galore, kind: trick, path: tricks/galore.yaml }
  - { id: dpo, kind: trick, path: tricks/dpo.yaml }
  - { id: rope-scaling, kind: trick, path: tricks/rope-scaling.yaml }
  - { id: moe-routing, kind: trick, path: tricks/moe-routing.yaml }
  - { id: flash-attn, kind: trick, path: tricks/flash-attn.yaml }
  - { id: sparse-attn, kind: trick, path: tricks/sparse-attn.yaml }
  - { id: ddpm-noise, kind: trick, path: tricks/ddpm-noise.yaml }
  - { id: qwen3-7b-lora-ascend-910b, kind: model, path: models/qwen3-7b-lora-ascend-910b.yaml }
```

---

## 7. Key Design Rules

1. **Lifecycle in metadata, not directories** — cards don't move between
   folders when their state changes.
2. **Structured fields for code queries, prose for LLM reasoning** —
   deterministic code filters on `kind`, `category`, `id`, `tags`; LLM
   interprets `description`, `fix.summary`, diffs.
3. **No vocab/ yet** — defer structured normalization of `framework`,
   `hardware`, `platform` values until actually needed.
4. **All bootstrap cards declare it** — `source.kind: bootstrap`,
   `confidence.level: bootstrap`.
5. **Schema enums are strict** — `kind`, `lifecycle.state`,
   `confidence.level`, `category` are closed enums. Prose fields are
   free-form.

---

## 8. Implementation Steps

1. Create directory structure
2. Write 5 schema YAML files
3. Seed 6 operator cards
4. Seed 3 known_failure cards
5. Seed 18 trick cards (9 perf + 9 algo)
6. Seed 1 model card
7. Write manifests (index.yaml + pack.yaml)
8. Write README.md and docs/overview.md
