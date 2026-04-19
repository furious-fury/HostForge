import { Navigate } from "react-router-dom";
import { useUIPrefs } from "../hooks/useUIPrefs";
import { DashboardPage } from "./DashboardPage";

export function DefaultLandingRoute() {
  const { prefs } = useUIPrefs();
  const landing = prefs.defaultLanding;
  if (landing === "/") {
    return <DashboardPage />;
  }
  return <Navigate to={landing} replace />;
}
