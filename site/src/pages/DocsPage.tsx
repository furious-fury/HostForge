import type { LoaderFunctionArgs } from "react-router-dom";
import { redirect, useLoaderData } from "react-router-dom";
import { Head } from "vite-react-ssg";

import { DocsLayout } from "../components/DocsLayout";
import type { DocMeta } from "../lib/docs";
import { getDocBySlug } from "../lib/docs";
import { extractHeadings, markdownToHtml, type TocItem } from "../lib/mdRender";

export type DocsLoaderData = {
  meta: DocMeta;
  html: string;
  headings: TocItem[];
};

export async function docsPageLoader({ params }: LoaderFunctionArgs): Promise<DocsLoaderData> {
  const slug = params.slug ?? "";
  const doc = getDocBySlug(slug);
  if (!doc) {
    throw redirect("/docs/introduction");
  }
  const headings = extractHeadings(doc.body);
  const html = await markdownToHtml(doc.body);
  return { meta: doc.meta, html, headings };
}

export function DocsPage() {
  const { meta, html, headings } = useLoaderData() as DocsLoaderData;
  return (
    <>
      <Head>
        <title>{`${meta.title} · HostForge docs`}</title>
        <meta name="description" content={meta.description} />
      </Head>
      <DocsLayout meta={meta} slug={meta.slug} headings={headings} html={html} />
    </>
  );
}
