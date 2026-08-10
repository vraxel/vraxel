import * as React from "react"
import { XIcon } from "lucide-react"
import { Dialog as DialogPrimitive } from "radix-ui"

import { cn } from "@/shared/lib/utils"
import { Button } from "@/shared/ui/button"

function Dialog({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />
}

function DialogTrigger({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Trigger>) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />
}

function DialogPortal({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />
}

function DialogClose({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />
}

function DialogOverlay({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        "fixed inset-0 z-50 bg-black/50 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0",
        className
      )}
      {...props}
    />
  )
}

function DialogContent({
  className,
  children,
  showCloseButton = true,
  onCloseAutoFocus,
  onPointerDownOutside,
  onInteractOutside,
  onPointerDown,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Content> & {
  showCloseButton?: boolean
}) {
  return (
    <DialogPortal data-slot="dialog-portal">
      <DialogOverlay />
      {/*
        Bug #269: 创建服务 dialog 默认状态下字体模糊，点击输入框聚焦才变清晰。
        旧实现把视觉容器自己定位为 `fixed top-[50%] left-[50%] translate(-50%, -50%)`。
        当视口高 / max-h:85vh 等给出非整数像素 (例如 viewport=633 → 85vh≈538.05),
        translate(-50%) 会把元素落在子像素位置 (transform: matrix(1,0,0,1,-448,-269.023))，
        Chrome 文字栅格化在子像素上看起来发虚。点击输入框触发 focus / repaint，
        合成层重新栅格化时被对齐到整数像素，文字突然变清晰。

        修复：DialogPrimitive.Content 自身只做 fixed+flex 居中容器（透明、无边框/背景），
        视觉框（圆角/边框/背景/动画）下沉到内部 `<div>`。Flex 居中让浏览器直接给出
        整数像素的边距，不再依赖 translate(-50%, -50%)。consumer className 继续落到
        视觉框上以保留 `sm:max-w-Nxl h-[85vh] flex flex-col overflow-hidden` 等约定。
        外层吸收 zoom-in/out 动画前缀以便内层 group-data-* 触发，外层自身仅 fade。

        反作用：Radix dismissableLayer 的 outside-click 检测是 "target 不在
        Content 子树内" → 触发。但 Content 现在自占 fixed inset-0，target
        永远在 Content 内（点 transparent margin 也是 Content 自己），outside
        永远不触发，所以点 dialog 外部不再自动关闭。下面 onPointerDown
        手动 synthesise outside-click 语义：仅当 target 就是 Content 元素本
        身（不是任何子节点）才触发 close。consumer 既可继续传 Radix 标准
        的 onPointerDownOutside / onInteractOutside 拦截（busy 时 preventDefault
        等模式不变），也可以自己接管 onPointerDown。
      */}
      <DialogPrimitive.Content
        data-slot="dialog-content"
        onCloseAutoFocus={(e) => {
          if (onCloseAutoFocus) {
            onCloseAutoFocus(e)
          } else {
            // Prevent focus returning to trigger during close animation
            // to avoid aria-hidden conflict with ancestor elements
            e.preventDefault()
          }
        }}
        onPointerDown={(e) => {
          onPointerDown?.(e)
          if (e.defaultPrevented) return
          if (e.target !== e.currentTarget) return
          // Build a synthetic Radix-shape event so consumer-supplied
          // onPointerDownOutside / onInteractOutside (which Radix would
          // call but no longer can — see comment above) still see and
          // can preventDefault their dismissal.
          if (onPointerDownOutside || onInteractOutside) {
            let prevented = false
            const synthetic = {
              detail: { originalEvent: e.nativeEvent },
              preventDefault: () => { prevented = true; e.preventDefault() },
              get defaultPrevented() { return prevented },
              currentTarget: e.currentTarget,
              target: e.target,
              type: "dismissableLayer.pointerDownOutside",
            } as unknown as CustomEvent
            onPointerDownOutside?.(synthetic as never)
            if (!prevented) onInteractOutside?.(synthetic as never)
            if (prevented) return
          }
          // Trigger close. Prefer the rendered Close button so we ride
          // the same Radix-provided onOpenChange path the X icon uses;
          // fall back to Escape when no close button is rendered.
          const closeBtn = e.currentTarget.querySelector<HTMLElement>('[data-slot="dialog-close"]')
          if (closeBtn) {
            closeBtn.click()
          } else {
            e.currentTarget.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }))
          }
        }}
        className="group/dialog-content fixed inset-0 z-50 flex items-center justify-center p-4 outline-none pointer-events-none data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0"
        {...props}
      >
        <div
          data-slot="dialog-content-frame"
          className={cn(
            // grid-cols-[minmax(0,1fr)]: grid item 默认 min-width:auto = min-content,
            // 长 whitespace-nowrap 文本(如 SelectTrigger 显示长凭证名)会撑大 grid track,
            // 突破 dialog 内容区。显式 minmax(0,1fr) 强制 track 收缩,溢出由子元素自己处理。
            "relative grid grid-cols-[minmax(0,1fr)] w-full max-w-[calc(100%-2rem)] gap-4 rounded-lg border bg-background p-6 shadow-lg duration-200 overflow-hidden pointer-events-auto group-data-[state=open]/dialog-content:animate-in group-data-[state=open]/dialog-content:zoom-in-95 group-data-[state=closed]/dialog-content:animate-out group-data-[state=closed]/dialog-content:zoom-out-95 sm:max-w-lg",
            className
          )}
        >
          {children}
          {showCloseButton && (
            <DialogPrimitive.Close
              data-slot="dialog-close"
              className="absolute top-4 right-4 rounded-xs opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:outline-hidden disabled:pointer-events-none data-[state=open]:bg-accent data-[state=open]:text-muted-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4"
            >
              <XIcon />
              <span className="sr-only">Close</span>
            </DialogPrimitive.Close>
          )}
        </div>
      </DialogPrimitive.Content>
    </DialogPortal>
  )
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="dialog-header"
      className={cn("flex flex-col gap-2 text-center sm:text-left", className)}
      {...props}
    />
  )
}

function DialogFooter({
  className,
  showCloseButton = false,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  showCloseButton?: boolean
}) {
  return (
    <div
      data-slot="dialog-footer"
      className={cn(
        "flex flex-col-reverse gap-2 sm:flex-row sm:justify-end",
        className
      )}
      {...props}
    >
      {children}
      {showCloseButton && (
        <DialogPrimitive.Close asChild>
          <Button variant="outline">Close</Button>
        </DialogPrimitive.Close>
      )}
    </div>
  )
}

function DialogTitle({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      className={cn("text-lg leading-none font-semibold", className)}
      {...props}
    />
  )
}

function DialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      className={cn("text-sm text-muted-foreground", className)}
      {...props}
    />
  )
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
}
