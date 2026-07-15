# HostForge Screen Architecture

## Product hierarchy

The new product model should be:

**Application → Service → Deployment**

An **Application** groups everything belonging to one product.  
A **Service** is an independently deployable component.  
A **Deployment** is one build-and-release attempt for a service.

Example:

```text
TaxIO
├── web
├── api
└── worker
    ├── Deployment #184
    ├── Deployment #183
    └── Deployment #182
```

This structure preserves the capabilities in the existing HostForge UI while giving the product a more technical and scalable information architecture.

### Primary screen flow

The main drill-down path through the product should be:

```text
Applications list
    ↓
Application overview
    ↓
Services list
    ↓
Service overview
    ↓
Deployment detail
```

Each level should preserve the current application and service context through breadcrumbs, page titles, and back navigation.

---

## 1. Global application shell

This shell should remain consistent across every authenticated screen.

### Sidebar

#### Workspace
- Overview
- Applications
- Deployments
- Observability

#### Platform
- Settings
- Documentation
- System status

#### Bottom section
- HostForge version
- Server connection status
- Operator profile
- Log out

The sidebar should be narrow and structured like the references: neutral background, small icons, compact labels, and subtle active states.

### Topbar

The topbar should contain:

- Breadcrumb
- Global search or command menu
- Create button
- Notifications
- System-health indicator
- Operator menu

Example:

```text
Applications / TaxIO / Services / API
```

The global **Create** button can offer:

- New application
- New service
- Deploy service

---

## 2. Platform overview

Route:

```text
/
```

### Purpose

Give the operator a quick view of the entire HostForge installation.

### Primary layout

```text
┌──────────────────────────────────────────────────────────────────────┐
│ Overview                              Create Application   Deploy    │
│ Monitor applications, deployments and host resources.               │
├────────────┬────────────┬────────────┬────────────┬──────────────────┤
│ Apps       │ Services   │ Deploys    │ Failed     │ Containers       │
├───────────────────────────────────────┬──────────────────────────────┤
│ Host Resources                        │ System Health                │
│ CPU / Memory / Disk / Network         │ Docker        Operational    │
│                                       │ Caddy         Operational    │
│                                       │ GitHub App    Attention      │
├───────────────────────────────────────┼──────────────────────────────┤
│ Recent Deployments                    │ Setup Progress               │
│ Service / App / Status / Commit       │ 4 of 5 completed             │
└───────────────────────────────────────┴──────────────────────────────┘
```

The desktop overview uses a five-column KPI strip followed by an asymmetric two-column content grid. On smaller screens, the KPI cards should wrap and the right-side panels should stack below their corresponding primary panels.

#### Header

```text
Overview
Monitor your applications, deployments, and host resources.
```

Actions:

- Deploy service
- Create application
- Refresh

#### KPI cards

- Applications
- Active services
- Deployments today
- Failed deployments
- Running containers

Avoid generic commercial metrics such as revenue or customers. Every metric should feel operational.

#### Infrastructure panel

Display:

- CPU usage
- Memory usage
- Root disk usage
- Network throughput
- Running containers

The panel can use sparklines or a compact time-series graph.

#### Recent deployments

Table columns:

- Service
- Application
- Commit
- Status
- Trigger
- Started
- Duration

Each row should open the deployment details screen.

#### System health

Status rows:

- Host
- Docker
- Caddy
- GitHub App
- DNS
- Build engine
- Webhook ingress

Status values:

- Operational
- Degraded
- Needs attention
- Offline

#### Onboarding checklist

Only show this until setup is complete.

Tasks:

- Configure platform domain
- Connect GitHub App
- Verify webhook ingress
- Validate Caddy
- Deploy first service

---

## 3. Applications list

Route:

```text
/applications
```

### Purpose

Show the products being managed on HostForge.

### Header

```text
Applications
Organize related services, deployments, domains, and configuration.
```

Primary action:

```text
Create application
```

### Application card or table

A table is recommended for scalability.

Columns:

- Application
- Services
- Production status
- Latest deployment
- Domains
- Updated
- Actions

Example:

| Application | Services | Status | Latest deployment |
|---|---:|---|---|
| TaxIO | 3 | Healthy | 8 minutes ago |
| GameNation | 2 | Degraded | 2 hours ago |
| HostForge Docs | 1 | Healthy | Yesterday |

Each application can have a small icon or generated initial.

### Filters

- All
- Healthy
- Degraded
- No services

