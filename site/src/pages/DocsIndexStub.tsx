import { Navigate } from "react-router-dom";
import { Head } from "vite-react-ssg";

/** `/docs` → `/docs/introduction` (SSG-friendly; no loader redirect). */
export function DocsIndexStub() {
  return (
    <>
      <Head>
        <meta content="0;url=/docs/introduction" httpEquiv="refresh" />
        <link href="/docs/introduction" rel="canonical" />
      </Head>
      <Navigate replace to="/docs/introduction" />
    </>
  );
}
