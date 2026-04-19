import { Route, Routes } from "react-router-dom";
import { Shell } from "./components/Shell";
import { ToastProvider } from "./components/ToastProvider";
import { DashboardPage } from "./pages/DashboardPage";
import { DeploymentsPage } from "./pages/DeploymentsPage";
import { DeploymentPage } from "./pages/DeploymentPage";
import { NewProjectPage } from "./pages/NewProjectPage";
import { ProjectPage } from "./pages/ProjectPage";
import { ProjectsPage } from "./pages/ProjectsPage";

export default function App() {
  return (
    <ToastProvider>
      <Shell>
        <Routes>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/deployments" element={<DeploymentsPage />} />
          <Route path="/projects" element={<ProjectsPage />} />
          <Route path="/projects/new" element={<NewProjectPage />} />
          <Route path="/projects/:projectID" element={<ProjectPage />} />
          <Route path="/projects/:projectID/deployments/:deploymentID" element={<DeploymentPage />} />
          <Route path="*" element={<DashboardPage />} />
        </Routes>
      </Shell>
    </ToastProvider>
  );
}
