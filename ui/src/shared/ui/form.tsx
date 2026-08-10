"use client"

import * as React from "react"
import type { Label as LabelPrimitive } from "radix-ui"
import { Slot } from "radix-ui"
import {
  Controller,
  FormProvider,
  useFormContext,
  useFormState,
  type ControllerProps,
  type FieldPath,
  type FieldValues,
} from "react-hook-form"

import { cn } from "@/shared/lib/utils"
import { Label } from "@/shared/ui/label"

const Form = FormProvider

type FormFieldContextValue<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
> = {
  name: TName
}

const FormFieldContext = React.createContext<FormFieldContextValue>(
  {} as FormFieldContextValue
)

const FormField = <
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
>({
  ...props
}: ControllerProps<TFieldValues, TName>) => {
  return (
    <FormFieldContext.Provider value={{ name: props.name }}>
      <Controller {...props} />
    </FormFieldContext.Provider>
  )
}

const useFormField = () => {
  const fieldContext = React.useContext(FormFieldContext)
  const itemContext = React.useContext(FormItemContext)
  // Hooks below run unconditionally to satisfy React's Rules of Hooks --
  // the throw guards land afterwards. useFormState tolerates an empty
  // name (returns the whole form state); useFormContext returns null
  // when there is no FormProvider above, which we surface via the same
  // throw path.
  const formContext = useFormContext()
  const formState = useFormState({ name: fieldContext.name })

  // Both context providers use {} as their default value, so
  // React.useContext returns truthy even outside any provider. Detect
  // missing context by required keys. Throwing here is intentional:
  // FormLabel / FormControl / FormDescription / FormMessage rendered
  // outside FormField+FormItem produces htmlFor="undefined-form-item",
  // which the browser flags as "Incorrect use of <label for=FORM_ELEMENT>"
  // and is the recurring source of label-id-mismatch bugs.
  if (!fieldContext.name) {
    throw new Error(
      "Form components (FormLabel/FormControl/FormDescription/FormMessage) must be used inside <FormField>. " +
        "For purely visual labels not bound to react-hook-form, use <Label> from @/components/ui/label.",
    )
  }
  if (!itemContext.id) {
    throw new Error(
      "Form components (FormLabel/FormControl/FormDescription/FormMessage) must be wrapped in <FormItem>.",
    )
  }
  if (!formContext) {
    throw new Error(
      "Form components must be rendered inside a react-hook-form <Form> (FormProvider).",
    )
  }

  const fieldState = formContext.getFieldState(fieldContext.name, formState)
  const { id } = itemContext

  return {
    id,
    name: fieldContext.name,
    formItemId: `${id}-form-item`,
    formDescriptionId: `${id}-form-item-description`,
    formMessageId: `${id}-form-item-message`,
    ...fieldState,
  }
}

type FormItemContextValue = {
  id: string
}

const FormItemContext = React.createContext<FormItemContextValue>(
  {} as FormItemContextValue
)

function FormItem({ className, ...props }: React.ComponentProps<"div">) {
  const id = React.useId()
  const fieldContext = React.useContext(FormFieldContext)
  const ref = React.useRef<HTMLDivElement>(null)

  // Dev guard: catches Radix Select/Checkbox/Switch/RadioGroup that forgot
  // to receive name={field.name}. Their hidden BubbleInput then mounts
  // with neither name nor id, which is exactly what Chrome flags as
  // "A form field element should have an id or name attribute". <Input>
  // and <Textarea> have their own guards in ui/input.tsx and
  // ui/textarea.tsx, so this only catches the BubbleInput case here.
  React.useEffect(() => {
    if (import.meta.env.PROD) return
    if (!ref.current) return
    const orphans = ref.current.querySelectorAll(
      "input:not([name]):not([id])," +
        "select:not([name]):not([id])," +
        "textarea:not([name]):not([id])",
    )
    if (orphans.length > 0) {
      const first = orphans[0] as HTMLElement
      throw new Error(
        `<FormField name="${fieldContext.name ?? "?"}"> contains a form field ` +
          `element without name or id. Likely a Radix Select / Checkbox / Switch / ` +
          `RadioGroup missing name={field.name}. ` +
          `Element: ${first.outerHTML.slice(0, 200)}`,
      )
    }
  })

  return (
    <FormItemContext.Provider value={{ id }}>
      <div
        ref={ref}
        data-slot="form-item"
        className={cn("grid grid-cols-1 gap-2", className)}
        {...props}
      />
    </FormItemContext.Provider>
  )
}

function FormLabel({
  className,
  required,
  children,
  ...props
}: React.ComponentProps<typeof LabelPrimitive.Root> & { required?: boolean }) {
  const { error, formItemId } = useFormField()

  // Dev guard: <FormControl> uses Slot to inject id={formItemId} into a
  // labelable element. If the surrounding <FormItem> has no <FormControl>
  // (e.g. label sits above a Checkbox list / button group / ScrollArea),
  // no DOM element ever takes that id and the browser logs
  // "Incorrect use of <label for=FORM_ELEMENT>". By the time useEffect
  // runs the commit phase has mounted every sibling, so a single
  // getElementById is the ground truth -- no register / unregister
  // bookkeeping needed.
  React.useEffect(() => {
    if (import.meta.env.PROD) return
    if (!document.getElementById(formItemId)) {
      throw new Error(
        `<FormLabel> with htmlFor="${formItemId}" has no matching <FormControl> ` +
          `inside its <FormItem>. Either wrap the bound input in <FormControl>, ` +
          `or use plain <Label> from @/components/ui/label for section headings ` +
          `(see ui/CLAUDE.md "裸 FormItem 包多个互斥子字段时").`,
      )
    }
  }, [formItemId])

  return (
    <Label
      data-slot="form-label"
      data-error={!!error}
      className={cn("data-[error=true]:text-destructive", className)}
      htmlFor={formItemId}
      {...props}
    >
      {children}
      {required && <span className="text-destructive ml-0.5">*</span>}
    </Label>
  )
}

function FormControl({ ...props }: React.ComponentProps<typeof Slot.Root>) {
  const { error, formItemId, formDescriptionId, formMessageId } = useFormField()

  return (
    <Slot.Root
      data-slot="form-control"
      id={formItemId}
      aria-describedby={
        !error
          ? `${formDescriptionId}`
          : `${formDescriptionId} ${formMessageId}`
      }
      aria-invalid={!!error}
      {...props}
    />
  )
}

function FormDescription({ className, ...props }: React.ComponentProps<"p">) {
  const { formDescriptionId } = useFormField()

  return (
    <p
      data-slot="form-description"
      id={formDescriptionId}
      className={cn("text-sm text-muted-foreground break-words", className)}
      {...props}
    />
  )
}

function FormMessage({ className, ...props }: React.ComponentProps<"p">) {
  const { error, formMessageId } = useFormField()
  const body = error ? String(error?.message ?? "") : props.children
  const text = typeof body === "string" ? body : undefined

  return (
    <p
      data-slot="form-message"
      id={formMessageId}
      title={text}
      className={cn("h-10 text-sm leading-5 text-destructive line-clamp-2", className)}
      {...props}
    >
      {body}
    </p>
  )
}

export {
  useFormField,
  Form,
  FormItem,
  FormLabel,
  FormControl,
  FormDescription,
  FormMessage,
  FormField,
}
