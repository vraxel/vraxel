import { ApiReferenceReact } from "@scalar/api-reference-react"
import "@scalar/api-reference-react/style.css"

export default function ApiDocsPage() {
  return (
    <div className="h-screen">
      <ApiReferenceReact
        configuration={{
          url: "/docs/openapi.json",
        }}
      />
    </div>
  )
}
