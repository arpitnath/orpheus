#!/usr/bin/env python3
"""
Competitive Intelligence Agent - Long-running multi-step workflow

Demonstrates Orpheus handling of 5-10 minute workflows with:
- Multi-phase research pipeline (6 phases)
- Real-time progress streaming
- Workspace persistence
- Multi-provider LLM support (OpenAI + Anthropic)
"""

import os
import sys
import json
import asyncio
import time
from pathlib import Path
from datetime import datetime
from typing import Any, Dict, List

import httpx
from openai import OpenAI
from anthropic import Anthropic
from pydantic import BaseModel


# ─────────────────────────────────────────────────────────────
# Configuration
# ─────────────────────────────────────────────────────────────

OPENAI_MODEL = "gpt-5-mini"  # Latest GPT-5 mini model (Jan 2026)
ANTHROPIC_MODEL = "claude-haiku-4-5"  # Latest Haiku 4.5 (Jan 2026)

# Initialize clients
openai_client = OpenAI(api_key=os.getenv("OPENAI_API_KEY"))
anthropic_client = Anthropic(api_key=os.getenv("ANTHROPIC_API_KEY"))

# Workspace directories
WORKSPACE_BASE = Path("/workspace")
SOURCES_DIR = WORKSPACE_BASE / "sources"
EXTRACTS_DIR = WORKSPACE_BASE / "extracts"
ANALYSIS_DIR = WORKSPACE_BASE / "analysis"
REPORTS_DIR = WORKSPACE_BASE / "reports"


# ─────────────────────────────────────────────────────────────
# Data Models
# ─────────────────────────────────────────────────────────────

class CompanyExtract(BaseModel):
    """Structured data extracted from sources"""
    company: str
    features: List[Dict[str, Any]]
    pricing: Dict[str, Any]
    tech_stack: List[str]
    market_position: str
    strengths: List[str]
    weaknesses: List[str]


class PhaseResult(BaseModel):
    """Result from a single phase"""
    phase: str
    duration_s: float
    status: str
    details: Dict[str, Any]


# ─────────────────────────────────────────────────────────────
# Utility Functions
# ─────────────────────────────────────────────────────────────

def log_progress(step: int, total: int, phase: str, message: str, elapsed: float):
    """Log progress to stderr for real-time visibility"""
    print(f"[{step}/{total}] {phase}: {message} ({elapsed:.1f}s)", file=sys.stderr, flush=True)


def setup_workspace():
    """Create workspace directories"""
    for directory in [SOURCES_DIR, EXTRACTS_DIR, ANALYSIS_DIR, REPORTS_DIR]:
        directory.mkdir(parents=True, exist_ok=True)


def save_json(path: Path, data: Any):
    """Save data as JSON"""
    with open(path, 'w') as f:
        json.dump(data, f, indent=2, default=str)


def call_openai(prompt: str, system: str = None) -> str:
    """Call OpenAI GPT-4o"""
    messages = []
    if system:
        messages.append({"role": "system", "content": system})
    messages.append({"role": "user", "content": prompt})

    response = openai_client.chat.completions.create(
        model=OPENAI_MODEL,
        messages=messages,
        temperature=0.7
    )
    return response.choices[0].message.content


def call_anthropic(prompt: str, system: str = None) -> str:
    """Call Anthropic Claude"""
    response = anthropic_client.messages.create(
        model=ANTHROPIC_MODEL,
        max_tokens=4096,
        system=system if system else "You are a strategic business analyst.",
        messages=[{"role": "user", "content": prompt}]
    )
    return response.content[0].text


async def web_search(query: str) -> Dict[str, Any]:
    """Simple web search simulation (can be replaced with real API)"""
    # For demonstration, return mock data
    # In production, integrate with actual search API (e.g., Serper, Tavily)
    await asyncio.sleep(0.5)  # Simulate network delay

    return {
        "query": query,
        "results": [
            {
                "title": f"Result for {query}",
                "url": f"https://example.com/search?q={query.replace(' ', '+')}",
                "snippet": f"Information about {query}...",
                "timestamp": datetime.now().isoformat()
            }
        ]
    }


# ─────────────────────────────────────────────────────────────
# Phase 1: Information Gathering
# ─────────────────────────────────────────────────────────────

