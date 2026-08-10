import * as React from "react"

import { Input } from "./input"

// IntegerInput — drop-in replacement for <Input type="number"> when the
// field stores a non-negative integer as a string. Visual rejection so
// invalid input never reaches the form state:
//
//   - digits only (paste / IME edge cases stripped)
//   - leading zeros stripped (preserves standalone "0" for e.g. replicas)
//   - length capped to max's digit count
//   - value clamped to max (paste of > max truncates to max)
//
// Value is kept as a string (not a number) so it composes with the zod
// `z.string()` schemas used elsewhere; an empty string represents "not
// entered" rather than 0. zod still validates format defensively in case
// the user bypasses the input (programmatic injection, refill flows).
//
// `type="text"` + `inputMode="numeric"` deliberately replaces the native
// `type="number"`: the spec for type=number permits leading zeros,
// scientific notation and decimal separators, none of which we want for
// integer fields like ports and resource counts.

type IntegerInputProps = Omit<
  React.ComponentProps<typeof Input>,
  "type" | "value" | "onChange" | "inputMode" | "pattern"
> & {
  value?: string
  onChange?: (value: string) => void
  max?: number
}

function IntegerInput({ value, onChange, max, ...rest }: IntegerInputProps) {
  const maxLen = max !== undefined ? String(max).length : undefined

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    let v = e.target.value.replace(/\D/g, "")
    if (v.length > 1) v = v.replace(/^0+/, "") || "0"
    if (maxLen && v.length > maxLen) v = v.slice(0, maxLen)
    if (max !== undefined && v !== "" && Number(v) > max) v = String(max)
    onChange?.(v)
  }

  return (
    <Input
      {...rest}
      type="text"
      inputMode="numeric"
      pattern="\d*"
      value={value ?? ""}
      onChange={handleChange}
    />
  )
}

export { IntegerInput }
