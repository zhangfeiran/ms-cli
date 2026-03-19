# incubating/factory — Structure, Schemas, and Seed Cards

## Authoritative Design Source

This is the authoritative factory structure spec for v0.1.

---

## 1. Directory Structure

```text
incubating/factory/
├── README.md
├── docs/
│   └── overview.md
├── schemas/
│   ├── known_issue.schema.yaml
│   ├── operator.schema.yaml
│   ├── perf_feature.schema.yaml
│   ├── algo_feature.schema.yaml
│   ├── model.schema.yaml
│   └── report.schema.yaml
├── cards/
│   ├── known_issues/
│   ├── operators/
│   ├── perf_features/
│   ├── algo_features/
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

## 2. Card Taxonomy (5 kinds)

| kind | directory | what it holds |
|------|-----------|---------------|
| `known_issue` | `cards/known_issues/` | Diagnosed problems (failure, accuracy, performance) with detection rules and fixes |
| `operator` | `cards/operators/` | Operator support/status per platform, fallbacks, variants |
| `perf_feature` | `cards/perf_features/` | Performance optimizations (compute, memory, communication, compilation) |
| `algo_feature` | `cards/algo_features/` | Algorithm-level techniques (loss, attention, optimizer, regularization, scaling) |
| `model` | `cards/models/` | Model training profiles with expected metrics and baselines |

Performance features and algorithm features are separate kinds with their own
schemas and directories. This makes filtering and skill routing straightforward.

---

## 3. Common Metadata (all cards share this)

Every card YAML file has these top-level fields:

```yaml
kind: <operator | known_issue | perf_feature | algo_feature | model>
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

### 4.1 known_issue.schema.yaml

```yaml
required: [kind, id, lifecycle, source, confidence, symptom, severity, description, detection]
properties:
  kind: { const: known_issue }
  id: { type: string }
  symptom: { enum: [failure, accuracy, performance] }  # problem category
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

The `symptom` field drives skill routing:
- `failure` -> `failure-agent` consults these during `/diagnose` and `/fix`
- `accuracy` -> `accuracy-agent` consults these
- `performance` -> `performance-agent` consults these

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

### 4.3 perf_feature.schema.yaml

```yaml
required: [kind, id, lifecycle, source, confidence, name, category]
properties:
  kind: { const: perf_feature }
  id: { type: string }
  name: { type: string }
  category: { enum: [compute, memory, communication, compilation] }
  lifecycle:
    state: { enum: [draft, stable, archived] }
  source:
    kind: { enum: [bootstrap, execution, manual] }
  confidence:
    level: { enum: [bootstrap, observed, verified] }
  description: { type: string }          # what it does, LLM-readable
  expected_gain: { type: string }        # e.g. "+10% throughput"
  platforms: { type: array, items: string }  # applicable platforms
  compatible_methods: { type: array, items: string }  # e.g. ["lora", "full"]
  config_diff: { type: string }          # optional inline config change
  code_diff: { type: string }            # optional inline code change
  dependencies: { type: array, items: string }  # optional feature ids
```

### 4.4 algo_feature.schema.yaml

```yaml
required: [kind, id, lifecycle, source, confidence, name, category]
properties:
  kind: { const: algo_feature }
  id: { type: string }
  name: { type: string }
  category: { enum: [loss, attention, optimizer, regularization, scaling] }
  lifecycle:
    state: { enum: [draft, stable, archived] }
  source:
    kind: { enum: [bootstrap, execution, manual] }
  confidence:
    level: { enum: [bootstrap, observed, verified] }
  description: { type: string }          # what it does, LLM-readable
  expected_gain: { type: string }        # e.g. "+1.5 pts accuracy"
  platforms: { type: array, items: string }  # applicable platforms
  compatible_methods: { type: array, items: string }  # e.g. ["lora", "full"]
  config_diff: { type: string }          # optional inline config change
  code_diff: { type: string }            # optional inline code change
  dependencies: { type: array, items: string }  # optional feature ids
```

### 4.5 model.schema.yaml

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
  known_issues: { type: array, items: string }  # known_issue card ids
  verified_perf_features: { type: object }   # perf_feature id -> observed metrics
  verified_algo_features: { type: object }   # algo_feature id -> observed metrics
```

### 4.6 report.schema.yaml

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

### 5.2 Known Issues (3 cards)

