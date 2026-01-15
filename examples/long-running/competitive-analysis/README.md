# Competitive Intelligence Agent

**Long-running multi-step workflow demonstrating Orpheus capabilities**

This agent performs deep competitive analysis using a 6-phase research pipeline that takes 5-10 minutes to complete. It demonstrates Orpheus's ability to handle long-running workflows with extended timeout support.

## What This Demonstrates

- ✅ **Extended timeout support** (600 seconds for long workflows)
- ✅ **Real-time progress visibility** (stdout streaming via `orpheus logs`)
- ✅ **Workspace persistence** (intermediate results survive crashes)
- ✅ **Multi-provider LLM support** (OpenAI for research, Anthropic for synthesis)

## Technical Stack

- **Framework**: OpenAI SDK + Anthropic SDK (direct integration)
- **Primary Model**: GPT-4o (phases 1-4: research, extraction, analysis)
- **Secondary Model**: Claude 3.5 Sonnet (phase 5: strategic synthesis)
- **Runtime**: Python 3.10+
- **Timeout**: 600 seconds (10 minutes)

## Multi-Step Workflow (6 Phases)

### Phase 1: Information Gathering (60-90s)
- LLM generates strategic search queries (8-10 per company)
- Executes web searches (simulated or real API)
- Caches raw results to `/workspace/sources/{company}/raw.json`

### Phase 2: Source Extraction (45-60s)
- LLM extracts structured data (features, pricing, tech stack)
- Saves to `/workspace/extracts/{company}.json`

### Phase 3: Comparative Analysis (60-90s)
- LLM builds comparison matrix (feature × company)
- Identifies differentiators, overlaps, gaps
- Saves to `/workspace/analysis/comparison.json`

### Phase 4: Market Intelligence (45-60s)
- LLM detects market trends from all sources
- Saves to `/workspace/analysis/intelligence.json`

### Phase 5: Strategic Synthesis (60-90s)
- **Switches to Claude 3.5 Sonnet** for deep reasoning
- Generates SWOT analysis and strategic recommendations
- Saves to `/workspace/analysis/strategy.json`

### Phase 6: Report Generation (30-45s)
- LLM formats comprehensive markdown report
- Saves to `/workspace/reports/competitive_analysis_{timestamp}.md`

**Total Time**: 5-8 minutes

## Prerequisites

1. **Environment Variables**:
   ```bash
   export OPENAI_API_KEY="sk-..."
   export ANTHROPIC_API_KEY="sk-ant-..."
   ```

2. **Python Dependencies** (installed automatically by Orpheus):
   - openai>=1.12.0
   - anthropic>=0.18.0
   - httpx>=0.26.0
   - beautifulsoup4>=4.12.0
   - pydantic>=2.0.0

## Usage

### Deploy the Agent

```bash
cd examples/competitive-intelligence-agent
orpheus deploy .
```

### Execute Analysis

```bash
orpheus invoke competitive-intelligence-agent '{
  "target_company": "Stripe",
  "competitors": ["Adyen", "Square"],
  "focus_areas": ["pricing", "features", "market_position"]
}'
```

### Monitor Progress (Real-time)

```bash
# Watch live progress logs
orpheus logs competitive-intelligence-agent -f

# Example output:
# [1/6] Information Gathering: Generating search queries (2.1s)
# [1/6] Information Gathering: Executing 10 searches (15.3s)
# [1/6] Information Gathering: Gathered 24 sources (72.0s)
# [2/6] Source Extraction: Processing sources (8.4s)
# ...
```

### Check Workspace

```bash
# View workspace contents
orpheus workspace info competitive-intelligence-agent

# Expected structure:
# /workspace/
#   sources/
#     Stripe/raw.json
#     Adyen/raw.json
#     Square/raw.json
#   extracts/
#     Stripe.json
#     Adyen.json
#     Square.json
#   analysis/
#     comparison.json
#     intelligence.json
#     strategy.json
#   reports/
#     competitive_analysis_20260115_143022.md
```

## Input Format

```json
{
  "target_company": "Stripe",           // Company you're analyzing
  "competitors": ["Adyen", "Square"],   // Competitors to compare against
  "focus_areas": [                      // Optional, defaults shown
    "pricing",
    "features",
    "market_position"
  ]
}
```

## Output Format

