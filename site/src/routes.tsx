import type { RouteRecord } from "vite-react-ssg";

import { SiteRootLayout } from "./components/SiteRootLayout";
import { DocsPage, docsPageLoader } from "./pages/DocsPage";
import { DocsIndexStub } from "./pages/DocsIndexStub";
import { LandingPage } from "./pages/LandingPage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { getStaticDocPaths } from "./lib/docs";

export const routes: RouteRecord[] = [
  {
    path: "/",
    Component: SiteRootLayout,
    children: [
      { index: true, Component: LandingPage },
      { path: "docs", Component: DocsIndexStub },
      {
        path: "docs/:slug",
        Component: DocsPage,
        loader: docsPageLoader,
        getStaticPaths: getStaticDocPaths,
      },
      { path: "*", Component: NotFoundPage },
    ],
  },
];
