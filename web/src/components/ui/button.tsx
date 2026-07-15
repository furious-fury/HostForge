/* eslint-disable react-refresh/only-export-components */
import * as React from "react"
import { Slot } from "@radix-ui/react-slot"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-[color,background-color,border-color,box-shadow,transform] outline-none hover:-translate-y-px focus-visible:ring-3 focus-visible:ring-ring/30 active:translate-y-0 disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-accent text-accent-foreground shadow-[0_2px_8px_rgb(31_35_30_/_0.08)] hover:bg-accent/90 hover:shadow-[0_5px_14px_rgb(31_35_30_/_0.14)] focus-visible:ring-ring/35 active:shadow-[0_1px_4px_rgb(31_35_30_/_0.08)]",
        outline: "border bg-muted/80 shadow-[0_2px_8px_rgb(31_35_30_/_0.055)] hover:-translate-y-px hover:border-foreground/25 hover:bg-muted hover:shadow-[0_5px_14px_rgb(31_35_30_/_0.11)] focus-visible:border-accent focus-visible:ring-3 focus-visible:ring-ring/25 active:translate-y-0 active:shadow-[0_1px_4px_rgb(31_35_30_/_0.06)]",
        ghost: "border border-transparent hover:border-border hover:bg-muted hover:shadow-[0_4px_12px_rgb(31_35_30_/_0.08)] focus-visible:border-accent active:shadow-none",
        destructive: "bg-destructive text-white shadow-[0_2px_8px_rgb(127_29_29_/_0.12)] hover:bg-destructive/90 hover:shadow-[0_5px_14px_rgb(127_29_29_/_0.18)] focus-visible:ring-destructive/30",
      },
      size: {
        default: "h-9 px-4 py-2",
        sm: "h-8 px-3 text-xs",
        lg: "h-10 px-6",
        icon: "size-9",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
)

function Button({
  className,
  variant,
  size,
  asChild = false,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
  }) {
  const Comp = asChild ? Slot : "button"

  return <Comp className={cn(buttonVariants({ variant, size, className }))} {...props} />
}

export { Button, buttonVariants }
