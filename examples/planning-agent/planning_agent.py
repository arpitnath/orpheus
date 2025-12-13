from agents import (
  Agent,
  ModelSettings,
  RunConfig,
  Runner,
  TResponseInputItem,
  WebSearchTool,
  trace,
)
from openai.types.shared.reasoning import Reasoning
from pydantic import BaseModel

# Tool definitions
web_search_preview = WebSearchTool(
  search_context_size="medium",
  user_location={
    "type": "approximate"
  }
)
class TriageSchema(BaseModel):
  has_all_details: bool
  initative_goal: str
  target_timeframe: str
  current_resources: str


triage = Agent(
  name="Triage",
  instructions="""You are an assistant that gathers the key details needed to create a business initiative plan.

Look through the conversation to extract the following:
1. Initiative goal (what the team or organization aims to achieve)
2. Target completion date or timeframe
3. Available resources or current capacity (e.g., headcount, budget, or tool access)

If all three details are present anywhere in the conversation, return:
{
  \"has_all_details\": true,
  \"initiative_goal\": \"<user-provided goal>\",
  \"target_timeframe\": \"<user-provided date or period>\",
  \"current_resources\": \"<user-provided resources>\"
}""",
  model="gpt-5",
  output_type=TriageSchema,
  model_settings=ModelSettings(
    store=True,
    reasoning=Reasoning(
      effort="minimal",
      summary="auto"
    )
  )
)


launch_helper = Agent(
  name="Launch helper",
  instructions="""Come up with a tailored plan to help the user run a new business initiative. Consider all the details they've provide and offer a succinct, bullet point list for how to run the initiative.

Use the web search tool to get additional context and synthesize a succinct answer that clearly explains how to run the project, identifying unique opportunities, highlighting risks and laying out mitigations that make sense.
""",
  model="gpt-4.1-mini",
  tools=[
    web_search_preview
  ],
  model_settings=ModelSettings(
    temperature=1,
    top_p=1,
    max_tokens=2048,
    store=True
  )
)


get_data = Agent(
  name="Get data",
  instructions="""Collect the missing data from the user

Look through the conversation to extract the following:
1. Initiative goal (what the team or organization aims to achieve)
2. Target completion date or timeframe
3. Available resources or current capacity (e.g., headcount, budget, or tool access)

Make sure they are provided, be concise.""",
  model="gpt-5",
  model_settings=ModelSettings(
    store=True,
    reasoning=Reasoning(
      effort="minimal",
      summary="auto"
    )
  )
)


class WorkflowInput(BaseModel):
  input_as_text: str


# Main code entrypoint
async def run_workflow(workflow_input: WorkflowInput):
  with trace("Agent builder workflow"):
    state = {

    }
    workflow = workflow_input.model_dump()
    conversation_history: list[TResponseInputItem] = [
      {
        "role": "user",
        "content": [
          {
            "type": "input_text",
            "text": workflow["input_as_text"]
          }
        ]
      }
    ]
    triage_result_temp = await Runner.run(
      triage,
      input=[
        *conversation_history
      ],
      run_config=RunConfig(trace_metadata={
        "__trace_source__": "agent-builder"
      })
    )

    conversation_history.extend([item.to_input_item() for item in triage_result_temp.new_items])

    triage_result = {
      "output_text": triage_result_temp.final_output.json(),
      "output_parsed": triage_result_temp.final_output.model_dump()
    }
    if triage_result["output_parsed"]["has_all_details"] == True:
      launch_helper_result_temp = await Runner.run(
        launch_helper,
        input=[
          *conversation_history
        ],
        run_config=RunConfig(trace_metadata={
          "__trace_source__": "agent-builder"
        })
      )

      conversation_history.extend([item.to_input_item() for item in launch_helper_result_temp.new_items])

      launch_helper_result = {
        "output_text": launch_helper_result_temp.final_output_as(str)
      }
    else:
      get_data_result_temp = await Runner.run(
        get_data,
        input=[
          *conversation_history
        ],
        run_config=RunConfig(trace_metadata={
          "__trace_source__": "agent-builder"
        })
      )

      conversation_history.extend([item.to_input_item() for item in get_data_result_temp.new_items])

      get_data_result = {
        "output_text": get_data_result_temp.final_output_as(str)
      }
