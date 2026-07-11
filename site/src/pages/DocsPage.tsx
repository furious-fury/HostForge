import type { LoaderFunctionArgs } from "react-router-dom";
import { redirect, useLoaderData } from "react-router-dom";
import { Head } from "vite-react-ssg";

import { DocsLayout } from "../components/DocsLayout";
import type { DocMeta } from "../lib/docs";
import { getDocBySlug } from "../lib/docs";
import { extractHeadings, markdownToHtml, type TocItem } from "../lib/mdRender";
import {
  OG_IMAGE_HEIGHT,
  OG_IMAGE_WIDTH,
  SITE_NAME,
  SITE_URL,
  TWITTER_HANDLE,
  ogImageUrl,
} from "../lib/seo";

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
  const title = `${meta.title} · HostForge docs`;
  const canonical = `${SITE_URL}/docs/${meta.slug}`;
  const ogImage = ogImageUrl();

  const articleSchema = {
    "@context": "https://schema.org",
    "@type": "TechArticle",
    headline: meta.title,
    description: meta.description,
    inLanguage: "en-US",
    url: canonical,
    isPartOf: {
      "@type": "WebSite",
      name: SITE_NAME,
      url: `${SITE_URL}/`,
    },
    mainEntityOfPage: canonical,
    image: ogImage,
    articleSection: meta.group,
  };

  return (
    <>
      <Head>
        <title>{title}</title>
        <meta name="description" content={meta.description} />
        <link rel="canonical" href={canonical} />

        <meta property="og:type" content="article" />
        <meta property="og:site_name" content={SITE_NAME} />
        <meta property="og:url" content={canonical} />
        <meta property="og:title" content={title} />
        <meta property="og:description" content={meta.description} />
        <meta property="og:image" content={ogImage} />
        <meta property="og:image:width" content={String(OG_IMAGE_WIDTH)} />
        <meta property="og:image:height" content={String(OG_IMAGE_HEIGHT)} />
        <meta property="article:section" content={meta.group} />

        <meta name="twitter:card" content="summary_large_image" />
        <meta name="twitter:site" content={TWITTER_HANDLE} />
        <meta name="twitter:title" content={title} />
        <meta name="twitter:description" content={meta.description} />
        <meta name="twitter:image" content={ogImage} />

        <script type="application/ld+json">{JSON.stringify(articleSchema)}</script>
      </Head>
      <DocsLayout meta={meta} slug={meta.slug} headings={headings} html={html} />
    </>
  );
}