async def phase1_gather_sources(companies: List[str], focus_areas: List[str]) -> PhaseResult:
    """Phase 1: Gather information from web sources"""
    start_time = time.time()
    log_progress(1, 6, "Information Gathering", "Generating search queries", time.time() - start_time)

    # Generate search queries using LLM
    query_prompt = f"""Generate 8-10 strategic search queries for competitive intelligence research.

Companies: {', '.join(companies)}
Focus Areas: {', '.join(focus_areas)}

Return queries as JSON array: ["query1", "query2", ...]"""

    queries_json = call_openai(query_prompt, "You are a research strategist. Return only valid JSON.")

    try:
        queries = json.loads(queries_json)
        if isinstance(queries, dict) and "queries" in queries:
            queries = queries["queries"]
    except:
        # Fallback queries
        queries = [
            f"{company} {area}"
            for company in companies
            for area in focus_areas
        ]

    log_progress(1, 6, "Information Gathering", f"Executing {len(queries)} searches", time.time() - start_time)

    # Execute searches in parallel
    search_tasks = [web_search(query) for query in queries[:10]]
    search_results = await asyncio.gather(*search_tasks)

    # Save raw results per company
    for company in companies:
        company_results = [r for r in search_results if company.lower() in r["query"].lower()]
        company_dir = SOURCES_DIR / company
        company_dir.mkdir(exist_ok=True)
        save_json(company_dir / "raw.json", company_results)

    duration = time.time() - start_time
    log_progress(1, 6, "Information Gathering", f"Gathered {len(search_results)} sources", duration)

    return PhaseResult(
        phase="gathering",
        duration_s=round(duration, 1),
        status="completed",
        details={"sources": len(search_results), "queries": len(queries)}
    )


# ─────────────────────────────────────────────────────────────
# Phase 2: Source Extraction
# ─────────────────────────────────────────────────────────────

async def phase2_extract_data(companies: List[str]) -> PhaseResult:
    """Phase 2: Extract structured data from sources"""
    start_time = time.time()
    log_progress(2, 6, "Source Extraction", "Processing sources", time.time() - start_time)

    extracts = {}

    for company in companies:
        # Load raw sources
        company_dir = SOURCES_DIR / company
        raw_file = company_dir / "raw.json"

        if raw_file.exists():
            with open(raw_file) as f:
                sources = json.load(f)
        else:
            sources = []

        # Extract structured data using LLM
        extraction_prompt = f"""Extract structured competitive intelligence from these sources about {company}.

Sources: {json.dumps(sources, indent=2)}

Extract and return as JSON:
{{
  "company": "{company}",
  "features": [{{"name": "...", "description": "...", "unique": true/false}}],
  "pricing": {{"model": "...", "starting_price": "...", "enterprise": true/false}},
  "tech_stack": ["technology1", "technology2"],
  "market_position": "leader/challenger/niche",
  "strengths": ["strength1", "strength2"],
  "weaknesses": ["weakness1", "weakness2"]
}}"""

        try:
            extract_json = call_openai(extraction_prompt, "You are a data analyst. Return only valid JSON.")
            extract_data = json.loads(extract_json)
        except:
            # Fallback mock data
            extract_data = {
                "company": company,
                "features": [{"name": "Feature 1", "description": "Description", "unique": False}],
                "pricing": {"model": "Usage-based", "starting_price": "Contact sales", "enterprise": True},
                "tech_stack": ["Cloud", "API"],
                "market_position": "challenger",
                "strengths": ["Strong technology", "Good support"],
                "weaknesses": ["Limited integrations", "Higher pricing"]
            }

        extracts[company] = extract_data
        save_json(EXTRACTS_DIR / f"{company}.json", extract_data)

    duration = time.time() - start_time
    log_progress(2, 6, "Source Extraction", f"Extracted data from {len(extracts)} companies", duration)

    return PhaseResult(
        phase="extraction",
        duration_s=round(duration, 1),
        status="completed",
        details={"items": len(extracts)}
    )


# ─────────────────────────────────────────────────────────────
# Phase 3: Comparative Analysis
# ─────────────────────────────────────────────────────────────

