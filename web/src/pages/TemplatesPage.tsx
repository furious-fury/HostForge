import { ButtonLink } from "../components/Button";
import { Panel } from "../components/Panel";

const templates = [
  { name: "Web Service", description: "Application service with HTTP routing and health checks." },
  { name: "Worker", description: "Background process for queues, jobs, and async tasks." },
  { name: "Postgres", description: "Managed database service attached to an application." },
  { name: "Redis / Cache", description: "Low-latency cache and queue backing service." },
  { name: "Cron Job", description: "Scheduled container workload with application-level visibility." },
  { name: "Custom Container", description: "Bring your own image and runtime configuration." },
];

export function TemplatesPage() {
  return (
    <div className="flex flex-col gap-5">
      <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">Templates</div>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight text-text">Service templates</h1>
          <p className="mt-1 text-sm text-muted">Start from common service shapes before customizing them in an application workspace.</p>
        </div>
        <ButtonLink to="/projects/new" variant="primary" size="sm">New application</ButtonLink>
      </header>

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {templates.map((template) => (
          <Panel key={template.name} title={template.name}>
            <p className="text-sm text-muted">{template.description}</p>
          </Panel>
        ))}
      </div>
    </div>
  );
}
