import { useState } from "react";
import { Head } from "vite-react-ssg";

import { DashboardPreview } from "../components/DashboardPreview";
import { Footer } from "../components/Footer";
import { Hero } from "../components/Hero";
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
      <div className="flex h-screen flex-col overflow-hidden bg-bg">
        <Navbar />
        <main className="relative flex min-h-0 flex-1 flex-col">
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
          <div className="relative z-10 flex min-h-0 flex-1 flex-col items-center px-6 pb-4">
            <DashboardPreview />
          </div>
        </main>
        <Footer />
      </div>
    </>
  );
}