async def phase3_compare(companies: List[str]) -> PhaseResult:
    """Phase 3: Build comparison matrix and identify differentiators"""
    start_time = time.time()
    log_progress(3, 6, "Comparative Analysis", "Building comparison matrix", time.time() - start_time)

    # Load all extracts
    extracts = {}
    for company in companies:
        extract_file = EXTRACTS_DIR / f"{company}.json"
        if extract_file.exists():
            with open(extract_file) as f:
                extracts[company] = json.load(f)

    # Build comparison using LLM
    comparison_prompt = f"""Analyze these company profiles and create a competitive comparison.

Data: {json.dumps(extracts, indent=2)}

Return as JSON:
{{
  "feature_matrix": [
    {{"feature": "...", "companies": {{"Company1": true, "Company2": false}}, "differentiator": true/false}}
  ],
  "overlaps": ["feature that all companies have"],
  "unique_to": {{"Company1": ["unique feature 1"], "Company2": ["unique feature 2"]}},
  "gaps": [{{"company": "...", "missing": "...", "impact": "high/medium/low"}}],
  "summary": "Overall competitive landscape summary"
}}"""

    try:
        comparison_json = call_openai(comparison_prompt, "You are a competitive analyst. Return only valid JSON.")
        comparison = json.loads(comparison_json)
    except:
        # Fallback
        comparison = {
            "feature_matrix": [],
            "overlaps": ["Payment processing", "API access"],
            "unique_to": {company: ["Unique feature"] for company in companies},
            "gaps": [],
            "summary": "Competitive landscape analysis"
        }

    save_json(ANALYSIS_DIR / "comparison.json", comparison)

    feature_count = len(comparison.get("feature_matrix", []))
    duration = time.time() - start_time
    log_progress(3, 6, "Comparative Analysis", f"Analyzed {feature_count} features", duration)

    return PhaseResult(
        phase="analysis",
        duration_s=round(duration, 1),
        status="completed",
        details={"features": feature_count}
    )


# ─────────────────────────────────────────────────────────────
# Phase 4: Market Intelligence
# ─────────────────────────────────────────────────────────────

async def phase4_intelligence(companies: List[str]) -> PhaseResult:
    """Phase 4: Extract market trends and insights"""
    start_time = time.time()
    log_progress(4, 6, "Market Intelligence", "Analyzing market trends", time.time() - start_time)

    # Load comparison data
    comparison_file = ANALYSIS_DIR / "comparison.json"
    if comparison_file.exists():
        with open(comparison_file) as f:
            comparison = json.load(f)
    else:
        comparison = {}

    # Detect trends using LLM
    trends_prompt = f"""Identify market trends and strategic insights from this competitive analysis.

Comparison Data: {json.dumps(comparison, indent=2)}
Companies: {', '.join(companies)}

Return as JSON:
{{
  "trends": [
    {{"trend": "...", "evidence": "...", "impact": "high/medium/low", "direction": "up/down/stable"}}
  ],
  "market_gaps": ["opportunity 1", "opportunity 2"],
  "threats": ["threat 1", "threat 2"],
  "predictions": ["prediction 1", "prediction 2"]
}}"""

    try:
        intelligence_json = call_openai(trends_prompt, "You are a market intelligence analyst. Return only valid JSON.")
        intelligence = json.loads(intelligence_json)
    except:
        # Fallback
        intelligence = {
            "trends": [
                {"trend": "API-first approach", "evidence": "All companies offer APIs", "impact": "high", "direction": "up"}
            ],
            "market_gaps": ["Small business segment", "International expansion"],
            "threats": ["Regulatory changes", "New entrants"],
            "predictions": ["Consolidation expected", "AI integration growing"]
        }

    save_json(ANALYSIS_DIR / "intelligence.json", intelligence)

    trend_count = len(intelligence.get("trends", []))
    duration = time.time() - start_time
    log_progress(4, 6, "Market Intelligence", f"Identified {trend_count} trends", duration)

    return PhaseResult(
        phase="intelligence",
        duration_s=round(duration, 1),
        status="completed",
        details={"trends": trend_count}
    )


# ─────────────────────────────────────────────────────────────
# Phase 5: Strategic Synthesis (Anthropic)
# ─────────────────────────────────────────────────────────────