Search by application or service name.

### Empty state

```text
Create your first application

Applications group the services that make up a product.
```

Actions:

- Create application
- Import from GitHub

---

## 4. Create application

Route:

```text
/applications/new
```

### Purpose

Create the application container before adding services.

### Recommended flow

#### Step 1: Application details

Fields:

- Application name
- Description
- Environment or stage
- Team or owner, reserved for later

Example:

```text
Name: TaxIO
Description: Nigerian personal income tax platform
```

#### Step 2: Add first service

Options:

- Import GitHub repository
- Enter repository URL
- Create empty application

Creating an empty application should be supported because users may want to configure shared domains or variables before adding services.

#### Step 3: Review

Show:

- Application name
- Initial service
- Repository
- Branch
- Build method
- Environment variables

Actions:

- Create and deploy
- Create without deploying

---

## 5. Application overview

Route:

```text
/applications/:applicationID
```

### Purpose

Provide a product-level overview across all services.

### Header

Display:

- Application icon
- Application name
- Description
- Overall health
- Production URL
- Last updated

Actions:

- Add service
- Deploy all, only when supported safely
- Application settings
- More menu

### Application navigation

- Overview
- Services
- Deployments
- Domains
- Environment
- Activity
- Settings

### Overview content

#### Summary cards

- Services
- Healthy services
- Failed deployments
- Domains

#### Services panel

Each row should show:

- Service name
- Service type
- Branch
- Runtime status
- Latest deployment
- Public URL
- Resource usage

Example service types:

- Web service
- API service
- Worker
- Scheduled job
- Static site

For the first release, HostForge may only support a generic **Web service**, while the UI structure leaves room for future types.

#### Deployment activity

A timeline of deployments across all services.

#### Application domains

Show the primary domain and any service-level domains.

#### Shared environment

Show only variable names and scope, never secret values.

---

## 6. Services list

Route:

```text
/applications/:applicationID/services
```

### Purpose

Show every deployable unit belonging to the application.

### Header

```text
Services
Deploy and manage the components of TaxIO.
```

Primary action:

```text
Add service
```

### Service table

Columns:

- Service
- Type
- Source
- Branch
- Status
- Latest deployment
- URL
- Actions

Possible service status values:

- Running
- Deploying
- Stopped
- Failed
- Awaiting deployment

### Row actions

- Open service
- Deploy
- Restart
- Stop
- View logs
- Settings

---

## 7. Add service

Route:

```text
/applications/:applicationID/services/new
```

### Purpose

Connect a repository and configure a deployable service.

### Recommended flow

#### Step 1: Source

Options:

- GitHub App (the only supported private-source authentication path)

For GitHub App:

- Select installation
- Select repository
- Select branch

#### Step 2: Service configuration

Fields:

- Service name
- Service type
- Root directory
- Branch
- Build command
- Start command
- Internal port
- Health-check path

Build method can be automatically detected, with an advanced override.

#### Step 3: Environment

- Add environment variables
- Import from `.env`
- Mark variables as secret
- Choose application or service scope

#### Step 4: Networking

- Generate HostForge URL
- Add custom domain later
- Internal port
- Health-check settings

#### Step 5: Deploy

Show a review panel before starting.

Once deployment begins, transition into a live build screen rather than leaving users staring at a spinner.

---

## 8. Service overview

Route:

```text
/applications/:applicationID/services/:serviceID
```

### Purpose

This is the main operational workspace for a deployable service.

### Header

Display:

- Service name
- Application name
- Runtime status
- Service type
- Repository and branch
- Latest commit
- Public URL

Primary actions:

- Deploy
- Restart
- Stop

Secondary menu:

- Roll back
- Copy service URL
- Open repository
- Delete service

### Service navigation

- Overview
- Deployments
- Logs
- Metrics
- Environment
- Domains
- Settings

### Overview content

#### Runtime card

- Status
- Container identifier
- Port
- Started at
- Uptime
- Current deployment

#### Latest deployment

- Status
- Commit
- Author
- Trigger
- Duration
- Started
- Deployment link

#### Resource usage

- CPU
- Memory
- Network
- Container health

#### Source configuration

- Repository
- Branch
- Build method
- Root directory
- Auto-deploy status

#### Domains

- Platform URL
- Custom domains
- DNS verification state
- TLS state

#### Recent activity

- Deployment started
- Environment variable changed
- Domain added
- Service restarted
- Rollback completed

---

## 9. Deployments list

