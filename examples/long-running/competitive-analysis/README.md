# Competitive Analysis Agent

This agent demonstrates **long-running tasks** in Orpheus. It takes a list of URLs and performs a deep analysis, which can take several minutes to complete.

## How it Works

1.  **Input:** Takes a list of competitor URLs and a topic.
2.  **Processing:**
    *   Simulates crawling each website (5-10 seconds per site).
    *   Extracts key points related to the topic.
    *   Synthesizes a final report using an LLM (simulated).
3.  **Output:** Returns a markdown-formatted analysis report.

## Key Features

*   **Custom Timeout:** Configured with a 10-minute timeout (`timeout: 600`) to handle long processing times.
*   **Progress Tracking:** Logs progress to stdout, which can be viewed in real-time logs.
*   **Memory Usage:** Allocates more memory (`memory: 1024`) for handling large text content.

## Usage

```bash
orpheus run competitive-analysis '{
  "urls": ["example.com", "competitor.io"],
  "topic": "pricing strategy"
}'
```
