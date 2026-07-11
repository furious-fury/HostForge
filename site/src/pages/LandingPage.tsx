import { useState } from "react";
import { Head } from "vite-react-ssg";

import { CodeSampleSection } from "../components/CodeSampleSection";
import { DashboardPreview } from "../components/DashboardPreview";
import { FaqSection } from "../components/FaqSection";
import { FeatureGridSection } from "../components/FeatureGridSection";
import { FinalCtaSection } from "../components/FinalCtaSection";
import { Footer } from "../components/Footer";
import { Hero } from "../components/Hero";
import { HowItWorksSection } from "../components/HowItWorksSection";
import { Navbar } from "../components/Navbar";
import {
  DEFAULT_KEYWORDS,
  OG_IMAGE_HEIGHT,
  OG_IMAGE_WIDTH,
  SITE_NAME,
  SITE_URL,
  TWITTER_HANDLE,
  absoluteUrl,
  ogImageUrl,
} from "../lib/seo";

const TITLE = "HostForge — self-hosted PaaS from Git";
const DESCRIPTION =
  "Ship Git repos to Docker on a single Linux host. Nixpacks builds, Caddy zero-downtime cutover, live logs, encrypted env, and a React control plane — all self-hosted.";

const STRUCTURED_DATA = {
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "WebSite",
      "@id": `${SITE_URL}/#website`,
      url: `${SITE_URL}/`,
      name: SITE_NAME,
      description: DESCRIPTION,
      inLanguage: "en-US",
    },
    {
      "@type": "SoftwareApplication",
      name: SITE_NAME,
      applicationCategory: "DeveloperApplication",
      operatingSystem: "Linux",
      description: DESCRIPTION,
      url: `${SITE_URL}/`,
      image: ogImageUrl(),
      offers: {
        "@type": "Offer",
        price: "0",
        priceCurrency: "USD",
      },
    },
  ],
};

export function LandingPage() {
  const [videoFailed, setVideoFailed] = useState(false);
  const canonical = `${SITE_URL}/`;
  const ogImage = ogImageUrl();

  return (
    <>
      <Head>
        <title>{TITLE}</title>
        <meta name="description" content={DESCRIPTION} />
        <meta name="keywords" content={DEFAULT_KEYWORDS} />
        <link rel="canonical" href={canonical} />

        <meta property="og:type" content="website" />
        <meta property="og:site_name" content={SITE_NAME} />
        <meta property="og:url" content={canonical} />
        <meta property="og:title" content={TITLE} />
        <meta property="og:description" content={DESCRIPTION} />
        <meta property="og:image" content={ogImage} />
        <meta property="og:image:width" content={String(OG_IMAGE_WIDTH)} />
        <meta property="og:image:height" content={String(OG_IMAGE_HEIGHT)} />
        <meta property="og:image:alt" content="HostForge control plane dashboard" />

        <meta name="twitter:card" content="summary_large_image" />
        <meta name="twitter:site" content={TWITTER_HANDLE} />
        <meta name="twitter:title" content={TITLE} />
        <meta name="twitter:description" content={DESCRIPTION} />
        <meta name="twitter:image" content={ogImage} />

        <link rel="alternate" type="application/xml" title="Sitemap" href={absoluteUrl("/sitemap.xml")} />
        <script type="application/ld+json">{JSON.stringify(STRUCTURED_DATA)}</script>
      </Head>
      <div className="flex min-h-screen flex-col bg-bg">
        <Navbar />
        <main className="relative flex flex-1 flex-col">
          <section className="relative flex flex-col overflow-hidden">
            {!videoFailed ? (
              <video
                aria-hidden
                autoPlay
                className="pointer-events-none absolute inset-0 z-0 h-full w-full object-cover opacity-25"
                loop
                muted
                playsInline
                onError={() => setVideoFailed(true)}
              >
                <source src="/demo.mp4" type="video/mp4" />
              </video>
            ) : null}
            <Hero />
            <div className="relative z-10 flex flex-col items-center px-6 pb-16">
              <DashboardPreview />
            </div>
          </section>

          <FeatureGridSection />
          <HowItWorksSection />
          <CodeSampleSection />
          <FaqSection />
          <FinalCtaSection />
        </main>
        <Footer />
      </div>
    </>
  );
}
