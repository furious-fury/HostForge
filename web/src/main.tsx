import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import App from "./router"
import { ThemeProvider } from "./theme-provider"
import { BrowserRouter } from "react-router-dom"
import { QueryClientProvider } from "@tanstack/react-query"
import { queryClient } from "./query-client"
import "./index.css"
import { RouteAwareAppErrorBoundary } from "./app-error-boundary"
import { ToastProvider } from "./toast-provider"

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <ToastProvider>
            <RouteAwareAppErrorBoundary>
              <App />
            </RouteAwareAppErrorBoundary>
          </ToastProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </BrowserRouter>
  </StrictMode>,
)
