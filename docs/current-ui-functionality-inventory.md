# Current UI Functionality Inventory

## Purpose

This document inventories the functionality exposed by the current HostForge management UI so a replacement UI can preserve product capability while changing the visual design, information architecture, and interaction model.

This inventory is based on the current React management app in `web/` and the Go server/API in `cmd/server`.

## Where the current UI is housed

- Management UI: `web/`
- UI entrypoint: `web/src/main.tsx`
- App routes: `web/src/App.tsx`
- Shared shell/layout: `web/src/components/Shell.tsx`
- API client layer: `web/src/api.ts`, `web/src/api/settings.ts`, `web/src/api/host.ts`
- UI serving from server: `cmd/server/static_ui.go`
- API/auth/webhook route registration: `cmd/server/main.go`

Important distinction:

- `web/` is the actual control-plane UI
- `site/` is the public marketing/docs site

## Current top-level route map

The currently routed management UI surfaces are:

- `/`
  - Default landing route
  - Resolves to the dashboard overview
- `/observability`
  - Observability and operational telemetry
- `/projects`
  - Project list
- `/projects/new`
  - New project / new application flow
- `/projects/:projectID`
  - Project overview
- `/projects/:projectID/settings`
  - Project settings side sheet
- `/projects/:projectID/deployments/:deploymentID`
  - Deployment detail view
- `/settings`
  - Host/system/global settings

Not currently routed, but present as code:

- `web/src/pages/DeploymentsPage.tsx`
- `web/src/pages/TemplatesPage.tsx`

These should be treated as partial or dormant surfaces unless intentionally revived.

## Authentication and app shell

### Admin session login

Current UI supports:

- Login form for the `admin` user
- Password-based authenticated session creation
- Session existence check on app load
- Logout and session deletion

Frontend sources:

- `web/src/App.tsx`

Server/API backing:

- `POST /auth/session`
- `GET /auth/session`
- `DELETE /auth/session`

Handler sources:

- `cmd/server/main.go` via:
  - `handleSessionRoutes`
  - `handleSessionCreate`
  - `handleSessionStatus`
  - `handleSessionDelete`

### Global shell behavior

Current UI shell includes:

- Sidebar navigation
- Topbar/breadcrumbing
- Toasts
- Confirm dialogs
- UI preference persistence

Primary frontend sources:

- `web/src/components/Shell.tsx`
- `web/src/components/Sidebar.tsx`
- `web/src/components/Topbar.tsx`
- `web/src/components/ToastProvider.tsx`
- `web/src/components/useConfirm.tsx`
- `web/src/hooks/useUIPrefs.tsx`

## Dashboard overview

Primary screen:

- `web/src/pages/DashboardPage.tsx`

Current functionality:

- Overview heading and summary framing
- Onboarding progress panel when bootstrap is not complete
- KPI cards for:
  - Active projects
  - Deployments in last 24 hours
  - Failed deployments in last 24 hours
  - Running containers
- Host metrics card with:
  - CPU
  - Memory
  - Root disk usage
  - Network throughput
  - Historical sparklines
- Recent activity table with latest deployments
- System health/status summary panel

Backend/API dependencies:

- `GET /api/projects`
- `GET /api/deployments`
- `GET /api/system/status`
- `GET /api/onboarding`
- `GET /api/system/host/snapshot`
- `GET /api/system/host/history`

Relevant server handlers:

- `handleProjectsList`
- `handleDeploymentsCollection`
- `handleSystemStatus`
- `handleOnboardingRoutes`
- `handleHostSnapshot`
- `handleHostHistory`

## Projects list

Primary screen:

- `web/src/pages/ProjectsPage.tsx`

Current functionality inferred from route structure and shared data layer:

- List all projects
- Navigate into a project overview
- Jump to create a new project
- Show deployment/runtime summary information per project

Backend/API dependencies:

- `GET /api/projects`

Relevant server handlers:

- `handleProjectsCollection`
- `handleProjectsList`

## New project / application creation

Primary screen:

- `web/src/pages/NewProjectPage.tsx`

Current functionality:

- Create a project from repository URL
- Create a project via GitHub App repository selection
- Installation picker for GitHub App installations
- Repository picker within an installation
- Branch discovery for repositories
- Project name suggestion and editing
- Optional environment variable entry before first deploy
- Project creation
- Immediate deploy after project creation
- Build/deploy progress state machine
- Deployment log streaming/tailing during provisioning

Backend/API dependencies:

- `POST /api/projects`
- `GET /api/repositories/branches`
- `GET /api/github/app`
- `GET /api/github/installations`
- `POST /api/github/installations/sync`
- `GET /api/github/installations/:id/repositories`
- `POST /api/projects/:projectID/deploy`
- `GET /api/projects/:projectID/deployments`
- `GET /api/deployments/:deploymentID/logs`
- live deployment log endpoint under `/api/deployments/:deploymentID/...`

Relevant server handlers:

- `handleProjectCreate`
- `handleRepositoryBranches`
- `handleGitHubAppRoutes`
- `handleGitHubApp`
- `handleGitHubInstallations`
- `handleGitHubInstallationsSync`
- `handleGitHubInstallationRepositories`
- `handleProjectDeployAction`
- `handleProjectDeploymentsGet`
- `handleDeploymentRoutes`
- `handleDeploymentLogsTail`
- `handleDeploymentLogsLive`

## Project overview

Primary screen:

- `web/src/pages/ProjectPage.tsx`

Current functionality:

- Project summary header
  - name
  - repo URL
  - stack/build badges
  - latest deployment status
  - branch/container/last deploy summary
- Access links section
  - public/default URL visibility
  - domain summary
- Runtime controls
  - redeploy
  - restart
  - rollback
  - stop
- Build method panel
- Deployment history table
- Error/retry feedback
- Environment-pending redeploy warning

Backend/API dependencies:

- `GET /api/projects/:projectID`
- `GET /api/projects/:projectID/deployments`
- `POST /api/projects/:projectID/deploy`
- `POST /api/projects/:projectID/restart`
- `POST /api/projects/:projectID/rollback`
- `POST /api/projects/:projectID/stop`
- `DELETE /api/projects/:projectID`

Relevant server handlers:

- `handleProjectGet`
- `handleProjectDeploymentsGet`
- `handleProjectDeployAction`
- `handleProjectRestartAction`
- `handleProjectRollbackAction`
- `handleProjectStopAction`
- `handleProjectDelete`

## Project settings inside the project screen

Primary screen:

- `web/src/pages/ProjectPage.tsx`

The project settings experience currently lives in a right-side sheet rather than a separate standalone page.

### Environment variables

Current functionality:

- List env vars
- Create env var
- Update env var
- Delete env var
- Mark project as needing redeploy after env change

Backend/API dependencies:

- `GET /api/projects/:projectID/env`
- `POST /api/projects/:projectID/env`
- `PUT /api/projects/:projectID/env/:envID`
- `DELETE /api/projects/:projectID/env/:envID`

Relevant server handlers:

- `handleProjectEnvList`
- `handleProjectEnvPost`
- `handleProjectEnvPut`
- `handleProjectEnvDelete`

### Private repository credentials

Current functionality:

- GitHub App source selection
- Personal access token save/rotate/delete
- SSH deploy key generate/view/delete
- Switch project git source method

Backend/API dependencies:

- `GET /api/projects/:projectID/git-auth`
- `PUT /api/projects/:projectID/git-auth`
- `DELETE /api/projects/:projectID/git-auth`
- `GET /api/projects/:projectID/ssh-key`
- `POST /api/projects/:projectID/ssh-key/generate`
- `DELETE /api/projects/:projectID/ssh-key`
- `PUT /api/projects/:projectID/git-source`
- `GET /api/github/installations`

Relevant server handlers:

- `handleProjectGitAuthGet`
- `handleProjectGitAuthPut`
- `handleProjectGitAuthDelete`
- `handleProjectSSHKeyGet`
- `handleProjectSSHKeyGenerate`
- `handleProjectSSHKeyDelete`
- `handleProjectGitSourcePut`
- `handleGitHubInstallations`

### Custom domains

Current functionality:

- Display platform/default URL
- Add custom domain
- Edit custom domain
- Delete custom domain
- Show Caddy route state
- Show registrar DNS verification state
- Trigger DNS refresh/check
- Show DNS guidance and copyable DNS records
- Surface Caddy sync outcomes

Backend/API dependencies:

- `GET /api/projects/:projectID`
- `GET /api/projects/:projectID/domains`
- `POST /api/projects/:projectID/domains`
- `PATCH /api/projects/:projectID/domains/:domainID`
- `DELETE /api/projects/:projectID/domains/:domainID`

Relevant server handlers:

- `handleProjectDomainsCollection`
- `handleProjectDomainsPost`
- `handleProjectDomainPatch`
- `handleProjectDomainDelete`

### Danger zone

Current functionality:

- Delete project with destructive confirmation

Backend/API dependencies:

- `DELETE /api/projects/:projectID`

Relevant server handlers:

- `handleProjectDelete`

## Deployment detail view

Primary screen:

- `web/src/pages/DeploymentPage.tsx`

Current functionality, based on route/API structure:

- View a single deployment
- Inspect deployment status and metadata
- Tail or stream deployment logs
- View deployment step timeline / step records
- Navigate back to project context

Backend/API dependencies:

- `GET /api/deployments/:deploymentID/...`
- deployment log read/tail endpoints
- deployment live log websocket/stream endpoint
- `GET /api/observability/deployments/:deploymentID/steps` or equivalent deployment-steps endpoint

Relevant server handlers:

- `handleDeploymentRoutes`
- `handleDeploymentLogsTail`
- `handleDeploymentLogsLive`
- `handleDeploymentSteps`

## Observability

Primary screen:

- `web/src/pages/ObservabilityPage.tsx`

Current functionality:

- Observability summary metrics
- Recent HTTP request records
- Recent deploy-step records
- Operational telemetry for requests and deployments

Backend/API dependencies:

- `GET /api/observability/summary`
- `GET /api/observability/requests`
- `GET /api/observability/deploy-steps`