```json
{
  "status": "success",
  "execution_time_seconds": 387,
  "report_path": "/workspace/reports/competitive_analysis_20260115.md",
  "summary": {
    "companies_analyzed": 3,
    "sources_gathered": 24,
    "features_compared": 47,
    "strategic_recommendations": 8
  },
  "phases": [
    {"phase": "gathering", "duration_s": 72, "status": "completed", "details": {"sources": 24}},
    {"phase": "extraction", "duration_s": 53, "status": "completed", "details": {"items": 3}},
    {"phase": "analysis", "duration_s": 71, "status": "completed", "details": {"features": 47}},
    {"phase": "intelligence", "duration_s": 49, "status": "completed", "details": {"trends": 12}},
    {"phase": "synthesis", "duration_s": 68, "status": "completed", "details": {"recommendations": 8}},
    {"phase": "report", "duration_s": 38, "status": "completed", "details": {"pages": 12}}
  ]
}
```

## Example Use Cases

### 1. Payment Processors
```bash
orpheus invoke competitive-intelligence-agent '{
  "target_company": "Stripe",
  "competitors": ["Adyen", "Square", "PayPal"],
  "focus_areas": ["pricing", "features", "developer_experience"]
}'
```

### 2. Cloud Providers
```bash
orpheus invoke competitive-intelligence-agent '{
  "target_company": "AWS",
  "competitors": ["Azure", "GCP"],
  "focus_areas": ["pricing", "services", "market_share"]
}'
```

### 3. SaaS Platforms
```bash
orpheus invoke competitive-intelligence-agent '{
  "target_company": "Notion",
  "competitors": ["Coda", "Airtable"],
  "focus_areas": ["features", "integrations", "pricing"]
}'
```

## Key Features

### Extended Execution
- **Extended Timeout**: 600s (10 minutes) for complex workflows
- **Real-time Streaming**: See progress as it happens via logs
- **Workspace Persistence**: Intermediate results saved at each phase
- **Multi-Provider**: Mix OpenAI, Anthropic, local models in one workflow

## Architecture Notes

### Progress Logging
All phases use this pattern for real-time visibility:
```python
print(f"[{step}/{total}] {phase}: {message} ({elapsed}s)", file=sys.stderr, flush=True)
```

Logs appear immediately in `orpheus logs -f` output.

### Workspace Structure
Each phase saves to `/workspace/` for persistence:
- **Sources**: Raw web search results
- **Extracts**: Structured company data
- **Analysis**: Comparison matrices and intelligence
- **Reports**: Final markdown report

If agent crashes, partial results survive for inspection.

### Multi-Provider LLM
- **OpenAI GPT-4o**: Phases 1-4 (research-heavy, fast iteration)
- **Anthropic Claude**: Phase 5 (deep synthesis, strategic thinking)

Different models for different cognitive tasks.

## Troubleshooting

### API Key Errors
```bash
# Check environment variables
echo $OPENAI_API_KEY
echo $ANTHROPIC_API_KEY

# Set if missing
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
```

### Timeout Issues
If execution exceeds 600s, increase timeout in `agent.yaml`:
```yaml
timeout: 900  # 15 minutes
```

### View Workspace After Crash
```bash
orpheus workspace info competitive-intelligence-agent
orpheus workspace download competitive-intelligence-agent ./local-backup
```

### JSON Parsing Errors
The agent includes fallback mock data if LLM returns invalid JSON. Check logs for warnings.

## Local Testing (Without Orpheus)

```bash
# Install dependencies
pip install -r requirements.txt

# Set API keys
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."

# Create workspace directory
mkdir -p /workspace

# Run directly
python agent.py '{
  "target_company": "Stripe",
  "competitors": ["Adyen"],
  "focus_areas": ["pricing"]
}'
```

## Performance Expectations

| Companies | Focus Areas | Estimated Time | Sources | LLM Calls |
|-----------|-------------|----------------|---------|-----------|
| 2         | 2           | 4-5 min        | 15-20   | 8-10      |
| 3         | 3           | 5-7 min        | 20-30   | 12-15     |
| 5         | 4           | 8-10 min       | 35-50   | 18-25     |

## Future Enhancements

- **Real Web Search**: Integrate Serper, Tavily, or Brave Search API
- **Source Validation**: Check URL validity and fetch actual content
- **Parallel Processing**: Run multiple company extracts simultaneously
- **Report Templates**: Customizable output formats (PDF, PPTX)
- **Incremental Updates**: Re-run analysis on specific companies only

## License

MIT License - See root repository for details.
