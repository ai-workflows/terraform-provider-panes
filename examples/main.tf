terraform {
  required_providers {
    panes = {
      source = "ai-workflows/panes"
    }
  }
}

provider "panes" {
  # api_url defaults to https://app.a9s.dev
  # token from PANES_TOKEN env var
}

# --- Agents (config only, not running) ---

resource "panes_agent" "builder" {
  name                 = "meridian-builder"
  template_id          = "custom"
  model                = "chatgpt:gpt-5.4"
  reasoning_effort     = "high"
  system_prompt        = file("prompts/builder-system.md")
  autopilot_prompt     = file("prompts/builder-autopilot.md")
  done_for_now_enabled = false
}

resource "panes_agent" "qa" {
  name                 = "meridian-qa"
  template_id          = "custom"
  model                = "chatgpt:gpt-5.4"
  reasoning_effort     = "high"
  system_prompt        = file("prompts/qa-system.md")
  autopilot_prompt     = file("prompts/qa-autopilot.md")
  done_for_now_enabled = false
}

resource "panes_agent" "runtime_specialist" {
  name                 = "meridian-runtime-specialist"
  template_id          = "custom"
  model                = "chatgpt:gpt-5.4"
  reasoning_effort     = "high"
  system_prompt        = file("prompts/runtime-specialist-system.md")
  autopilot_prompt     = file("prompts/runtime-specialist-autopilot.md")
  done_for_now_enabled = false
}

# --- Running instances ---
# Creating these starts the agent (provisions sandbox + session).
# Destroying stops the agent.
# Comment out to stop an agent without deleting its config.

resource "panes_agent_instance" "builder" {
  agent_id = panes_agent.builder.id
}

resource "panes_agent_instance" "qa" {
  agent_id = panes_agent.qa.id
}

resource "panes_agent_instance" "runtime_specialist" {
  agent_id = panes_agent.runtime_specialist.id
}

# --- Outputs ---

output "builder_status" {
  value = panes_agent_instance.builder.status
}

output "qa_status" {
  value = panes_agent_instance.qa.status
}

output "specialist_status" {
  value = panes_agent_instance.runtime_specialist.status
}