async def phase5_synthesize(target_company: str) -> PhaseResult:
    """Phase 5: Deep strategic synthesis using Claude"""
    start_time = time.time()
    log_progress(5, 6, "Strategic Synthesis", "Generating SWOT and recommendations", time.time() - start_time)

    # Load all analysis data
    analysis_data = {}
    for file in ["comparison.json", "intelligence.json"]:
        file_path = ANALYSIS_DIR / file
        if file_path.exists():
            with open(file_path) as f:
                analysis_data[file.replace(".json", "")] = json.load(f)

    # Generate strategy using Claude
    strategy_prompt = f"""You are a strategic business consultant. Synthesize this competitive intelligence into actionable recommendations.

Target Company: {target_company}
Analysis Data: {json.dumps(analysis_data, indent=2)}

Provide:
1. SWOT Analysis (specific to {target_company})
2. Strategic Recommendations (prioritized, actionable)
3. Risk Assessment
4. Opportunities to Pursue

Return as JSON:
{{
  "swot": {{
    "strengths": ["..."],
    "weaknesses": ["..."],
    "opportunities": ["..."],
    "threats": ["..."]
  }},
  "recommendations": [
    {{"priority": "high/medium/low", "action": "...", "rationale": "...", "timeline": "...", "impact": "..."}}
  ],
  "risks": [{{"risk": "...", "likelihood": "high/medium/low", "mitigation": "..."}}],
  "opportunities": [{{"opportunity": "...", "potential": "high/medium/low", "requirements": "..."}}]
}}"""

    try:
        strategy_json = call_anthropic(strategy_prompt)
        # Claude sometimes wraps JSON in markdown, clean it
        if "```json" in strategy_json:
            strategy_json = strategy_json.split("```json")[1].split("```")[0].strip()
        elif "```" in strategy_json:
            strategy_json = strategy_json.split("```")[1].split("```")[0].strip()

        strategy = json.loads(strategy_json)
    except Exception as e:
        print(f"[5/6] Strategy parsing warning: {e}", file=sys.stderr)
        # Fallback
        strategy = {
            "swot": {
                "strengths": ["Strong technology", "Market presence"],
                "weaknesses": ["Pricing pressure", "Limited features"],
                "opportunities": ["Market expansion", "Product innovation"],
                "threats": ["Competition", "Regulation"]
            },
            "recommendations": [
                {"priority": "high", "action": "Strengthen core features", "rationale": "Maintain competitive advantage", "timeline": "Q1-Q2", "impact": "High market retention"}
            ],
            "risks": [{"risk": "Market disruption", "likelihood": "medium", "mitigation": "Diversify offerings"}],
            "opportunities": [{"opportunity": "Enterprise segment", "potential": "high", "requirements": "Enhanced security"}]
        }

    save_json(ANALYSIS_DIR / "strategy.json", strategy)

    rec_count = len(strategy.get("recommendations", []))
    duration = time.time() - start_time
    log_progress(5, 6, "Strategic Synthesis", f"Generated {rec_count} recommendations", duration)

    return PhaseResult(
        phase="synthesis",
        duration_s=round(duration, 1),
        status="completed",
        details={"recommendations": rec_count}
    )


# ─────────────────────────────────────────────────────────────
# Phase 6: Report Generation
# ─────────────────────────────────────────────────────────────

