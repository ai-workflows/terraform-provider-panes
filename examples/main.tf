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

# Sandbox for the builder agent
resource "panes_sandbox" "builder" {
  cloud         = "gcp"
  instance_type = "n2-standard-8"
  nested_virt   = true
  disk_size     = 100
}

# Sandbox for the QA agent
resource "panes_sandbox" "qa" {
  cloud         = "gcp"
  instance_type = "n2-standard-8"
  nested_virt   = true
  disk_size     = 100
}

# Builder agent
resource "panes_agent" "builder" {
  name              = "meridian-builder"
  template_id       = "custom"
  model             = "chatgpt:gpt-5.4"
  reasoning_effort  = "high"
  system_prompt     = file("prompts/builder-system.md")
  autopilot_prompt  = file("prompts/builder-autopilot.md")
  done_for_now_enabled = false
}

# QA agent
resource "panes_agent" "qa" {
  name              = "meridian-qa"
  template_id       = "custom"
  model             = "chatgpt:gpt-5.4"
  reasoning_effort  = "high"
  system_prompt     = file("prompts/qa-system.md")
  autopilot_prompt  = file("prompts/qa-autopilot.md")
  done_for_now_enabled = false
}

# Runtime specialist agent
resource "panes_agent" "runtime_specialist" {
  name              = "meridian-runtime-specialist"
  template_id       = "custom"
  model             = "chatgpt:gpt-5.4"
  reasoning_effort  = "high"
  system_prompt     = file("prompts/runtime-specialist-system.md")
  autopilot_prompt  = file("prompts/runtime-specialist-autopilot.md")
  done_for_now_enabled = false
}

output "builder_sandbox_id" {
  value = panes_sandbox.builder.id
}

output "qa_sandbox_id" {
  value = panes_sandbox.qa.id
}

output "builder_agent_id" {
  value = panes_agent.builder.id
}

output "qa_agent_id" {
  value = panes_agent.qa.id
}

output "specialist_agent_id" {
  value = panes_agent.runtime_specialist.id
}