There should be two versions of this screen.

### Global deployments

Route:

```text
/deployments
```

Shows deployments from every application and service.

### Service deployments

Route:

```text
/applications/:applicationID/services/:serviceID/deployments
```

Shows deployments for one service.

### Header actions

- Deploy service
- Refresh

### Filters

- Application
- Service
- Status
- Trigger
- Branch
- Date range

### Deployment table

Columns:

- Deployment
- Application
- Service
- Commit
- Status
- Trigger
- Started
- Duration

Status values:

- Queued
- Building
- Releasing
- Healthy
- Failed
- Cancelled
- Rolled back

A short deployment identifier could look like:

```text
dep_7H3KD9
```

This feels more technical than using only sequential numbers.

---

## 10. Deployment detail

Route:

```text
/applications/:applicationID/services/:serviceID/deployments/:deploymentID
```

### Purpose

Explain exactly what happened during one deployment.

### Header

Display:

- Deployment identifier
- Status
- Service
- Commit
- Branch
- Trigger
- Start time
- Duration

Actions:

- Redeploy
- Roll back to this version
- Cancel, when still active
- Open commit
- Download logs

### Main layout

A two-column layout is recommended.

#### Left: deployment timeline

Stages:

1. Queued
2. Source fetched
3. Build configuration detected
4. Image built
5. Container created
6. Health check passed
7. Route activated

Each stage should show:

- Status
- Start time
- Duration
- Error summary

#### Right: deployment information

- Commit
- Repository
- Branch
- Triggered by
- Build method
- Image identifier
- Container identifier
- Public URL

#### Logs panel

Features:

- Live log stream
- Follow logs toggle
- Search
- Wrap lines
- Timestamps
- Copy selected output
- Download
- Jump to error
- Filter by build stage

The logs should visually resemble a terminal but remain integrated with the light UI.

#### Failure state

When a deployment fails, show:

- Failed stage
- Concise error summary
- Relevant log lines
- Retry action
- Suggested configuration area to inspect

Do not rely on a red status badge alone.

---

## 11. Service logs

Route:

```text
/applications/:applicationID/services/:serviceID/logs
```

### Purpose

Show runtime logs separately from deployment/build logs.

### Controls

- Live stream
- Time range
- Search
- Log level
- Container instance
- Pause stream
- Clear visible logs
- Download

### Log table

Columns or inline metadata:

- Timestamp
- Level
- Source
- Message

Future-ready filters:

- stdout
- stderr
- HTTP requests
- application logs

---

## 12. Service metrics

Route:

```text
/applications/:applicationID/services/:serviceID/metrics
```

### Purpose

Show runtime performance for one service.

### Metrics

- CPU usage
- Memory usage
- Network ingress
- Network egress
- Container restarts
- Request rate
- Error rate
- Response time

The first version can expose only the host/container metrics that already exist, with unsupported metrics omitted rather than filled with fake placeholders.

---

## 13. Environment variables

Two scopes should exist.

### Application environment

Route:

```text
/applications/:applicationID/environment
```

Variables inherited by multiple services.

### Service environment

Route:

```text
/applications/:applicationID/services/:serviceID/environment
```

Overrides or service-specific variables.

### Table columns

- Key
- Value
- Scope
- Services
- Last updated
- Actions

Secret values should appear as:

```text
••••••••••••
```

Actions:

- Add variable
- Edit
- Delete
- Reveal temporarily
- Copy
- Import `.env`
- Export variable names

After changes, show:

```text
Configuration changed. A new deployment is required.
```

Actions:

- Deploy now
- Deploy later

---

## 14. Domains

Application route:

```text
/applications/:applicationID/domains
```

Service route:

```text
/applications/:applicationID/services/:serviceID/domains
```

### Domain table

Columns:

- Domain
- Assigned service
- DNS
- TLS
- Routing
- Updated
- Actions

States:

- Verified
- Pending DNS
- Invalid record
- Caddy sync failed
- TLS provisioning
- Active

### Add-domain flow

1. Enter domain
2. Select service
3. Display required DNS records
4. Check DNS
5. Activate route
6. Provision TLS

The screen should clearly separate:

- Registrar verification
- HostForge routing
- TLS certificate state

---

## 15. Observability

Route:

```text
/observability
```

### Purpose

Provide platform-wide operational telemetry.

### Navigation

- Overview
- Requests
- Deploy steps
- Host
- Events

### Overview cards