Relevant server handlers:

- `handleObservabilityRoutes`
- `handleObservabilitySummary`
- `handleObservabilityRequests`
- `handleObservabilityDeploySteps`

## Global settings

Primary screen:

- `web/src/pages/SettingsPage.tsx`

Current settings tabs:

- Account
- GitHub App
- Webhooks
- DNS
- Caddy
- Build and health
- System
- Preferences
- About

Primary settings API entrypoint:

- `GET /api/settings`
- `POST /api/settings/actions/...`

Relevant server handlers:

- `handleSettingsRoutes`
- `handleSettingsGet`
- `handleSettingsActionCaddyValidate`
- `handleSettingsActionCaddySync`
- `handleSettingsActionRefreshStatus`
- `handleSettingsActionDetectPublicIPv4`

### Account

Current functionality:

- View operator/account-level settings and secret state
- Review management auth and webhook secret related information

Frontend source:

- `web/src/pages/settings/AccountSection.tsx`

### GitHub App

Current functionality:

- View GitHub App configuration state
- Generate GitHub App manifest
- Exchange manifest callback code
- Delete stored GitHub App configuration
- View installations / connect app workflow context
- Configure webhook/public callback URL inputs for local/public setups

Frontend source:

- `web/src/pages/settings/GitHubAppSection.tsx`

Backend/API dependencies:

- `GET /api/github/app`
- `POST /api/github/app/manifest`
- `POST /api/github/app/exchange`
- `DELETE /api/github/app`
- `GET /api/github/installations`
- `POST /api/github/installations/sync`

### Webhooks

Current functionality:

- Show webhook-related server configuration
- Show route probe/system status context for webhook ingress

Frontend source:

- `web/src/pages/settings/WebhooksSection.tsx`

### DNS

Current functionality:

- Show DNS-related host/platform configuration
- Detect public IPv4

Frontend source:

- `web/src/pages/settings/DnsSection.tsx`

Backend/API dependencies:

- `POST /api/settings/actions/detect-public-ipv4`

### Caddy

Current functionality:

- Show Caddy-related configuration
- Validate generated/active Caddy setup
- Trigger Caddy sync

Frontend source:

- `web/src/pages/settings/CaddySection.tsx`

Backend/API dependencies:

- `POST /api/settings/actions/caddy-validate`
- `POST /api/settings/actions/caddy-sync`

### Build and health

Current functionality:

- Show deploy default settings
- Show health-check-related settings/explanations

Frontend source:

- `web/src/pages/settings/DeploySection.tsx`

### System

Current functionality:

- View host/server/system configuration
- View host metrics in a settings context
- Refresh system status

Frontend source:

- `web/src/pages/settings/SystemSection.tsx`

Backend/API dependencies:

- `GET /api/system/status`
- `GET /api/system/host/snapshot`
- `GET /api/system/host/history`
- `POST /api/settings/actions/refresh-status`

### Preferences

Current functionality:

- Local UI preferences such as default landing page and deployments page size

Frontend source:

- `web/src/pages/settings/PreferencesSection.tsx`

Note:

- Preferences are UI-side behavior, not core backend product capability.

### About

Current functionality:

- Show version/build/about information

Frontend source:

- `web/src/pages/settings/AboutSection.tsx`

## Onboarding capability

Current onboarding functionality currently surfaces in the dashboard and server status flow:

- Bootstrap/onboarding status visibility
- GitHub App completion state
- Platform domain state
- Permanent ingress completion state
- Completion endpoint support on the server

Backend/API dependencies:

- `GET /api/onboarding`
- onboarding completion route under `/api/onboarding/...`

Relevant server handlers:

- `handleOnboardingRoutes`
- `handleOnboardingComplete`

## Webhook capability

The current product also includes server-side functionality that matters to the management experience even when not deeply exposed as a standalone screen:

- GitHub webhook endpoint
- webhook-triggered deployments
- installation event handling

Server/API entrypoints:

- configured webhook path, default `/hooks/github`

Relevant handlers:

- `handleGitHubWebhook`
- `handleInstallationEvent`

## Current rebuild checklist

A replacement UI should account for, at minimum:

- Admin authentication session flow
- Dashboard KPIs, onboarding, recent activity, host metrics, system status
- Project list and navigation
- New project flow from URL and GitHub App installation
- Branch lookup and repo selection
- Initial deploy and live deployment logs
- Project overview header and runtime controls
- Deployment history
- Environment variable CRUD
- Git credential management
- SSH deploy key management
- Git source switching
- Domain CRUD plus DNS/Caddy visibility
- Project deletion
- Deployment detail and step/log inspection
- Observability summaries and request/deploy telemetry
- Global settings across account, GitHub App, webhooks, DNS, Caddy, deploy defaults, system, preferences, and about

## Suggested next documentation pass

The next useful document would be a feature-to-endpoint matrix with these columns:

- UI surface
- user action
- frontend component/page
- API endpoint(s)
- server handler(s)
- notes for redesign

That would be the best bridge from this legacy UI into a rebuilt UI with a new IA and visual system.