async def phase6_generate_report(target_company: str, companies: List[str]) -> PhaseResult:
    """Phase 6: Generate comprehensive markdown report"""
    start_time = time.time()
    log_progress(6, 6, "Report Generation", "Creating markdown report", time.time() - start_time)

    # Load all data
    all_data = {
        "extracts": {},
        "comparison": {},
        "intelligence": {},
        "strategy": {}
    }

    for company in companies:
        extract_file = EXTRACTS_DIR / f"{company}.json"
        if extract_file.exists():
            with open(extract_file) as f:
                all_data["extracts"][company] = json.load(f)

    for key, file in [("comparison", "comparison.json"), ("intelligence", "intelligence.json"), ("strategy", "strategy.json")]:
        file_path = ANALYSIS_DIR / file
        if file_path.exists():
            with open(file_path) as f:
                all_data[key] = json.load(f)

    # Generate report using LLM
    report_prompt = f"""Create a comprehensive competitive intelligence report in markdown format.

Target Company: {target_company}
Competitors: {', '.join([c for c in companies if c != target_company])}
Data: {json.dumps(all_data, indent=2)}

Structure:
# Competitive Intelligence Report: {target_company}

## Executive Summary
[High-level findings and key takeaways]

## Company Profiles
[Detailed profiles for each company]

## Competitive Analysis
[Feature comparison matrix and differentiators]

## Market Intelligence
[Trends, gaps, predictions]

## Strategic Recommendations
[SWOT and prioritized actions]

## Appendix
[Data sources and methodology]

Make it professional, actionable, and data-driven. Use tables, lists, and clear sections."""

    try:
        report_content = call_openai(report_prompt, "You are a business analyst writing for C-level executives.")
    except:
        # Fallback minimal report
        report_content = f"""# Competitive Intelligence Report: {target_company}

## Executive Summary
Competitive analysis completed for {', '.join(companies)}.

## Analysis Complete
Full analysis available in workspace data files.
"""

    # Save report
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    report_path = REPORTS_DIR / f"competitive_analysis_{timestamp}.md"
    report_path.write_text(report_content)

    # Estimate page count (rough: 500 words per page)
    word_count = len(report_content.split())
    page_count = max(1, word_count // 500)

    duration = time.time() - start_time
    log_progress(6, 6, "Report Generation", f"Generated {page_count}-page report", duration)

    return PhaseResult(
        phase="report",
        duration_s=round(duration, 1),
        status="completed",
        details={"pages": page_count, "path": str(report_path)}
    )


# ─────────────────────────────────────────────────────────────
# Main Handler
# ─────────────────────────────────────────────────────────────

def handler(input_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Orpheus handler for Competitive Intelligence Agent.

    Input:
    {
        "target_company": "Stripe",
        "competitors": ["Adyen", "Square"],
        "focus_areas": ["pricing", "features", "market_position"]
    }

    Output:
    {
        "status": "success",
        "execution_time_seconds": 387,
        "report_path": "/workspace/reports/competitive_analysis_20260115.md",
        "summary": {...},
        "phases": [...]
    }
    """
    start_time = time.time()

    try:
        # Parse input
        target_company = input_data.get("target_company", "")
        competitors = input_data.get("competitors", [])
        focus_areas = input_data.get("focus_areas", ["pricing", "features", "market_position"])

        if not target_company or not competitors:
            return {
                "status": "error",
                "error": "target_company and competitors are required",
                "agent": "competitive-intelligence-agent"
            }

        # All companies to analyze
        all_companies = [target_company] + competitors

        print(f"[START] Competitive Intelligence Analysis", file=sys.stderr, flush=True)
        print(f"[CONFIG] Target: {target_company}, Competitors: {', '.join(competitors)}", file=sys.stderr, flush=True)
        print(f"[CONFIG] Focus Areas: {', '.join(focus_areas)}", file=sys.stderr, flush=True)

        # Setup workspace
        setup_workspace()

        # Execute 6-phase pipeline
        phases = []

        # Phase 1: Information Gathering
        result1 = asyncio.run(phase1_gather_sources(all_companies, focus_areas))
        phases.append(result1.dict())

        # Phase 2: Source Extraction
        result2 = asyncio.run(phase2_extract_data(all_companies))
        phases.append(result2.dict())

        # Phase 3: Comparative Analysis
        result3 = asyncio.run(phase3_compare(all_companies))
        phases.append(result3.dict())

        # Phase 4: Market Intelligence
        result4 = asyncio.run(phase4_intelligence(all_companies))
        phases.append(result4.dict())

        # Phase 5: Strategic Synthesis (Claude)
        result5 = asyncio.run(phase5_synthesize(target_company))
        phases.append(result5.dict())

        # Phase 6: Report Generation
        result6 = asyncio.run(phase6_generate_report(target_company, all_companies))
        phases.append(result6.dict())

        # Calculate totals
        total_duration = time.time() - start_time

        print(f"[COMPLETE] Total time: {total_duration:.1f}s", file=sys.stderr, flush=True)

        return {
            "status": "success",
            "execution_time_seconds": round(total_duration, 1),
            "report_path": result6.details["path"],
            "summary": {
                "companies_analyzed": len(all_companies),
                "sources_gathered": result1.details["sources"],
                "features_compared": result3.details["features"],
                "strategic_recommendations": result5.details["recommendations"]
            },
            "phases": phases
        }

    except Exception as e:
        elapsed = time.time() - start_time
        print(f"[ERROR] Failed after {elapsed:.1f}s: {str(e)}", file=sys.stderr, flush=True)

        return {
            "status": "error",
            "error": str(e),
            "execution_time_seconds": round(elapsed, 1),
            "agent": "competitive-intelligence-agent"
        }


# ─────────────────────────────────────────────────────────────
# CLI Entry Point (for local testing)
# ─────────────────────────────────────────────────────────────

if __name__ == "__main__":
    if len(sys.argv) > 1:
        try:
            input_data = json.loads(sys.argv[1])
        except json.JSONDecodeError:
            print("Error: Invalid JSON input", file=sys.stderr)
            sys.exit(1)
    else:
        # Read from stdin (Orpheus mode)
        input_data = json.loads(sys.stdin.read())

    result = handler(input_data)
    print(json.dumps(result, indent=2))
