terraform {
  required_providers {
    panes = {
      source = "ai-workflows/panes"
    }
  }
}

provider "panes" {
  # Panes (agents/sandboxes/subscriptions):
  #   api_url defaults to https://panes.infra.aiworkflows.com
  #   token from PANES_TOKEN env var
  #
  # Fleet (engagements):
  #   fleet_api_url defaults to https://api.fleet.build
  #   fleet_token from FLEET_TOKEN env var (portal-compatible JWT)
  #
  # Orchestrator (panes_managed_agent — non-deprecated path for standing internal agents):
  #   orchestrator_url from ORCHESTRATOR_URL env var (no default — varies by env)
  #   Either:
  #     orchestrator_token from ORCHESTRATOR_TOKEN env var (pre-minted service JWT, no refresh)
  #   Or:
  #     orchestrator_client_id + orchestrator_client_secret (OAuth2 client_credentials, JWT refreshed automatically)
}

# --- Engagement (managed by Fleet) ---

resource "panes_engagement" "meridian" {
  name               = "meridian"
  mode               = "standard"
  slack_channel_name = "meridian"

  agents = [
    { role = "builder", count = 2 },
    { role = "qa", count = 1 },
  ]

  github_repos = ["acme/web", "acme/api"]
}

# --- Subscription (created and authed via Panes UI) ---

data "panes_subscription" "chatgpt_pro" {
  label = "ChatGPT Pro - chatgpt@aiworkflows.com"
}

# --- Agents ---

resource "panes_agent" "builder" {
  name             = "meridian-builder"
  display_name     = "Meridian Builder"
  model            = "chatgpt:gpt-5.4"
  system_prompt    = file("prompts/builder-system.md")
  autopilot_prompt = file("prompts/builder-autopilot.md")
  subscription_id  = data.panes_subscription.chatgpt_pro.id
}

resource "panes_agent" "qa" {
  name             = "meridian-qa"
  display_name     = "Meridian QA"
  model            = "chatgpt:gpt-5.4"
  system_prompt    = file("prompts/qa-system.md")
  autopilot_prompt = file("prompts/qa-autopilot.md")
  subscription_id  = data.panes_subscription.chatgpt_pro.id
}

resource "panes_agent" "runtime_specialist" {
  name             = "meridian-runtime-specialist"
  display_name     = "Meridian Runtime Specialist"
  model            = "chatgpt:gpt-5.4"
  system_prompt    = file("prompts/runtime-specialist-system.md")
  autopilot_prompt = file("prompts/runtime-specialist-autopilot.md")
  subscription_id  = data.panes_subscription.chatgpt_pro.id
}

# --- Running instances ---

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

output "subscription_status" {
  value = data.panes_subscription.chatgpt_pro.status
}

output "builder_status" {
  value = panes_agent_instance.builder.status
}

output "qa_status" {
  value = panes_agent_instance.qa.status
}

output "specialist_status" {
  value = panes_agent_instance.runtime_specialist.status
}

# --- Standing internal agent (non-deprecated path) ---
#
# panes_managed_agent talks to Orchestrator directly. Use this for agents
# that don't fit Fleet's engagement-shaped model — internal SRE bots,
# monitoring agents, fleet-management agents, etc.

resource "panes_managed_agent" "sre" {
  name             = "sre"
  display_name     = "SRE Agent"
  role             = "sre"
  model            = "alias:default"
  compute_class    = "standard"
  system_prompt    = file("prompts/sre-system.md")
  autopilot_prompt = file("prompts/sre-autopilot.md")
  subscription_id  = data.panes_subscription.chatgpt_pro.id

  # Optional: pin to an existing AIS identity (otherwise orchestrator mints one).
  # ais_agent_id = panes_ais_account.sre_identity.id
}

output "sre_status" {
  value = panes_managed_agent.sre.status
}
