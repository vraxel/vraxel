// Standalone entry for the Scalar API reference (api-docs.html). Kept out
// of the SPA graph on purpose: Scalar ships an embedded Vue runtime
// (~2.5MB pre-gzip) that would otherwise ride in every user's first
// paint. Static-import rule intact -- this is the multi-entry split the
// no-dynamic-import policy prescribes for oversized leaves.
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import ApiDocsPage from "./pages/api-docs"

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ApiDocsPage />
  </StrictMode>,
)
