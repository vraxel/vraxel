import { BrowserRouter, useRoutes } from "react-router"
import { QueryClientProvider } from "@tanstack/react-query"
import { queryClient } from "@/core/query/client"
import { routes } from "./routes"
import { Toaster } from "@/shared/ui/sonner"

function AppRoutes() {
  return useRoutes(routes)
}

export default function App() {
  return (
    <BrowserRouter>
      <QueryClientProvider client={queryClient}>
        <AppRoutes />
        {/* swipeDirections=[] turns off sonner's swipe-to-dismiss. The swipe
            translates the toast on pointer drag, which competes with native text
            selection so toast text (e.g. long server validation errors) can't be
            dragged-to-select for copying. With no swipe direction the toast never
            moves and the drag selects text normally; closeButton restores an
            explicit dismiss affordance, and hover still pauses the auto-dismiss
            timer so there is time to copy (#474). */}
        <Toaster
          position="top-center"
          offset="4rem"
          closeButton
          swipeDirections={[]}
          toastOptions={{ classNames: { description: "break-all" } }}
        />
      </QueryClientProvider>
    </BrowserRouter>
  )
}
