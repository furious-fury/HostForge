import { Outlet } from "react-router-dom";

import { SiteThemeProvider } from "../hooks/useSiteTheme";

export function SiteRootLayout() {
  return (
    <SiteThemeProvider>
      <Outlet />
    </SiteThemeProvider>
  );
}
