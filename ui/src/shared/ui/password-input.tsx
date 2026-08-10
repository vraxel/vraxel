import * as React from "react"
import { Eye, EyeOff } from "lucide-react"
import { Input } from "@/shared/ui/input"
import { cn } from "@/shared/lib/utils"

// PasswordInput wraps Input with a trailing eye icon that toggles
// between password and plaintext mode. Drop-in replacement for
// <Input type="password" .../>. Bug #507.
//
// Radix Slot (FormControl) injects id / aria-* onto PasswordInput's
// props via cloneElement. The ...props spread forwards them to <Input>
// and ultimately to the native <input>, so label click-to-focus and
// aria-invalid red-ring styling work correctly.
function PasswordInput({ className, ref, ...props }: Omit<React.ComponentProps<"input">, "type">) {
  const [visible, setVisible] = React.useState(false)
  return (
    <div className="relative">
      <Input
        {...props}
        type={visible ? "text" : "password"}
        className={cn("pr-9", className)}
        ref={ref}
      />
      <button
        type="button"
        tabIndex={-1}
        className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
        onClick={() => setVisible((v) => !v)}
      >
        {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </div>
  )
}

export { PasswordInput }