- Request rate
- Error rate
- Average response time
- Active deployments
- Failed deploy steps
- Host health

### Request records

Columns:

- Time
- Method
- Path
- Status
- Duration
- Application
- Service

### Deploy-step records

Columns:

- Time
- Deployment
- Service
- Step
- Status
- Duration

### Host metrics

- CPU
- Memory
- Disk
- Network
- Container count

---

## 16. Application settings

Route:

```text
/applications/:applicationID/settings
```

### Sections

#### General
- Name
- Description
- Application icon
- Default environment

#### Git integration
- Default GitHub installation
- Repository access
- Credentials

#### Environment
- Shared variables
- Secret handling

#### Activity
- Configuration changes
- Deployment events
- Operator actions

#### Danger zone
- Archive application
- Delete application

Deleting an application containing services must clearly explain what will happen to:

- Services
- Deployments
- Domains
- Environment variables
- Containers

---

## 17. Service settings

Route:

```text
/applications/:applicationID/services/:serviceID/settings
```

### Sections

#### General
- Service name
- Service type
- Description

#### Source
- Repository
- Branch
- Root directory
- Auto-deploy

#### Build
- Build method
- Build command
- Start command
- Build context

#### Runtime
- Internal port
- Restart policy
- Health-check path
- Health-check timeout

#### Git credentials
- GitHub App installation (private source)
- Public repositories require no stored credential

#### Danger zone
- Stop service
- Disconnect repository
- Delete service

---

## 18. Global settings

Route:

```text
/settings
```

Use a dedicated vertical settings menu.

### Sections

#### Account
- Operator account
- Password
- Session information

#### GitHub App
- Configuration status
- Manifest setup
- Installations
- Sync installations
- Delete configuration

#### Webhooks
- Webhook URL
- Secret status
- Ingress probe
- Recent webhook events

#### Networking and DNS
- Platform domain
- Public IPv4 detection
- DNS information

#### Caddy
- Configuration status
- Validate configuration
- Sync routes

#### Deployments
- Default build settings
- Health-check defaults
- Deployment timeout
- Retention preferences

#### System
- Host details
- Docker status
- Build engine status
- Resource metrics
- Refresh status

#### Preferences
- Default landing page
- Table page size
- Time format
- Reduced motion

#### About
- HostForge version
- Build information
- Documentation
- License

---

## 19. Authentication

Route:

```text
/login
```

### Layout

Keep it minimal.

Left or central card:

- HostForge logo
- “Sign in to your control plane”
- Username
- Password
- Sign in button

Secondary information:

- Host connection
- HostForge version

Avoid marketing content on a locally hosted administrative login screen.

---

## 20. First-time onboarding

Route:

```text
/onboarding
```

### Flow

#### Step 1
Create admin password

#### Step 2
Configure platform URL

#### Step 3
Connect GitHub App

#### Step 4
Verify Caddy and DNS

#### Step 5
Create first application

#### Step 6
Add and deploy first service

The user should be able to leave onboarding and return later. The overview screen can continue showing unfinished tasks.

---

## Main route map

```text
/
├── applications
│   ├── new
│   └── :applicationID
│       ├── overview
│       ├── services
│       │   ├── new
│       │   └── :serviceID
│       │       ├── overview
│       │       ├── deployments
│       │       │   └── :deploymentID
│       │       ├── logs
│       │       ├── metrics
│       │       ├── environment
│       │       ├── domains
│       │       └── settings
│       ├── deployments
│       ├── domains
│       ├── environment
│       ├── activity
│       └── settings
├── deployments
├── observability
├── settings
├── onboarding
└── login
```

---

## Important backend consideration

The current HostForge model appears to treat a **project as the directly deployable unit**. Introducing Applications and Services is not just a wording change if one application should genuinely contain multiple services.

A proper domain model would become:

```text
Application
  has many Services

Service
  belongs to Application
  has repository configuration
  has environment variables
  has domains
  has many Deployments

Deployment
  belongs to Service
```

For a gradual migration, every existing project can initially become:

```text
Application: Existing project name
Service: web
```

That preserves existing data while enabling multi-service applications later.

---

## Recommended design order

Design these first because they establish nearly every reusable component:

1. Global shell
2. Platform overview
3. Applications list
4. Application overview
5. Service overview
6. Deployment detail

Those six screens establish the sidebar, breadcrumbs, cards, tables, health indicators, charts, tabs, actions, logs, and responsive rules required by the rest of the product.