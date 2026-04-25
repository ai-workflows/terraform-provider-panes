# Migrating from `panes_agent` to `panes_managed_agent`

The `panes_agent` and `panes_agent_instance` resources are deprecated (since
PR #13). The non-deprecated successor for **standing internal agents** —
the SRE bot, Fleet builder, monitoring agent, etc. — is `panes_managed_agent`,
which talks to Orchestrator directly instead of going through the Panes
proxy shim.

This document walks through migrating one consumer (the SRE agent in
`ai-workflows/agents/terraform/sre`) end-to-end. The other internal staff
agents (`fleet`, `monitoring`, `portal`, `ops`, `platform-eng`) follow the
same pattern.

> **Customer engagements** (Amboras-style — managed via `agent-modules` v1)
> are out of scope here. Their migration path is tracked separately at
> [`ai-workflows/agent-modules#5`](https://github.com/ai-workflows/agent-modules/issues/5)
> and uses `panes_engagement` rather than `panes_managed_agent`.

## Pre-requisites

- The agent already exists in orchestrator. (Internal staff agents do —
  their records are in orchestrator-staging's `agents` table.) Look up the
  ID with:

  ```bash
  gcloud compute ssh panes-staging \
      --project=aiworkflows-panes \
      --zone=us-central1-a \
      --tunnel-through-iap \
      --command='sudo docker run --rm --network host postgres:16 psql \
          "$(gcloud secrets versions access latest --secret=orchestrator-staging-database-url --project=aiworkflows-panes)" \
          -c "SELECT id, name, status FROM agents ORDER BY name;"'
  ```

- The CI workflow (`.github/workflows/terraform-<role>.yml`) is already
  reaching the panes-staging proxy shim via Tailscale. Orchestrator-staging
  sits behind the same gateway, so connectivity is already in place.

- An OAuth2 client_credentials pair for the auth service (or a way to mint
  a service JWT in CI before `terraform apply`). The provider's
  `orchestrator_client_id` + `orchestrator_client_secret` config fields
  refresh JWTs automatically; alternatively, mint a token in a CI step and
  pass it via `ORCHESTRATOR_TOKEN`.

## The migration (per repo)

Take the SRE repo as a worked example. Current state:

```hcl
# terraform/sre/main.tf
resource "panes_agent" "sre" {
  name             = "SRE Agent"
  email            = "sre@agents.fleet.build"
  template_id      = "custom"
  model            = "alias:default"
  compute_class    = "standard"
  system_prompt    = file("prompts/system.md")
  autopilot_prompt = file("prompts/autopilot.md")
  subscription_id  = data.panes_subscription.sre.id
  capabilities     = ["browser"]
}

resource "panes_agent_instance" "sre" {
  agent_id = panes_agent.sre.id
}

resource "panes_ais_account_link" "sre_github" {
  agent_id    = panes_agent.sre.ais_agent_id
  account_id  = var.github_bot_account_id
  permissions = ["read", "totp", "browser_state"]
}
```

Target state:

```hcl
# terraform/sre/main.tf

# Remove the deprecated agent + instance from state without touching the
# underlying orchestrator record.
removed { from = panes_agent.sre,           lifecycle { destroy = false } }
removed { from = panes_agent_instance.sre,  lifecycle { destroy = false } }

# Bind the existing orchestrator agent to the new resource type.
import {
  to = panes_managed_agent.sre
  id = "agent-dj5s8dai"   # SRE Agent's id from the orchestrator-staging lookup above
}

resource "panes_managed_agent" "sre" {
  name             = "SRE Agent"
  display_name     = "SRE Agent"
  role             = "sre"
  model            = "alias:default"
  compute_class    = "standard"
  system_prompt    = file("prompts/system.md")
  autopilot_prompt = file("prompts/autopilot.md")
  subscription_id  = data.panes_subscription.sre.id
}

# panes_ais_account_link's `agent_id` is the AIS agent identity, not the
# managed-agent record id — the field is preserved across the resource-type
# swap because both resources surface `ais_agent_id` with identical
# semantics. Update the reference:
resource "panes_ais_account_link" "sre_github" {
  agent_id    = panes_managed_agent.sre.ais_agent_id
  account_id  = var.github_bot_account_id
  permissions = ["read", "totp", "browser_state"]
}
```

Provider config updates (in the same `main.tf` or `provider.tf`):

```hcl
provider "panes" {
  api_url = var.panes_api_url
  token   = var.panes_token
  org_id  = var.panes_org_id

  # New for panes_managed_agent — orchestrator endpoint.
  orchestrator_url           = "http://gcp-gateway/orchestrator"  # via Tailscale
  orchestrator_client_id     = var.orchestrator_client_id
  orchestrator_client_secret = var.orchestrator_client_secret
}
```

## Apply order (no destroy of the live agent)

The TF 1.7+ `removed` and `import` blocks are evaluated *before* the regular
plan, so a single `terraform apply` does the whole migration atomically:

1. `removed` blocks drop `panes_agent.sre` and `panes_agent_instance.sre`
   from state. `lifecycle { destroy = false }` skips calling Delete on the
   underlying record.
2. `import` block populates `panes_managed_agent.sre` state by reading the
   record at `agent-dj5s8dai`.
3. The resource declaration is reconciled against imported state — there
   should be no diff if the config faithfully mirrors the existing record.
   Any minor differences (e.g. role field newly populated) get applied via
   PATCH.

Verify with `terraform plan` before applying. The plan should show:

```
Terraform will perform the following actions:

  # panes_agent.sre will no longer be managed by Terraform, but will not be destroyed
  # (destroy = false is set in the configuration)
  . removed
  ...

  # panes_managed_agent.sre will be imported
  # ...

Plan: 0 to add, 0 to change, 0 to destroy.
```

If the plan shows actual changes (other than the removed / imported lines),
something has drifted between the deprecated resource's state and the
orchestrator record. Investigate before applying.

## After all consumers migrate

Once `agents/terraform/{sre,fleet,monitoring,portal,ops,platform-eng}` are
all on `panes_managed_agent`, the deprecated resources have no consumers
and can be removed from this provider. Tracking issue:
[`ai-workflows/docs#18`](https://github.com/ai-workflows/docs/issues/18).

The deprecated resources to remove at that point:

- `panes_agent` (and its tests)
- `panes_agent_instance` (and its tests)
- `panes_subscription` resource (the data source stays — no consumers of
  the resource form per `gh search code 'resource "panes_subscription"'`)
- The Panes-side agent-CRUD methods in `internal/client/client.go`

The Panes proxy shim in `ai-workflows/panes` (`api/agents.ts`) can be
removed in the same pass — once nothing calls `/api/agents`, the shim
serves no purpose. Tracking: [`ai-workflows/panes#982`](https://github.com/ai-workflows/panes/issues/982).
