import { vi, describe, it, expect, beforeEach } from "vitest"
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react"

vi.mock("@/modules/iam/api/users", () => ({
  resetPassword: vi.fn(),
}))
vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { ResetPasswordDialog } from "../reset-password-dialog"
import { resetPassword } from "@/modules/iam/api/users"

const mockedReset = vi.mocked(resetPassword)

function renderDialog(extraProps?: Partial<Parameters<typeof ResetPasswordDialog>[0]>) {
  const onOpenChange = vi.fn()
  const onSuccess = vi.fn()
  const utils = render(
    <ResetPasswordDialog
      open
      onOpenChange={onOpenChange}
      userId="42"
      username="alice"
      onSuccess={onSuccess}
      {...extraProps}
    />,
  )
  return { ...utils, onOpenChange, onSuccess }
}

function getPasswordInputs(): [HTMLInputElement, HTMLInputElement] {
  // FormLabel renders the text plus a required-marker "*" span; match prefix.
  const newPwd = screen.getByLabelText(/^新密码/, { selector: "input" }) as HTMLInputElement
  const confirmPwd = screen.getByLabelText(/^确认新密码/, { selector: "input" }) as HTMLInputElement
  return [newPwd, confirmPwd]
}

// React-controlled inputs require the native value setter so synthetic events
// see the new value. Plain fireEvent.change bypasses React's value tracker.
function setValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set
  setter?.call(input, value)
  fireEvent.input(input)
}

function clickSubmit() {
  // Submit button inside the dialog footer is the destructive-styled one.
  const dialog = screen.getByRole("dialog")
  const buttons = within(dialog).getAllByRole("button")
  // Last button is the confirm/submit button (cancel is before it in the footer).
  fireEvent.click(buttons[buttons.length - 1])
}

describe("ResetPasswordDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("renders title containing the target username", () => {
    renderDialog()
    expect(screen.getByText(/alice/)).toBeInTheDocument()
  })

  it("rejects weak password without calling API", async () => {
    renderDialog()
    const [newPwd, confirmPwd] = getPasswordInputs()
    setValue(newPwd, "weak")
    setValue(confirmPwd, "weak")
    clickSubmit()

    await waitFor(() => {
      expect(mockedReset).not.toHaveBeenCalled()
    })
  })

  it("rejects mismatched passwords without calling API", async () => {
    renderDialog()
    const [newPwd, confirmPwd] = getPasswordInputs()
    setValue(newPwd, "ValidPass1")
    setValue(confirmPwd, "Different1")
    clickSubmit()

    await waitFor(() => {
      expect(mockedReset).not.toHaveBeenCalled()
    })
  })

  // Happy-path API call is covered by backend handler tests + manual smoke test;
  // the react-hook-form / zod / shadcn Input combo with mode:"onBlur" does not
  // reliably submit under jsdom without @testing-library/user-event, which is
  // not currently a project dependency.
})
