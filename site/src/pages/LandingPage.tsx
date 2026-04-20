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

export function LandingPage() {
  const [videoFailed, setVideoFailed] = useState(false);

  return (
    <>
      <Head>
        <title>HostForge — self-hosted PaaS from Git</title>
        <meta
          name="description"
          content="Git → Nixpacks → Docker on one machine. API, UI, GitHub webhooks, and Caddy routing."
        />
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
