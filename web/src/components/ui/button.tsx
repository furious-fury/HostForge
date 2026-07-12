import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../../lib/utils";

const buttonVariants = cva(
  "inline-flex min-h-10 items-center justify-center gap-2 rounded-md border font-medium transition-colors duration-150 active:translate-y-px focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus focus-visible:ring-offset-2 focus-visible:ring-offset-bg disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        primary: "border-primary bg-primary text-primary-ink hover:brightness-110",
        secondary: "border-border-strong bg-transparent text-text hover:border-muted hover:bg-border",
        danger: "border-danger bg-transparent text-danger hover:bg-danger hover:text-primary-ink",
        ghost: "border-transparent bg-transparent text-muted hover:bg-surface-alt hover:text-text",
      },
      size: {
        sm: "min-h-8 px-3 py-1.5 text-xs",
        md: "px-4 py-2 text-sm",
      },
    },
    defaultVariants: { variant: "secondary", size: "md" },
  },
);

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> {}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(({ className, variant, size, ...props }, ref) => (
  <button ref={ref} className={cn(buttonVariants({ variant, size }), className)} {...props} />
));
Button.displayName = "Button";

export { Button, buttonVariants };
