// Radix renders a hidden BubbleInput for every checkbox inside a <form>.
// The permission tree contributes ~100 of them; unnamed, they trip
// FormItem's dev guard (and Chrome's "form field should have an id or
// name" warning), which crashed the role create/edit dialog on open.
import { describe, expect, it } from "vitest"
import { useForm } from "react-hook-form"
import { render } from "@testing-library/react"
import { TooltipProvider } from "@/shared/ui/tooltip"
import { Form, FormField, FormItem, FormMessage } from "@/shared/ui/form"
import { PermissionSelector } from "../permission-selector"
import type { Permission } from "@/modules/iam/api/types"

const permissions: Permission[] = [
  {
    apiVersion: "iam/v1",
    kind: "Permission",
    metadata: { id: "1", name: "iam:users:list", createdAt: "", updatedAt: "" },
    spec: {
      code: "iam:users:list",
      method: "GET",
      path: "/users",
      scope: "platform",
      description: "",
    },
  },
  {
    apiVersion: "iam/v1",
    kind: "Permission",
    metadata: { id: "2", name: "iam:users:create", createdAt: "", updatedAt: "" },
    spec: {
      code: "iam:users:create",
      method: "POST",
      path: "/users",
      scope: "platform",
      description: "",
    },
  },
]

function Harness() {
  const form = useForm<{ rules: string[] }>({ defaultValues: { rules: [] } })
  return (
    <TooltipProvider>
      <Form {...form}>
        <form>
          <FormField
            control={form.control}
            name="rules"
            render={() => (
              <FormItem>
                <PermissionSelector
                  permissions={permissions}
                  value={[]}
                  onChange={() => {}}
                  scope="platform"
                />
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </TooltipProvider>
  )
}

describe("PermissionSelector inside a FormField", () => {
  it("mounts without tripping the orphan-form-control guard", () => {
    expect(() => render(<Harness />)).not.toThrow()
  })

  it("gives every hidden checkbox input a name", () => {
    const { container } = render(<Harness />)
    const inputs = container.querySelectorAll("input")
    expect(inputs.length).toBeGreaterThan(0)
    const orphans = [...inputs].filter((el) => !el.getAttribute("name") && !el.getAttribute("id"))
    expect(orphans).toHaveLength(0)
  })
})