| id | symptom | severity | detection pattern | from demo function |
|----|---------|----------|-------------------|--------------------|
| `dsa-torch27-ascend` | failure | critical | `DSA operator.*not implemented` | `AnalyzeFailure()` — DSA op not in torch 2.7 |
| `fp16-softmax-drift` | accuracy | high | `accuracy drift.*16.8 pts` | `AnalyzeSingleLaneDrift()` — fp16 softmax causes 16.8pt accuracy drop |
| `cann-flash-attn-version` | failure | high | `FlashAttentionScore.*not found` | `RunNPUAnalysis()` — FlashAttention needs CANN >= 8.0.RC3 |

### 5.3 Perf Features (9 cards, from `RunSingleLanePerfFeature`)

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

### 5.4 Algo Features (9 cards, from `RunSingleLaneAlgoFeature`)

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

Each feature card includes `config_diff` and/or `code_diff` extracted from the
demo diff strings.

### 5.5 Models (1 card)

| id | model | method | platform |
|----|-------|--------|----------|
| `qwen3-7b-lora-ascend-910b` | qwen3-7b | lora | ascend-910b |

Expected metrics (from demo step data):

- `final_loss`: 0.831 (base), 0.831 (perfFixed), 0.718 (trickApplied)
- `throughput`: "510-520 tok/s" (base), "565-572 tok/s" (with fused-adam)
- `eval_acc.ceval-valid`: 72.1% (baseline), 73.6% (with MHC)
- `known_issues`: ["dsa-torch27-ascend", "fp16-softmax-drift"]
- `verified_perf_features`: fused-adam (571 tok/s)
- `verified_algo_features`: mhc (+1.5 pts)

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
  known_issues: 3
  perf_features: 9
  algo_features: 9
  models: 1
cards:
  - { id: dsa, kind: operator, path: operators/dsa.yaml }
  - { id: adam, kind: operator, path: operators/adam.yaml }
  - { id: fused-adam, kind: operator, path: operators/fused-adam.yaml }
  - { id: softmax, kind: operator, path: operators/softmax.yaml }
  - { id: flash-attention, kind: operator, path: operators/flash-attention.yaml }
  - { id: sdpa, kind: operator, path: operators/sdpa.yaml }
  - { id: dsa-torch27-ascend, kind: known_issue, path: known_issues/dsa-torch27-ascend.yaml }
  - { id: fp16-softmax-drift, kind: known_issue, path: known_issues/fp16-softmax-drift.yaml }
  - { id: cann-flash-attn-version, kind: known_issue, path: known_issues/cann-flash-attn-version.yaml }
  - { id: fused-adam, kind: perf_feature, path: perf_features/fused-adam.yaml }
  - { id: flash-attn-v2, kind: perf_feature, path: perf_features/flash-attn-v2.yaml }
  - { id: gradient-ckpt, kind: perf_feature, path: perf_features/gradient-ckpt.yaml }
  - { id: bf16-mixed, kind: perf_feature, path: perf_features/bf16-mixed.yaml }
  - { id: graph-mode, kind: perf_feature, path: perf_features/graph-mode.yaml }
  - { id: comm-overlap, kind: perf_feature, path: perf_features/comm-overlap.yaml }
  - { id: zero-offload, kind: perf_feature, path: perf_features/zero-offload.yaml }
  - { id: sequence-parallel, kind: perf_feature, path: perf_features/sequence-parallel.yaml }
  - { id: selective-recompute, kind: perf_feature, path: perf_features/selective-recompute.yaml }
  - { id: mhc, kind: algo_feature, path: algo_features/mhc.yaml }
  - { id: lora-plus, kind: algo_feature, path: algo_features/lora-plus.yaml }
  - { id: galore, kind: algo_feature, path: algo_features/galore.yaml }
  - { id: dpo, kind: algo_feature, path: algo_features/dpo.yaml }
  - { id: rope-scaling, kind: algo_feature, path: algo_features/rope-scaling.yaml }
  - { id: moe-routing, kind: algo_feature, path: algo_features/moe-routing.yaml }
  - { id: flash-attn, kind: algo_feature, path: algo_features/flash-attn.yaml }
  - { id: sparse-attn, kind: algo_feature, path: algo_features/sparse-attn.yaml }
  - { id: ddpm-noise, kind: algo_feature, path: algo_features/ddpm-noise.yaml }
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
2. Write 6 schema YAML files
3. Seed 6 operator cards
4. Seed 3 known_issue cards
5. Seed 9 perf_feature cards
6. Seed 9 algo_feature cards
7. Seed 1 model card
8. Write manifests (index.yaml + pack.yaml)
9. Write README.md and docs/overview.md
